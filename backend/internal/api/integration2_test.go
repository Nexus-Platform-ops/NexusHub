package api

// Dating radar, swipes, whisper TTL, coins ledger, marketplace and chat.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nexus/backend/internal/config"
)

func setupRadarWorld(t *testing.T) (tokenA, tokenB, tokenC, tokenD, tokenE, tokenF string, uidA, uidB int64) {
	t.Helper()
	uidA, tokenA = makeUser(t, "Radar A")
	uidB, tokenB = makeUser(t, "Radar B") // friend of A
	_, tokenC = makeUser(t, "Radar C")    // hidden profile
	_, tokenD = makeUser(t, "Radar D")
	_, tokenE = makeUser(t, "Radar E")
	_, tokenF = makeUser(t, "Radar F")
	for _, tok := range []string{tokenA, tokenB, tokenC, tokenD, tokenE, tokenF} {
		makeAdult(t, tok)
	}

	addFriend(t, uidA, uidB)

	type named struct {
		name string
		tok  string
	}
	for _, n := range []named{{"A", tokenA}, {"B", tokenB}, {"C", tokenC}, {"D", tokenD}, {"E", tokenE}, {"F", tokenF}} {
		st, resp, _ := doReq(t, "PUT", "/api/v1/dating/profile",
			map[string]any{
				"nickname":               "Nick" + n.name,
				"bio":                    "bio " + n.name,
				"is_hidden_from_friends": n.name == "C",
			}, n.tok)
		needStatus(t, st, http.StatusOK, resp)
	}
	i := 0
	for _, n := range []named{{"B", tokenB}, {"C", tokenC}, {"D", tokenD}, {"E", tokenE}, {"F", tokenF}} {
		i++
		st, resp, _ := doReq(t, "GET",
			fmt.Sprintf("/api/v1/radar?lat=%.5f&lng=%.5f", 16.8+float64(i)*0.001, 96.15), nil, n.tok)
		needStatus(t, st, http.StatusOK, resp)
	}
	return
}

func TestRadarKAnonymity(t *testing.T) {
	resetDB(t)
	tokenA, _, _, _, _, _, _, _ := setupRadarWorld(t)

	// Isolated user far away → degraded, zero nodes.
	_, tokenG := makeUser(t, "Radar G")
	makeAdult(t, tokenG)
	st, resp, _ := doReq(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": "NickG", "is_hidden_from_friends": false}, tokenG)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=-20.0&lng=57.5", nil, tokenG)
	needStatus(t, st, http.StatusOK, resp)
	if resp["degraded"] != true || len(resp["nodes"].([]any)) != 0 {
		t.Fatalf("expected degraded radar for isolated user: %v", resp)
	}

	// A's radar: friend B and hidden C excluded, D/E/F remain.
	st, resp, rawBody := doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if resp["degraded"] != false {
		t.Fatalf("radar unexpectedly degraded: %v", resp)
	}
	nodes := resp["nodes"].([]any)
	if len(nodes) != 3 {
		t.Fatalf("radar nodes = %d want 3 (%v)", len(nodes), rawBody)
	}
	if strings.Contains(string(rawBody), "Nick") {
		t.Fatal("radar leaked real nicknames")
	}
	seen := map[string]bool{}
	for _, n := range nodes {
		m := n.(map[string]any)
		handle, _ := m["handle"].(string)
		if !strings.HasPrefix(handle, "Bee-") {
			t.Fatalf("handle not anonymous: %v", m)
		}
		seen[handle] = true
		switch m["distance_bucket"] {
		case "500m", "1km", "2km":
		default:
			t.Fatalf("bad distance bucket %q", m["distance_bucket"])
		}
	}
	if len(seen) != 3 {
		t.Fatalf("handles not unique: %v", seen)
	}

	// Validation errors.
	st, resp, _ = doReq(t, "GET", "/api/v1/radar", nil, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=999&lng=96.15", nil, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Dating profile validation + get.
	st, resp, _ = doReq(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": "x"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/dating/profile", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if resp["nickname"] != "NickA" {
		t.Fatalf("dating profile get: %v", resp)
	}
}

func TestSwipeAndWhisper(t *testing.T) {
	resetDB(t)
	tokenA, tokenB, _, _, _, _, uidA, uidB := setupRadarWorld(t)

	// Invalid swipes.
	st, resp, _ := doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": "!!!", "action": "like"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": nodeIDEncode(999999), "action": "like"}, tokenA)
	needStatus(t, st, http.StatusNotFound, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": nodeIDEncode(uidB), "action": "maybe"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Whisper before a match is rejected.
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "hi"}, tokenA)
	needStatus(t, st, http.StatusForbidden, resp)

	// Mutual like creates a match.
	st, resp, _ = doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": nodeIDEncode(uidB), "action": "like"}, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if resp["matched"] != false {
		t.Fatalf("matched before reciprocal like: %v", resp)
	}
	st, resp, _ = doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": nodeIDEncode(uidA), "action": "like"}, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["matched"] != true {
		t.Fatalf("mutual like did not match: %v", resp)
	}

	// Whisper send + read-once semantics.
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "self-destruct hello"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	wid := resp["id"].(string)
	if int(resp["expires_in"].(float64)) != 5 {
		t.Fatalf("whisper ttl = %v want 5", resp["expires_in"])
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid, nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["body"] != "self-destruct hello" {
		t.Fatalf("whisper body = %v", resp["body"])
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid, nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)

	// A non-recipient must not read (and the message is destroyed).
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "second"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	wid2 := resp["id"].(string)
	_, tokenX := makeUser(t, "Radar X")
	makeAdult(t, tokenX)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid2, nil, tokenX)
	needStatus(t, st, http.StatusForbidden, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid2, nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)

	// TTL expiry: after 5s in Redis the message is gone even unread.
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "expire me"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	wid3 := resp["id"].(string)
	time.Sleep(5100 * time.Millisecond)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid3, nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)
}

func TestCoinsLedger(t *testing.T) {
	resetDB(t)
	uid, token := makeUser(t, "Coin User")

	st, resp, _ := doReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 500, "request_id": "req-00000001"}, token)
	needStatus(t, st, http.StatusBadRequest, resp) // missing signature

	req := httptestReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 500, "request_id": "req-00000002"})
	req.Header.Set("X-Payment-Signature", "deadbeef")
	w := record(t, req, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad signature accepted: %d", w.Code)
	}

	st, resp, _ = doReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 0, "request_id": "req-00000003"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 500, "request_id": "short"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)

	out := topup(t, uid, token, 500, "req-10000001")
	if out["credited"].(float64) != 500 || out["balance"].(float64) != 500 {
		t.Fatalf("first topup: %v", out)
	}
	out = topup(t, uid, token, 1000, "req-10000002")
	if out["balance"].(float64) != 1500 {
		t.Fatalf("second topup: %v", out)
	}
	// Idempotent replay must not double-credit.
	out = topup(t, uid, token, 1000, "req-10000002")
	if out["idempotent_replay"] != true || out["balance"].(float64) != 1500 {
		t.Fatalf("replay: %v", out)
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/balance", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["balance"].(float64) != 1500 {
		t.Fatalf("balance = %v want 1500", resp["balance"])
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/ledger", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["ledger"].([]any)); got != 2 {
		t.Fatalf("ledger rows = %d want 2", got)
	}
}

func TestMarketOrders(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeUser(t, "Seller A")
	uidB, tokenB := makeUser(t, "Buyer B")
	uidC, tokenC := makeUser(t, "Buyer C")

	st, resp, _ := doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "x"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Zero Price", "price_coins": 0, "price_mmk": 0}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{
			"title": "Sony Headphones", "price_coins": 350,
			"township": "Kamayut", "description": "WH-1000XM5",
		}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	productID := int64(resp["id"].(float64))

	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "MMK Item", "price_mmk": 5000, "township": "Bahan"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)

	st, resp, _ = doReq(t, "GET", "/api/v1/market/products", nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["products"].([]any)); got != 2 {
		t.Fatalf("products = %d want 2", got)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/market/products?township=Kamayut", nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["products"].([]any)); got != 1 {
		t.Fatalf("township filter = %d want 1", got)
	}

	// Insufficient balance.
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": productID}, tokenB)
	needStatus(t, st, http.StatusConflict, resp)
	topup(t, uidB, tokenB, 100, "req-20000001")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": productID}, tokenB)
	needStatus(t, st, http.StatusConflict, resp)

	// Own listing rejected.
	topup(t, uidA, tokenA, 1000, "req-20000002")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": productID}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Successful purchase moves coins and deactivates the listing.
	topup(t, uidB, tokenB, 350, "req-20000003")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": productID}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)
	if resp["balance"].(float64) != 100 {
		t.Fatalf("buyer balance after buy = %v want 100", resp["balance"])
	}
	order := resp["order"].(map[string]any)
	if order["status"] != "paid" {
		t.Fatalf("order status = %v", order["status"])
	}

	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": productID}, tokenC)
	needStatus(t, st, http.StatusConflict, resp)

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/balance", nil, tokenA)
	if resp["balance"].(float64) != 1350 {
		t.Fatalf("seller balance = %v want 1350", resp["balance"])
	}

	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": 999999}, tokenB)
	needStatus(t, st, http.StatusNotFound, resp)

	// Concurrency: two buyers race for the last affordable listing.
	topup(t, uidB, tokenB, 1000, "req-20000004")
	topup(t, uidC, tokenC, 1000, "req-20000005")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Raced Item", "price_coins": 500}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	racedID := int64(resp["id"].(float64))

	var wg sync.WaitGroup
	var mu sync.Mutex
	successes := 0
	for _, tok := range []string{tokenB, tokenC} {
		wg.Add(1)
		go func(tok string) {
			defer wg.Done()
			st, _, _ := doReq(t, "POST", "/api/v1/market/orders",
				map[string]any{"product_id": racedID}, tok)
			mu.Lock()
			defer mu.Unlock()
			if st == http.StatusCreated {
				successes++
			}
		}(tok)
	}
	wg.Wait()
	if successes != 1 {
		t.Fatalf("concurrent purchases succeeded %d times, want exactly 1", successes)
	}
}

func TestUnifiedChat(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeUser(t, "Chat A")
	uidB, tokenB := makeUser(t, "Chat B")
	_, tokenC := makeUser(t, "Chat C")

	st, resp, _ := doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Nike Air Jordan", "price_coins": 120, "township": "Kamayut"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	productID := int64(resp["id"].(float64))

	// Personal thread.
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "personal", "peer_id": uidB}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	personalID := int64(resp["id"].(float64))

	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "personal"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "personal", "peer_id": uidA}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "personal", "peer_id": 999999}, tokenA)
	needStatus(t, st, http.StatusNotFound, resp)

	// Business thread.
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "business"}, tokenB)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "business", "product_id": productID}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)
	businessID := int64(resp["id"].(float64))

	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "business", "product_id": 999999}, tokenB)
	needStatus(t, st, http.StatusNotFound, resp)

	st, resp, _ = doReq(t, "GET", "/api/v1/chat/threads", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["threads"].([]any)); got != 2 {
		t.Fatalf("threads for A = %d want 2", got)
	}

	// Messages.
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/chat/threads/%d/messages", personalID),
		map[string]any{"body": "မင်္ဂလာပါ။ ဘာကူညီပေးရမလဲခင်ဗျာ?"}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/chat/threads/%d/messages", personalID),
		map[string]any{"body": ""}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/chat/threads/%d/messages", businessID),
		map[string]any{"body": "Inquiry for Nike Air Jordan"}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)

	// Outsider blocked.
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/chat/threads/%d/messages", personalID),
		map[string]any{"body": "intruder"}, tokenC)
	needStatus(t, st, http.StatusForbidden, resp)

	st, resp, _ = doReq(t, "GET", fmt.Sprintf("/api/v1/chat/threads/%d/messages", personalID), nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["messages"].([]any)); got != 1 {
		t.Fatalf("messages = %d want 1", got)
	}

	// Unknown thread.
	st, resp, _ = doReq(t, "GET", "/api/v1/chat/threads/999999/messages", nil, tokenA)
	needStatus(t, st, http.StatusNotFound, resp)
}

func httptestReq(t *testing.T, method, path string, body any) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, mustJSON(t, body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

func record(t *testing.T, req *http.Request, token string) *httptest.ResponseRecorder {
	t.Helper()
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	return w
}

func TestDevTopupGate(t *testing.T) {
	resetDB(t)
	uid, token := makeUser(t, "Dev Topup User")

	// Dev build (OTP_DEBUG=1) allows unsigned dev top-ups.
	out := topupDev(t, uid, token, 2450, "req-30000001")
	if out["balance"].(float64) != 2450 {
		t.Fatalf("dev topup balance = %v", out["balance"])
	}
	out = topupDev(t, uid, token, 500, "req-30000001")
	if out["balance"].(float64) != 2450 {
		t.Fatalf("dev replay double-credited: %v", out)
	}

	// Production build (OTP_DEBUG=0) must return 404.
	prod, err := New(testStore, testJWT, config.Config{
		RateLimitPerMin:   100000,
		WhisperTTLSeconds: 5,
		PaymentSigningKey: "test-payment-key",
	})
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	req := httptestReq(t, "POST", "/api/v1/coins/topup/dev",
		map[string]any{"amount": 100, "request_id": "req-30000009"})
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	prod.Routes().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("prod dev-topup status = %d want 404", w.Code)
	}
}

func topupDev(t *testing.T, uid int64, token string, amount int64, requestID string) map[string]any {
	t.Helper()
	req := httptestReq(t, "POST", "/api/v1/coins/topup/dev",
		map[string]any{"amount": amount, "request_id": requestID})
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

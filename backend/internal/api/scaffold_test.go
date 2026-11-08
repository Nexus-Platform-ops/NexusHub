package api

// Table-driven scaffolds for the whisper (dating.go), market (market.go) and
// safety (moderation.go) surfaces, plus the cross-cutting error paths:
// JSON decode reject, rate-limit exceed, RLS deny, TTL expiry and webhook
// duplicate. Every case goes through the public HTTP surface.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nexus/backend/internal/payments"
	"nexus/backend/internal/store"
)

// doRaw posts an unvalidated body so JSON-decode-reject paths can be exercised.
func doRaw(t *testing.T, method, path, raw, token string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// makeMatch records a mutual dating match directly in the dating schema.
func makeMatch(t *testing.T, a, b int64) {
	t.Helper()
	ctx := context.Background()
	err := testStore.DatingConn(ctx, func(q store.Querier) error {
		_, err := q.Exec(ctx,
			`INSERT INTO dating.matches (a_id, b_id)
			 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))`, a, b)
		return err
	})
	if err != nil {
		t.Fatalf("makeMatch: %v", err)
	}
}

func TestWhisperSendTableDriven(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeAdultUser(t, "Whisper A")
	uidB, _ := makeAdultUser(t, "Whisper B")
	_, tokenC := makeAdultUser(t, "Whisper C No Match")
	makeMatch(t, uidA, uidB)

	// Independent pair where B blocks A before the send attempt.
	uidD, _ := makeAdultUser(t, "Whisper D Blocker")
	uidE, tokenE := makeAdultUser(t, "Whisper E Blocked")
	makeMatch(t, uidD, uidE)
	st, resp, _ := doReq(t, "POST", "/api/v1/blocks",
		map[string]any{"node_id": nodeIDEncode(uidD)}, tokenE)
	needStatus(t, st, http.StatusCreated, resp)

	cases := []struct {
		name  string
		token string
		body  string
		want  int
	}{
		{"happy", tokenA, fmt.Sprintf(`{"to":%q,"body":"shin htet a kyi?"}`, nodeIDEncode(uidB)), http.StatusCreated},
		{"empty body", tokenA, fmt.Sprintf(`{"to":%q,"body":"   "}`, nodeIDEncode(uidB)), http.StatusBadRequest},
		{"oversized body", tokenA, fmt.Sprintf(`{"to":%q,"body":%q}`, nodeIDEncode(uidB), strings.Repeat("a", 501)), http.StatusBadRequest},
		{"invalid to node id", tokenA, `{"to":"!!!","body":"hi"}`, http.StatusBadRequest},
		{"self whisper", tokenA, fmt.Sprintf(`{"to":%q,"body":"hi"}`, nodeIDEncode(uidA)), http.StatusBadRequest},
		{"no mutual match", tokenC, fmt.Sprintf(`{"to":%q,"body":"hi"}`, nodeIDEncode(uidB)), http.StatusForbidden},
		{"blocked pair", tokenE, fmt.Sprintf(`{"to":%q,"body":"hi"}`, nodeIDEncode(uidD)), http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "POST", "/api/v1/whisper", tc.body, tc.token)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject must fail before any state change.
	got, out := doRaw(t, "POST", "/api/v1/whisper", `{"to":`, tokenA)
	needStatus(t, got, http.StatusBadRequest, out)

	// Non-adult callers are rejected by the 18+ gate before body validation.
	_, tokenMinor := makeUser(t, "Whisper Minor")
	st, resp, _ = doReq(t, "PUT", "/api/v1/me/birthdate",
		map[string]any{"date_of_birth": "2012-01-01"}, tokenMinor)
	needStatus(t, st, http.StatusOK, resp)
	got, out = doRaw(t, "POST", "/api/v1/whisper",
		fmt.Sprintf(`{"to":%q,"body":"hi"}`, nodeIDEncode(uidB)), tokenMinor)
	needStatus(t, got, http.StatusForbidden, out)
}

func TestWhisperReadLifecycle(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeAdultUser(t, "Read A")
	uidB, tokenB := makeAdultUser(t, "Read B")
	_, tokenC := makeAdultUser(t, "Read C Eavesdropper")
	makeMatch(t, uidA, uidB)

	st, resp, _ := doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "one-time secret"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("whisper id missing: %v", resp)
	}

	// Eavesdropper read is a 403 AND burns the message (safety property).
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+id, nil, tokenC)
	needStatus(t, st, http.StatusForbidden, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+id, nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)

	// Fresh whisper: recipient reads once, second read is gone.
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": "for your eyes"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	id2, _ := resp["id"].(string)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+id2, nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["body"] != "for your eyes" || resp["deleted"] != true ||
		int64(resp["from"].(float64)) != uidA {
		t.Fatalf("read payload = %v", resp)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+id2, nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)

	// Unknown id.
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/0123456789abcdef0123456789abcdef", nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)
}

func TestWhisperTTLExpiry(t *testing.T) {
	resetDB(t)
	uidA, _ := makeAdultUser(t, "TTL A")
	uidB, tokenB := makeAdultUser(t, "TTL B")

	// Seed a whisper whose TTL has already lapsed: the Redis TTL is the
	// backstop when the read-once delete never happened.
	payload, _ := json.Marshal(whisperPayload{From: uidA, To: uidB, Body: "too late"})
	ctx := context.Background()
	if err := testStore.RDB.Set(ctx, "whisper:expiredseed", payload, 60*time.Millisecond).Err(); err != nil {
		t.Fatalf("seed whisper: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	st, resp, _ := doReq(t, "GET", "/api/v1/whisper/expiredseed", nil, tokenB)
	needStatus(t, st, http.StatusGone, resp)
}

func TestMarketProductsTableDriven(t *testing.T) {
	resetDB(t)
	_, tokenSeller := makeUser(t, "Market Seller")

	cases := []struct {
		name string
		body string
		want int
	}{
		{"valid coins listing", `{"title":"Yamaha keyboard","price_coins":250,"township":"Kyauktada"}`, http.StatusCreated},
		{"title too short", `{"title":"a","price_coins":1}`, http.StatusBadRequest},
		{"title too long", fmt.Sprintf(`{"title":%q,"price_coins":1}`, strings.Repeat("t", 121)), http.StatusBadRequest},
		{"negative price", `{"title":"bad price","price_coins":-5}`, http.StatusBadRequest},
		{"no price at all", `{"title":"free?","price_coins":0,"price_mmk":0}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "POST", "/api/v1/market/products", tc.body, tokenSeller)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject.
	got, out := doRaw(t, "POST", "/api/v1/market/products", `{"title":`, tokenSeller)
	needStatus(t, got, http.StatusBadRequest, out)

	// Listing shows the active product; the township filter narrows it.
	st, resp, _ := doReq(t, "GET", "/api/v1/market/products", nil, tokenSeller)
	needStatus(t, st, http.StatusOK, resp)
	products, _ := resp["products"].([]any)
	if len(products) != 1 {
		t.Fatalf("products = %v", resp)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/market/products?township=Kyauktada", nil, tokenSeller)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["products"].([]any)); got != 1 {
		t.Fatalf("township hit = %d want 1", got)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/market/products?township=Bahan", nil, tokenSeller)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["products"].([]any)); got != 0 {
		t.Fatalf("township miss = %d want 0", got)
	}
}

func TestMarketOrdersTableDriven(t *testing.T) {
	resetDB(t)
	_, tokenSeller := makeUser(t, "Order Seller")
	uidBuyer, tokenBuyer := makeUser(t, "Order Buyer")

	// Fund the buyer with a signed server-side top-up.
	if out := topup(t, uidBuyer, tokenBuyer, 100, "order-topup-1"); out["balance"].(float64) != 100 {
		t.Fatalf("topup = %v", out)
	}

	// Coins-priced listing, MMK-only listing, and a listing the buyer owns.
	st, resp, _ := doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Coin priced", "price_coins": 100}, tokenSeller)
	needStatus(t, st, http.StatusCreated, resp)
	coinProduct := int64(resp["id"].(float64))

	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "MMK only", "price_mmk": 45000}, tokenSeller)
	needStatus(t, st, http.StatusCreated, resp)
	mmkProduct := int64(resp["id"].(float64))

	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Buyer owned", "price_coins": 5}, tokenBuyer)
	needStatus(t, st, http.StatusCreated, resp)
	ownProduct := int64(resp["id"].(float64))

	cases := []struct {
		name string
		body string
		want int
	}{
		{"happy purchase", fmt.Sprintf(`{"product_id":%d}`, coinProduct), http.StatusCreated},
		{"product already sold", fmt.Sprintf(`{"product_id":%d}`, coinProduct), http.StatusConflict},
		{"unknown product", `{"product_id":987654321}`, http.StatusNotFound},
		{"missing product id", `{"product_id":0}`, http.StatusBadRequest},
		{"own listing", fmt.Sprintf(`{"product_id":%d}`, ownProduct), http.StatusBadRequest},
		{"mmk only listing", fmt.Sprintf(`{"product_id":%d}`, mmkProduct), http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "POST", "/api/v1/market/orders", tc.body, tokenBuyer)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject.
	got, out := doRaw(t, "POST", "/api/v1/market/orders", `{"product_id":`, tokenBuyer)
	needStatus(t, got, http.StatusBadRequest, out)

	// Insufficient balance: fresh listing at a price above the balance.
	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Premium", "price_coins": 500}, tokenSeller)
	needStatus(t, st, http.StatusCreated, resp)
	premium := int64(resp["id"].(float64))
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": premium}, tokenBuyer)
	needStatus(t, st, http.StatusConflict, resp)
	if resp["error"] != "insufficient coins" {
		t.Fatalf("insufficient error = %v", resp)
	}

	// Seller received the coins and the buyer's balance dropped.
	if got := balanceOf(t, tokenBuyer); got != 0 {
		t.Fatalf("buyer balance = %d want 0", got)
	}
	if got := balanceOf(t, tokenSeller); got != 100 {
		t.Fatalf("seller balance = %d want 100", got)
	}
}

func TestReportsTableDriven(t *testing.T) {
	resetDB(t)
	uidReporter, tokenReporter := makeUser(t, "Reporter")
	uidTarget, _ := makeUser(t, "Reported User")

	cases := []struct {
		name string
		body string
		want int
	}{
		{"user report", fmt.Sprintf(`{"target_kind":"user","target_id":%q,"reason":"scam"}`, nodeIDEncode(uidTarget)), http.StatusCreated},
		{"product report", `{"target_kind":"product","target_id":"42","reason":"spam","details":"fake goods"}`, http.StatusCreated},
		{"whisper report", `{"target_kind":"whisper","target_id":"0123456789abcdef","reason":"harassment"}`, http.StatusCreated},
		{"unknown target kind", fmt.Sprintf(`{"target_kind":"comment","target_id":%q,"reason":"spam"}`, nodeIDEncode(uidTarget)), http.StatusBadRequest},
		{"unknown reason", fmt.Sprintf(`{"target_kind":"user","target_id":%q,"reason":"hate"}`, nodeIDEncode(uidTarget)), http.StatusBadRequest},
		{"empty target id", `{"target_kind":"user","target_id":"   ","reason":"spam"}`, http.StatusBadRequest},
		{"self report", fmt.Sprintf(`{"target_kind":"user","target_id":%q,"reason":"other"}`, nodeIDEncode(uidReporter)), http.StatusBadRequest},
		{"unknown user target", fmt.Sprintf(`{"target_kind":"user","target_id":%q,"reason":"scam"}`, nodeIDEncode(999999999)), http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "POST", "/api/v1/reports", tc.body, tokenReporter)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject.
	got, out := doRaw(t, "POST", "/api/v1/reports", `{"target_kind":`, tokenReporter)
	needStatus(t, got, http.StatusBadRequest, out)
}

func TestBlocksTableDriven(t *testing.T) {
	resetDB(t)
	_, tokenA := makeUser(t, "Block A")
	uidB, _ := makeUser(t, "Block B")
	nodeB := nodeIDEncode(uidB)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"block user", fmt.Sprintf(`{"node_id":%q}`, nodeB), http.StatusCreated},
		{"block self", fmt.Sprintf(`{"node_id":%q}`, nodeIDEncode(mustUserID(t, tokenA))), http.StatusBadRequest},
		{"block unknown user", fmt.Sprintf(`{"node_id":%q}`, nodeIDEncode(999999999)), http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "POST", "/api/v1/blocks", tc.body, tokenA)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject.
	got, out := doRaw(t, "POST", "/api/v1/blocks", `{"node_id":`, tokenA)
	needStatus(t, got, http.StatusBadRequest, out)

	// List shows the block as an opaque node id.
	st, resp, _ := doReq(t, "GET", "/api/v1/blocks", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	blocks, _ := resp["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("blocks = %v", resp)
	}
	entry, _ := blocks[0].(map[string]any)
	if entry["node_id"] != nodeB {
		t.Fatalf("block node_id = %v want %v", entry["node_id"], nodeB)
	}

	// Unblock is idempotent.
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/"+nodeB, nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/"+nodeB, nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)

	// Malformed node id in the path.
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/!!!", nil, tokenA)
	needStatus(t, st, http.StatusNotFound, resp)
}

func mustUserID(t *testing.T, token string) int64 {
	t.Helper()
	st, resp, _ := doReq(t, "GET", "/api/v1/me", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	return int64(resp["id"].(float64))
}

func TestBirthDateGateTableDriven(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Birthdate User")

	cases := []struct {
		name string
		body string
		want int
	}{
		{"adult date", `{"date_of_birth":"1995-03-20"}`, http.StatusOK},
		{"underage rejected not stored", `{"date_of_birth":"2015-07-01"}`, http.StatusOK},
		{"wrong format", `{"date_of_birth":"20-03-1995"}`, http.StatusBadRequest},
		{"future date", fmt.Sprintf(`{"date_of_birth":%q}`, time.Now().Add(48*time.Hour).Format("2006-01-02")), http.StatusBadRequest},
		{"pre-1900", `{"date_of_birth":"1889-01-01"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, out := doRaw(t, "PUT", "/api/v1/me/birthdate", tc.body, token)
			needStatus(t, got, tc.want, out)
		})
	}

	// JSON decode reject.
	got, out := doRaw(t, "PUT", "/api/v1/me/birthdate", `{"date_of_birth":`, token)
	needStatus(t, got, http.StatusBadRequest, out)

	// A dedicated minor: the under-18 attempt must report adult=false and
	// must never persist the birth date.
	_, tokenMinor := makeUser(t, "Birthdate Minor")
	got, out = doRaw(t, "PUT", "/api/v1/me/birthdate", `{"date_of_birth":"2015-07-01"}`, tokenMinor)
	needStatus(t, got, http.StatusOK, out)
	if out["adult"] != false || out["stored"] != false {
		t.Fatalf("underage response = %v", out)
	}
	var birthDate any
	err := testStore.Social.QueryRow(context.Background(),
		`SELECT birth_date FROM social.users WHERE id=$1`, mustUserID(t, tokenMinor)).Scan(&birthDate)
	if err != nil {
		t.Fatalf("read birth_date: %v", err)
	}
	if birthDate != nil {
		t.Fatalf("minor birth_date persisted: %v", birthDate)
	}
}

// TestErrorPathsRateLimitTableDriven pins the 429 behavior of the IP limiter
// on the public login surface with a tight limit.
func TestErrorPathsRateLimitTableDriven(t *testing.T) {
	s := New(testStore, testJWT, sCfgWithLimit(2))
	h := s.Routes()

	steps := []struct {
		name string
		want int
	}{
		{"first request allowed", http.StatusNotFound},
		{"second request allowed", http.StatusNotFound},
		{"third request exceeds limit", http.StatusTooManyRequests},
	}
	for _, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/auth/login",
				strings.NewReader(`{"phone":"+95900000999"}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			needStatus(t, w.Code, step.want, nil)
		})
	}
}

// TestErrorPathsRLSDeny verifies the default-deny RLS posture on
// dating.dating_profiles: the dating role without the service flag sees zero
// rows and cannot write, and the social role has no path into the dating
// schema at all. (RLS is enforced on dating_profiles; the other dating
// tables rely on role isolation — see docs/ARCHITECTURE.md.)
func TestErrorPathsRLSDeny(t *testing.T) {
	resetDB(t)
	uid, token := makeAdultUser(t, "RLS User")
	makeDatingReady(t, uid, token, "rls_nick")
	ctx := context.Background()

	// Social role: zero privileges on the dating schema.
	var n int
	err := testStore.Social.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles`).Scan(&n)
	if err == nil {
		t.Fatal("social role could query dating schema — isolation broken")
	}

	// Dating role without the service flag: default-deny filters reads...
	conn, err := testStore.Dating.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles`).Scan(&n); err != nil {
		t.Fatalf("unflagged select errored instead of filtering: %v", err)
	}
	if n != 0 {
		t.Fatalf("RLS returned %d rows to an unflagged connection", n)
	}
	// ...and blocks writes outright.
	if _, err := conn.Exec(ctx,
		`INSERT INTO dating.dating_profiles (user_id, nickname) VALUES (999999999,'rls-probe')`); err == nil {
		conn.Release()
		t.Fatal("RLS did not block unflagged INSERT")
	}
	conn.Release()

	// The flagged path used by the API still works.
	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		return q.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles`).Scan(&n)
	}); err != nil {
		t.Fatalf("flagged dating query failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("flagged profiles = %d want 1", n)
	}
}

// postWebhookRaw signs an arbitrary raw body so malformed-JSON events still
// carry a valid HMAC (the decode reject must happen after signature verify).
func postWebhookRaw(t *testing.T, raw, secret string, ts time.Time, badSig bool) (int, map[string]any) {
	t.Helper()
	tsStr := fmt.Sprintf("%d", ts.Unix())
	sig := payments.SignWebhook(secret, tsStr, []byte(raw))
	if badSig {
		sig = "0000"
	}
	req := httptest.NewRequest("POST", "/api/v1/payments/webhook", strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Timestamp", tsStr)
	req.Header.Set("X-Payment-Signature", sig)
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestErrorPathsWebhookTableDriven(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Webhook Scaffold")
	st, resp := createIntent(t, token, "coins_100")
	needStatus(t, st, http.StatusCreated, resp)
	intentID := resp["intent_id"].(string)
	secret := testServer.Cfg.PaymentSigningKey

	cases := []struct {
		name  string
		body  string
		badTS bool
		badSig bool
		want  int
	}{
		{"bad signature", fmt.Sprintf(`{"intent_id":%q,"status":"paid","amount_mmk":2000}`, intentID), false, true, http.StatusUnauthorized},
		{"stale timestamp", fmt.Sprintf(`{"intent_id":%q,"status":"paid","amount_mmk":2000}`, intentID), true, false, http.StatusUnauthorized},
		{"malformed json body", `{"intent_id":`, false, false, http.StatusBadRequest},
		{"unknown intent", `{"intent_id":"0123456789abcdef0123456789abcdef","status":"paid","amount_mmk":2000}`, false, false, http.StatusNotFound},
		{"short intent id", `{"intent_id":"x","status":"paid","amount_mmk":2000}`, false, false, http.StatusBadRequest},
		{"unsupported status", fmt.Sprintf(`{"intent_id":%q,"status":"pending","amount_mmk":2000}`, intentID), false, false, http.StatusBadRequest},
		{"amount mismatch", fmt.Sprintf(`{"intent_id":%q,"status":"paid","amount_mmk":1}`, intentID), false, false, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := time.Now()
			if tc.badTS {
				ts = ts.Add(-10 * time.Minute)
			}
			got, out := postWebhookRaw(t, tc.body, secret, ts, tc.badSig)
			needStatus(t, got, tc.want, out)
		})
	}

	// Valid paid event credits once; the duplicate replay must be flagged
	// and must never double-credit.
	st, resp = postWebhookRaw(t,
		fmt.Sprintf(`{"intent_id":%q,"status":"paid","amount_mmk":2000,"provider_ref":"ch_scaffold"}`, intentID),
		secret, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	st, resp = postWebhookRaw(t,
		fmt.Sprintf(`{"intent_id":%q,"status":"paid","amount_mmk":2000,"provider_ref":"ch_scaffold"}`, intentID),
		secret, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	if resp["duplicate"] != true {
		t.Fatalf("duplicate replay = %v", resp)
	}
	if got := balanceOf(t, token); got != 100 {
		t.Fatalf("balance after duplicate = %d want 100", got)
	}
}

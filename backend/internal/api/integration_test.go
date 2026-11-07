package api

// Integration tests run against a real PostgreSQL (both isolated roles) and
// a real Redis instance. Bootstrap first: bash scripts/bootstrap.sh
// Every endpoint is exercised through the public HTTP surface so status
// codes and JSON schemas are verified end to end.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"nexus/backend/internal/auth"
	"nexus/backend/internal/config"
	"nexus/backend/internal/store"
)

var (
	testStore  *store.Store
	testJWT    *auth.JWT
	testServer *Server
	phoneSeq   int64
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	var err error
	testStore, err = store.New(ctx,
		envOr("TEST_SOCIAL_URL", "postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable"),
		envOr("TEST_DATING_URL", "postgres://nexus_dating:dev_dating@127.0.0.1:5432/nexus?sslmode=disable"),
		envOr("TEST_REDIS_ADDR", "127.0.0.1:6379"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "integration setup failed (run scripts/bootstrap.sh):", err)
		os.Exit(1)
	}
	testJWT = auth.New("test-secret-that-is-long-enough-32b")
	testServer = New(testStore, testJWT, config.Config{
		RateLimitPerMin:   100000,
		WhisperTTLSeconds: 5,
		OTPDebug:          true,
		PaymentSigningKey: "test-payment-key",
	})
	os.Exit(m.Run())
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func resetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		"DELETE FROM social.chat_messages",
		"DELETE FROM social.chat_threads",
		"DELETE FROM social.comments",
		"DELETE FROM social.reactions",
		"DELETE FROM social.posts",
		"DELETE FROM social.friendships",
		"DELETE FROM social.user_profiles",
		"DELETE FROM social.users",
		"DELETE FROM market.orders",
		"DELETE FROM market.products",
		"DELETE FROM ledger.payment_intents",
		"DELETE FROM ledger.coin_transactions",
	} {
		if _, err := testStore.Social.Exec(ctx, stmt); err != nil {
			t.Fatalf("reset %q: %v", stmt, err)
		}
	}
	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		for _, stmt := range []string{
			"DELETE FROM dating.swipes",
			"DELETE FROM dating.matches",
			"DELETE FROM dating.radar_presence",
			"DELETE FROM dating.dating_profiles",
		} {
			if _, err := q.Exec(ctx, stmt); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("reset dating: %v", err)
	}
	if err := testStore.RDB.FlushDB(ctx).Err(); err != nil {
		t.Fatalf("flush redis: %v", err)
	}
}

func doReq(t *testing.T, method, path string, body any, token string) (int, map[string]any, []byte) {
	t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out, w.Body.Bytes()
}

func needStatus(t *testing.T, got, want int, body map[string]any) {
	t.Helper()
	if got != want {
		t.Fatalf("status = %d, want %d (body: %v)", got, want, body)
	}
}

// makeUser runs the full OTP flow and returns (user id, access token).
func makeUser(t *testing.T, name string) (int64, string) {
	t.Helper()
	n := atomic.AddInt64(&phoneSeq, 1)
	phone := fmt.Sprintf("+959%08d", n)
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": phone, "display_name": name}, "")
	needStatus(t, st, http.StatusCreated, resp)
	devCode, _ := resp["dev_code"].(string)
	if devCode == "" {
		t.Fatal("OTP_DEBUG did not expose dev_code")
	}
	st2, resp2, _ := doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": devCode}, "")
	needStatus(t, st2, http.StatusOK, resp2)
	user, _ := resp2["user"].(map[string]any)
	uid, _ := user["id"].(float64)
	token, _ := resp2["access_token"].(string)
	return int64(uid), token
}

func addFriend(t *testing.T, a, b int64) {
	t.Helper()
	ctx := context.Background()
	for _, pair := range [][2]int64{{a, b}, {b, a}} {
		if _, err := testStore.Social.Exec(ctx,
			`INSERT INTO social.friendships (user_id, friend_id) VALUES ($1,$2)`,
			pair[0], pair[1]); err != nil {
			t.Fatalf("addFriend: %v", err)
		}
	}
}

func sigFor(uid, amount int64, requestID string) string {
	mac := hmac.New(sha256.New, []byte(testServer.Cfg.PaymentSigningKey))
	fmt.Fprintf(mac, "%d:%d:%s", uid, amount, requestID)
	return hex.EncodeToString(mac.Sum(nil))
}

// topup posts a signed coin top-up through the public handler.
func topup(t *testing.T, uid int64, token string, amount int64, requestID string) map[string]any {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/v1/coins/topup",
		mustJSON(t, map[string]any{"amount": amount, "request_id": requestID}))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Payment-Signature", sigFor(uid, amount, requestID))
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func mustJSON(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func TestHealth(t *testing.T) {
	resetDB(t)
	st, resp, _ := doReq(t, "GET", "/api/v1/health", nil, "")
	needStatus(t, st, http.StatusOK, resp)
	if resp["ok"] != true {
		t.Fatalf("health not ok: %v", resp)
	}
}

func TestAuthFlow(t *testing.T) {
	resetDB(t)

	st, _, _ := doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": "abc", "display_name": "Zaw"}, "")
	needStatus(t, st, http.StatusBadRequest, nil)
	st, _, _ = doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": "+959700000001", "display_name": "Z"}, "")
	needStatus(t, st, http.StatusBadRequest, nil)

	uid, token := makeUser(t, "Thandar Myint")

	// Wrong code rejected.
	phone := fmt.Sprintf("+959%08d", atomic.AddInt64(&phoneSeq, 1))
	doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": phone, "display_name": "Wrong Code User"}, "")
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": "000000"}, "")
	needStatus(t, st, http.StatusBadRequest, resp)

	// Authenticated surface.
	st, resp, _ = doReq(t, "GET", "/api/v1/me", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if int64(resp["id"].(float64)) != uid {
		t.Fatalf("me id = %v want %d", resp["id"], uid)
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/me", nil, "")
	needStatus(t, st, http.StatusUnauthorized, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/me", nil, "not-a-jwt")
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Login + refresh flow.
	st, loginResp, _ := doReq(t, "POST", "/api/v1/auth/login",
		map[string]any{"phone": phone, "display_name": "x"}, "")
	needStatus(t, st, http.StatusOK, loginResp)
	devCode, _ := loginResp["dev_code"].(string)
	_, verifyResp, _ := doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": devCode}, "")
	refreshToken := verifyResp["refresh_token"].(string)

	st, resp, _ = doReq(t, "POST", "/api/v1/auth/refresh",
		map[string]any{"refresh_token": refreshToken}, "")
	needStatus(t, st, http.StatusOK, resp)
	newAccess := resp["access_token"].(string)
	st, resp, _ = doReq(t, "GET", "/api/v1/me", nil, newAccess)
	needStatus(t, st, http.StatusOK, resp)

	// Access token must NOT be accepted as refresh.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/refresh",
		map[string]any{"refresh_token": newAccess}, "")
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Login for unknown user.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/login",
		map[string]any{"phone": "+95911111111"}, "")
	needStatus(t, st, http.StatusNotFound, resp)
}

func TestPostsAndComments(t *testing.T) {
	resetDB(t)
	_, tokenA := makeUser(t, "Kyaw Zin Aung")
	_, tokenB := makeUser(t, "Maung Maung")

	st, resp, _ := doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": "", "township": "Kamayut"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	st, resp, _ = doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": strings.Repeat("a", 2001)}, tokenA)
	needStatus(t, st, http.StatusBadRequest, resp)

	st, resp, _ = doReq(t, "POST", "/api/v1/posts",
		map[string]any{
			"content":  "ကမာရွတ်မြို့နယ် coffee shop ရောက်ခဲ့ပါတယ်",
			"township": "Kamayut",
		}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	postID := int64(resp["id"].(float64))
	if resp["author"] != "Kyaw Zin Aung" {
		t.Fatalf("author = %v", resp["author"])
	}

	st, respB, _ := doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": "Mandalay post", "township": "Chanayethazan"}, tokenB)
	needStatus(t, st, http.StatusCreated, respB)
	postB := int64(respB["id"].(float64))

	st, resp, _ = doReq(t, "GET", "/api/v1/posts", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["posts"].([]any)); got != 2 {
		t.Fatalf("feed length = %d want 2", got)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/posts?township=Kamayut", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["posts"].([]any)); got != 1 {
		t.Fatalf("township filter got %d want 1", got)
	}

	st, resp, _ = doReq(t, "GET", fmt.Sprintf("/api/v1/posts/%d", postID), nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/posts/999999", nil, tokenA)
	needStatus(t, st, http.StatusNotFound, resp)

	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/reactions", postID),
		map[string]any{"kind": "like"}, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["reactions"].(float64) != 1 || resp["liked"] != true {
		t.Fatalf("first like: %v", resp)
	}
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/reactions", postID),
		map[string]any{"kind": "like"}, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["reactions"].(float64) != 0 || resp["liked"] != false {
		t.Fatalf("unlike: %v", resp)
	}
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/reactions", postID),
		map[string]any{"kind": "love"}, tokenB)
	needStatus(t, st, http.StatusOK, resp)
	if resp["liked"] != true {
		t.Fatalf("switch kind: %v", resp)
	}
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/reactions", postID),
		map[string]any{"kind": "angry"}, tokenB)
	needStatus(t, st, http.StatusBadRequest, resp)

	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/comments", postID),
		map[string]any{"body": "မင်္ဂလာပါ"}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/comments", postID),
		map[string]any{"body": ""}, tokenB)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "GET", fmt.Sprintf("/api/v1/posts/%d/comments", postID), nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
	if got := len(resp["comments"].([]any)); got != 1 {
		t.Fatalf("comments = %d want 1", got)
	}
	st, resp, _ = doReq(t, "POST", fmt.Sprintf("/api/v1/posts/%d/comments", postB),
		map[string]any{"body": "hello"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
}

func TestProfileEndpoints(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Profile User")
	st, resp, _ := doReq(t, "PUT", "/api/v1/profile",
		map[string]any{"real_name": "Thandar Myint", "township": "Kamayut, Yangon"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["real_name"] != "Thandar Myint" {
		t.Fatalf("profile: %v", resp)
	}
	st, resp, _ = doReq(t, "PUT", "/api/v1/profile",
		map[string]any{"real_name": ""}, token)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/profile", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["township"] != "Kamayut, Yangon" {
		t.Fatalf("profile get: %v", resp)
	}
}

func TestProfileIsolation(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	_, tokenA := makeUser(t, "Isolation User")
	_, tokenB := makeUser(t, "Other User")
	makeAdult(t, tokenA)

	st, resp, _ := doReq(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": "SecretNick", "bio": "hidden bio", "is_hidden_from_friends": true}, tokenA)
	needStatus(t, st, http.StatusOK, resp)

	st, resp, _ = doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": "hello social world", "township": "Kamayut"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)

	st, _, rawBody := doReq(t, "GET", "/api/v1/posts", nil, tokenB)
	needStatus(t, st, http.StatusOK, nil)
	if strings.Contains(string(rawBody), "SecretNick") ||
		strings.Contains(string(rawBody), "hidden bio") ||
		strings.Contains(string(rawBody), "is_hidden_from_friends") {
		t.Fatalf("dating data leaked into social feed: %s", rawBody)
	}

	var n int
	err := testStore.Social.QueryRow(ctx,
		`SELECT count(*) FROM dating.dating_profiles`).Scan(&n)
	if err == nil {
		t.Fatal("social role could query dating schema — isolation broken")
	}

	// RLS: a dating-role connection without the service flag sees nothing
	// (default-deny → zero rows) and cannot INSERT either.
	conn, err := testStore.Dating.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	err = conn.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles`).Scan(&n)
	if err != nil {
		t.Fatalf("unflagged select errored instead of filtering: %v", err)
	}
	if n != 0 {
		t.Fatal("RLS did not hide rows from unflagged dating connection")
	}
	if _, err := conn.Exec(ctx,
		`INSERT INTO dating.dating_profiles (user_id, nickname) VALUES (1,'rls-probe')`); err == nil {
		conn.Release()
		t.Fatal("RLS did not block unflagged INSERT")
	}
	conn.Release()

	// With the service flag the API can read it.
	err = testStore.DatingConn(ctx, func(q store.Querier) error {
		return q.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles`).Scan(&n)
	})
	if err != nil {
		t.Fatalf("flagged dating query failed: %v", err)
	}
	if n != 1 {
		t.Fatalf("dating profiles = %d want 1", n)
	}
}

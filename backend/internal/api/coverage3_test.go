package api

// Failure-path coverage: with a closed store every handler must fail with
// a clean 500 (serverError), and health must report 503.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"nexus/backend/internal/auth"
	"nexus/backend/internal/store"
)

func TestServerErrorHelper(t *testing.T) {
	w := httptest.NewRecorder()
	serverError(w, "unit", fmt.Errorf("boom"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestClientIPExtraction(t *testing.T) {
	r := httptest.NewRequest("POST", "/x", nil)
	r.RemoteAddr = "10.1.2.3:5555"
	if got := clientIP(r); got != "10.1.2.3" {
		t.Fatalf("ip = %q", got)
	}
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := clientIP(r); got != "1.2.3.4" {
		t.Fatalf("xff ip = %q", got)
	}
	r.Header.Del("X-Forwarded-For")
	r.RemoteAddr = "no-port"
	if got := clientIP(r); got != "no-port" {
		t.Fatalf("fallback = %q", got)
	}
}

func TestHandlersReport500OnClosedStore(t *testing.T) {
	ctx := context.Background()
	s, err := store.New(ctx,
		envOr("TEST_SOCIAL_URL", "postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable"),
		envOr("TEST_DATING_URL", "postgres://nexus_dating:dev_dating@127.0.0.1:5432/nexus?sslmode=disable"),
		envOr("TEST_REDIS_ADDR", "127.0.0.1:6379"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	uid, token := makeUser(t, "Closed Store User")
	srv, err := New(s, auth.New("test-secret-that-is-long-enough-32b"), sCfgWithLimit(100000))
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	h := srv.Routes()
	s.Close()

	type step struct {
		method, path string
		body         any
	}
	reqs := []step{
		{"POST", "/api/v1/auth/register", map[string]any{"phone": "+95988888888", "display_name": "Xy"}},
		{"POST", "/api/v1/auth/login", map[string]any{"phone": "+95988888888"}},
		{"GET", "/api/v1/me", nil},
		{"GET", "/api/v1/profile", nil},
		{"PUT", "/api/v1/profile", map[string]any{"real_name": "Closed"}},
		{"POST", "/api/v1/posts", map[string]any{"content": "hi"}},
		{"GET", "/api/v1/posts", nil},
		{"GET", "/api/v1/posts/1", nil},
		{"POST", "/api/v1/posts/1/reactions", map[string]any{"kind": "like"}},
		{"POST", "/api/v1/posts/1/comments", map[string]any{"body": "x"}},
		{"GET", "/api/v1/posts/1/comments", nil},
		{"GET", "/api/v1/dating/profile", nil},
		{"PUT", "/api/v1/dating/profile", map[string]any{"nickname": "Closed"}},
		{"GET", "/api/v1/radar?lat=16.8&lng=96.15", nil},
		{"POST", "/api/v1/dating/swipe", map[string]any{"node_id": nodeIDEncode(1), "action": "like"}},
		{"POST", "/api/v1/whisper", map[string]any{"to": nodeIDEncode(1), "body": "hi"}},
		{"GET", "/api/v1/whisper/abc", nil},
		{"GET", "/api/v1/coins/balance", nil},
		{"GET", "/api/v1/coins/ledger", nil},
		{"POST", "/api/v1/coins/topup/dev", map[string]any{"amount": 100, "request_id": "req-closed1"}},
		{"GET", "/api/v1/market/products", nil},
		{"POST", "/api/v1/market/products", map[string]any{"title": "Closed", "price_coins": 5}},
		{"POST", "/api/v1/market/orders", map[string]any{"product_id": 1}},
		{"POST", "/api/v1/chat/threads", map[string]any{"mode": "personal", "peer_id": uid + 1}},
		{"GET", "/api/v1/chat/threads", nil},
		{"GET", "/api/v1/chat/threads/1/messages", nil},
		{"POST", "/api/v1/chat/threads/1/messages", map[string]any{"body": "x"}},
	}
	for _, r := range reqs {
		treq := httptest.NewRequest(r.method, r.path, mustJSON(t, r.body))
		treq.Header.Set("Content-Type", "application/json")
		treq.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, treq)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("%s %s = %d want 500", r.method, r.path, w.Code)
		}
	}

	// A signed top-up against the closed store must also fail cleanly.
	treq := httptest.NewRequest("POST", "/api/v1/coins/topup",
		mustJSON(t, map[string]any{"amount": 100, "request_id": "req-closed2"}))
	treq.Header.Set("Content-Type", "application/json")
	treq.Header.Set("Authorization", "Bearer "+token)
	treq.Header.Set("X-Payment-Signature", sigFor(uid, 100, "req-closed2"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, treq)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("signed topup on closed store = %d want 500", w.Code)
	}

	// Health must report 503 with both dependencies down.
	treq = httptest.NewRequest("GET", "/api/v1/health", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, treq)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("health on closed store = %d want 503", w.Code)
	}
}

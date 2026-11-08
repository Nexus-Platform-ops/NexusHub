package api

// Full-surface smoke coverage: every registered endpoint (44 routes) is
// exercised anonymously (routing + auth middleware) and authentically
// (handler reached, no unexpected 5xx). Extends the coverage3 closed-store
// pattern to the success/middleware tier.

import (
	"net/http"
	"strings"
	"testing"
)

// smokeCase describes one HTTP probe. want is the set of acceptable status
// codes; the route only needs to reach its handler deterministically.
type smokeCase struct {
	name  string
	token bool // use a valid adult caller token
	body  any
	want  []int
}

func (c smokeCase) allowed(code int) bool {
	for _, w := range c.want {
		if code == w {
			return true
		}
	}
	return false
}

func runSmoke(t *testing.T, method, path string, c smokeCase, token string) {
	t.Helper()
	var got int
	var out map[string]any
	if raw, ok := c.body.(rawBody); ok {
		got, out = doRaw(t, method, path, string(raw), token)
	} else {
		got, out, _ = doReq(t, method, path, c.body, token)
	}
	if !c.allowed(got) {
		t.Fatalf("%s %s = %d, want one of %v (body: %v)", method, path, got, c.want, out)
	}
}

type rawBody string

func TestSmokeAllEndpointsAnonymous(t *testing.T) {
	resetDB(t)

	// Public surface (IP-limited only).
	public := []struct {
		method, path string
		body         any
		want         []int
	}{
		{"GET", "/api/v1/health", nil, []int{http.StatusOK}},
		{"POST", "/api/v1/auth/register", map[string]any{"phone": "+95977700001", "display_name": "Smoke Anon"}, []int{http.StatusCreated}},
		{"POST", "/api/v1/auth/register", rawBody(`{"phone":`), []int{http.StatusBadRequest}},
		{"POST", "/api/v1/auth/verify", map[string]any{"phone": "+95977700001", "code": "000000"}, []int{http.StatusBadRequest}},
		{"POST", "/api/v1/auth/login", map[string]any{"phone": "+95977700099"}, []int{http.StatusNotFound}},
		{"POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": "x"}, []int{http.StatusUnauthorized}},
		{"GET", "/api/v1/auth/social/facebook/start", nil, []int{http.StatusServiceUnavailable, http.StatusFound}},
		{"GET", "/api/v1/auth/social/facebook/callback", nil, []int{http.StatusBadRequest, http.StatusServiceUnavailable}},
		{"POST", "/api/v1/auth/social/telegram/callback", map[string]any{}, []int{http.StatusBadRequest, http.StatusUnauthorized, http.StatusServiceUnavailable}},
		{"POST", "/api/v1/payments/webhook", map[string]any{}, []int{http.StatusUnauthorized}},
	}
	for _, c := range public {
		t.Run("anon "+c.method+" "+c.path, func(t *testing.T) {
			runSmoke(t, c.method, c.path, smokeCase{body: c.body, want: c.want}, "")
		})
	}

	// Every authenticated route must reject anonymous callers with 401.
	authed := [][2]string{
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/profile"},
		{"PUT", "/api/v1/profile"},
		{"PUT", "/api/v1/me/birthdate"},
		{"POST", "/api/v1/reports"},
		{"POST", "/api/v1/blocks"},
		{"GET", "/api/v1/blocks"},
		{"DELETE", "/api/v1/blocks/" + nodeIDEncode(1)},
		{"GET", "/api/v1/auth/social/links"},
		{"DELETE", "/api/v1/auth/social/link/facebook"},
		{"POST", "/api/v1/posts"},
		{"GET", "/api/v1/posts"},
		{"GET", "/api/v1/posts/1"},
		{"POST", "/api/v1/posts/1/reactions"},
		{"GET", "/api/v1/posts/1/comments"},
		{"POST", "/api/v1/posts/1/comments"},
		{"GET", "/api/v1/dating/profile"},
		{"PUT", "/api/v1/dating/profile"},
		{"GET", "/api/v1/radar?lat=16.8&lng=96.15"},
		{"POST", "/api/v1/dating/swipe"},
		{"POST", "/api/v1/whisper"},
		{"GET", "/api/v1/whisper/abc"},
		{"GET", "/api/v1/coins/balance"},
		{"POST", "/api/v1/coins/topup"},
		{"POST", "/api/v1/coins/topup/dev"},
		{"GET", "/api/v1/coins/ledger"},
		{"POST", "/api/v1/coins/intents"},
		{"GET", "/api/v1/coins/intents/0123456789abcdef0123456789abcdef"},
		{"GET", "/api/v1/market/products"},
		{"POST", "/api/v1/market/products"},
		{"POST", "/api/v1/market/orders"},
		{"POST", "/api/v1/chat/threads"},
		{"GET", "/api/v1/chat/threads"},
		{"GET", "/api/v1/chat/threads/1/messages"},
		{"POST", "/api/v1/chat/threads/1/messages"},
	}
	for _, r := range authed {
		t.Run("anon "+r[0]+" "+r[1], func(t *testing.T) {
			got, out := doRaw(t, r[0], r[1], `{}`, "")
			if got != http.StatusUnauthorized {
				t.Fatalf("%s %s = %d want 401 (body: %v)", r[0], r[1], got, out)
			}
		})
	}
}

func TestSmokeAllEndpointsAuthenticated(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeAdultUser(t, "Smoke User A")
	uidB, _ := makeAdultUser(t, "Smoke User B")
	nodeB := nodeIDEncode(uidB)

	cases := []struct {
		method, path string
		c            smokeCase
	}{
		{"GET", "/api/v1/me", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"GET", "/api/v1/profile", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"PUT", "/api/v1/profile", smokeCase{token: true, body: map[string]any{"real_name": "Smoke"}, want: []int{http.StatusOK, http.StatusBadRequest}}},
		{"PUT", "/api/v1/me/birthdate", smokeCase{token: true, body: map[string]any{"date_of_birth": "1991-01-02"}, want: []int{http.StatusOK}}},
		{"POST", "/api/v1/reports", smokeCase{token: true, body: map[string]any{"target_kind": "user", "target_id": nodeB, "reason": "spam"}, want: []int{http.StatusCreated}}},
		{"POST", "/api/v1/blocks", smokeCase{token: true, body: map[string]any{"node_id": nodeB}, want: []int{http.StatusCreated}}},
		{"GET", "/api/v1/blocks", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"DELETE", "/api/v1/blocks/" + nodeB, smokeCase{token: true, want: []int{http.StatusOK}}},
		{"GET", "/api/v1/auth/social/links", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"DELETE", "/api/v1/auth/social/link/facebook", smokeCase{token: true, want: []int{http.StatusOK, http.StatusNotFound}}},
		{"POST", "/api/v1/posts", smokeCase{token: true, body: map[string]any{"content": "smoke post"}, want: []int{http.StatusCreated, http.StatusBadRequest}}},
		{"GET", "/api/v1/posts", smokeCase{token: true, want: []int{http.StatusOK}}},
		// IDs that no test can create: existence failures must not depend on
		// whatever ID 1 happens to be in a fresh database.
		{"GET", "/api/v1/posts/987654321", smokeCase{token: true, want: []int{http.StatusNotFound}}},
		{"POST", "/api/v1/posts/987654321/reactions", smokeCase{token: true, body: map[string]any{"kind": "like"}, want: []int{http.StatusNotFound}}},
		{"GET", "/api/v1/posts/987654321/comments", smokeCase{token: true, want: []int{http.StatusNotFound, http.StatusOK}}},
		{"POST", "/api/v1/posts/987654321/comments", smokeCase{token: true, body: map[string]any{"body": "smoke"}, want: []int{http.StatusNotFound}}},
		{"GET", "/api/v1/dating/profile", smokeCase{token: true, want: []int{http.StatusOK, http.StatusNotFound}}},
		{"PUT", "/api/v1/dating/profile", smokeCase{token: true, body: map[string]any{"nickname": "smoke"}, want: []int{http.StatusOK}}},
		{"GET", "/api/v1/radar?lat=16.8&lng=96.15", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"POST", "/api/v1/dating/swipe", smokeCase{token: true, body: map[string]any{"node_id": nodeB, "action": "like"}, want: []int{http.StatusOK, http.StatusNotFound, http.StatusBadRequest}}},
		{"POST", "/api/v1/whisper", smokeCase{token: true, body: map[string]any{"to": nodeB, "body": "smoke whisper"}, want: []int{http.StatusForbidden}}},
		{"GET", "/api/v1/whisper/abc", smokeCase{token: true, want: []int{http.StatusGone}}},
		{"GET", "/api/v1/coins/balance", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"POST", "/api/v1/coins/topup", smokeCase{token: true, body: map[string]any{"amount": 50, "request_id": "smoke-topup-1"}, want: []int{http.StatusCreated, http.StatusUnauthorized, http.StatusBadRequest}}},
		{"POST", "/api/v1/coins/topup/dev", smokeCase{token: true, body: map[string]any{"amount": 50, "request_id": "smoke-topup-dev-1"}, want: []int{http.StatusCreated, http.StatusForbidden, http.StatusNotFound}}},
		{"GET", "/api/v1/coins/ledger", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"POST", "/api/v1/coins/intents", smokeCase{token: true, body: map[string]any{"package_id": "coins_100"}, want: []int{http.StatusCreated}}},
		{"GET", "/api/v1/coins/intents/0123456789abcdef0123456789abcdef", smokeCase{token: true, want: []int{http.StatusNotFound}}},
		{"GET", "/api/v1/market/products", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"POST", "/api/v1/market/products", smokeCase{token: true, body: map[string]any{"title": "Smoke item", "price_coins": 10}, want: []int{http.StatusCreated}}},
		{"POST", "/api/v1/market/orders", smokeCase{token: true, body: map[string]any{"product_id": 987654321}, want: []int{http.StatusNotFound, http.StatusBadRequest}}},
		{"POST", "/api/v1/chat/threads", smokeCase{token: true, body: map[string]any{"mode": "personal", "peer_id": uidB}, want: []int{http.StatusCreated, http.StatusNotFound, http.StatusBadRequest}}},
		{"GET", "/api/v1/chat/threads", smokeCase{token: true, want: []int{http.StatusOK}}},
		{"GET", "/api/v1/chat/threads/987654321/messages", smokeCase{token: true, want: []int{http.StatusNotFound}}},
		{"POST", "/api/v1/chat/threads/987654321/messages", smokeCase{token: true, body: map[string]any{"body": "smoke"}, want: []int{http.StatusNotFound}}},
	}
	for _, tc := range cases {
		t.Run("authed "+tc.method+" "+tc.path, func(t *testing.T) {
			token := tokenA
			if !tc.c.token {
				token = ""
			}
			runSmoke(t, tc.method, tc.path, tc.c, token)
		})
	}

	// The signed top-up needs its HMAC header; assert it credits properly.
	// (The dev top-up exercised above may already have credited this user.)
	if out := topup(t, uidA, tokenA, 50, "smoke-topup-1"); out["credited"].(float64) != 50 {
		t.Fatalf("signed topup = %v", out)
	}
}

func TestSmokeRoutingHygiene(t *testing.T) {
	_, token := makeAdultUser(t, "Hygiene User")

	cases := []struct {
		method, path string
		token        string
		want         int
	}{
		{"GET", "/api/v1/no-such-route", "", http.StatusNotFound},
		{"DELETE", "/api/v1/me", token, http.StatusMethodNotAllowed},
		{"PATCH", "/api/v1/posts", token, http.StatusMethodNotAllowed},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			got, out := doRaw(t, c.method, c.path, `{}`, c.token)
			if got != c.want {
				t.Fatalf("%s %s = %d want %d (body: %v)", c.method, c.path, got, c.want, out)
			}
		})
	}

	// Over-long path segments must not panic or 5xx.
	long := "/api/v1/whisper/" + strings.Repeat("a", 128)
	got, out := doRaw(t, "GET", long, ``, token)
	if got != http.StatusNotFound {
		t.Fatalf("long whisper id = %d want 404 (body: %v)", got, out)
	}
}

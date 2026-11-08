package api

// Fault injection: dependency failures must surface as clean errors, never
// panics or hangs.
//
//   - Redis down        → health 503, whisper send/read 500
//   - PostGIS/Postgres  → connect deadline surfaces from the pool layer;
//     a closed pool makes handlers fail with 500
//   - JWT expired       → access and refresh calls get 401

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"nexus/backend/internal/store"
)

func TestFaultRedisDown(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeAdultUser(t, "Fault Redis A")
	uidB, tokenB := makeAdultUser(t, "Fault Redis B")
	makeMatch(t, uidA, uidB)

	// A Redis client pointing at a port nothing listens on.
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:6390", DialTimeout: 300 * time.Millisecond})
	fs := &store.Store{Social: testStore.Social, Dating: testStore.Dating, RDB: dead}
	srv, err := New(fs, testJWT, sCfgWithLimit(100000))
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	h := srv.Routes()

	// Health names the failed dependency.
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("health with redis down = %d want 503", w.Code)
	}

	// Whisper send reaches Redis after the match check and fails cleanly.
	cases := []struct {
		name, method, path string
		body               any
		token              string
	}{
		{"send", "POST", "/api/v1/whisper", map[string]any{"to": nodeIDEncode(uidB), "body": "redis down?"}, tokenA},
		{"read", "GET", "/api/v1/whisper/" + strings.Repeat("ab", 8), nil, tokenB},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			treq := httptest.NewRequest(c.method, c.path, mustJSON(t, c.body))
			treq.Header.Set("Content-Type", "application/json")
			treq.Header.Set("Authorization", "Bearer "+c.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, treq)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("%s %s with redis down = %d want 500", c.method, c.path, w.Code)
			}
		})
	}
}

func TestFaultPostgresTimeout(t *testing.T) {
	// A listener that accepts but never speaks: the pool must surface a
	// deadline instead of hanging forever.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
		}
	}()

	hanging := "postgres://nexus_social:dev_social@" + ln.Addr().String() + "/nexus?sslmode=disable&connect_timeout=2"

	pool, err := store.OpenPool(context.Background(), hanging)
	if err != nil {
		t.Fatalf("open pool against hanging listener: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	if err := pool.Ping(ctx); err == nil {
		t.Fatal("ping against hanging listener succeeded")
	} else if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "timeout") &&
		!strings.Contains(err.Error(), "deadline") {
		t.Fatalf("ping error = %v, want a deadline/timeout", err)
	}
}

func TestFaultPostgresPoolClosed(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Fault PG User")

	// Real pool, then closed: every query errors immediately, so handlers
	// must take the serverError path (500), never panic.
	ctx := context.Background()
	socialURL := envOr("TEST_SOCIAL_URL",
		"postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable")
	datingURL := envOr("TEST_DATING_URL",
		"postgres://nexus_dating:dev_dating@127.0.0.1:5432/nexus?sslmode=disable")
	closedSocial, err := store.OpenPool(ctx, socialURL)
	if err != nil {
		t.Fatalf("open social pool: %v", err)
	}
	closedSocial.Close()
	closedDating, err := store.OpenPool(ctx, datingURL)
	if err != nil {
		t.Fatalf("open dating pool: %v", err)
	}
	closedDating.Close()

	fs := &store.Store{Social: closedSocial, Dating: closedDating, RDB: testStore.RDB}
	srv, err := New(fs, testJWT, sCfgWithLimit(100000))
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	h := srv.Routes()

	cases := []struct {
		name, method, path string
		body               any
	}{
		{"market list", "GET", "/api/v1/market/products", nil},
		{"market create", "POST", "/api/v1/market/products", map[string]any{"title": "Fault", "price_coins": 1}},
		{"market order", "POST", "/api/v1/market/orders", map[string]any{"product_id": 1}},
		{"report", "POST", "/api/v1/reports", map[string]any{"target_kind": "product", "target_id": "1", "reason": "spam"}},
		{"block", "POST", "/api/v1/blocks", map[string]any{"node_id": nodeIDEncode(1)}},
		{"birthdate", "PUT", "/api/v1/me/birthdate", map[string]any{"date_of_birth": "1990-01-01"}},
		{"profile", "PUT", "/api/v1/profile", map[string]any{"real_name": "Fault"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			treq := httptest.NewRequest(c.method, c.path, mustJSON(t, c.body))
			treq.Header.Set("Content-Type", "application/json")
			treq.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, treq)
			if w.Code != http.StatusInternalServerError {
				t.Fatalf("%s %s on closed pool = %d want 500", c.method, c.path, w.Code)
			}
		})
	}

	// Health reports db=down.
	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("health on closed pools = %d want 503", w.Code)
	}
}

func TestFaultJWTExpired(t *testing.T) {
	resetDB(t)
	uid, _ := makeUser(t, "Fault JWT User")

	expiredAccess, err := testJWT.Sign(uid, "access", -time.Minute)
	if err != nil {
		t.Fatalf("sign expired access: %v", err)
	}
	expiredRefresh, err := testJWT.Sign(uid, "refresh", -time.Minute)
	if err != nil {
		t.Fatalf("sign expired refresh: %v", err)
	}
	validAccess, err := testJWT.Sign(uid, "access", time.Minute)
	if err != nil {
		t.Fatalf("sign valid access: %v", err)
	}

	cases := []struct {
		name, method, path string
		body               map[string]any
		token              string
		want               int
	}{
		{"expired access on me", "GET", "/api/v1/me", nil, expiredAccess, http.StatusUnauthorized},
		{"expired access on market", "GET", "/api/v1/market/products", nil, expiredAccess, http.StatusUnauthorized},
		{"expired refresh", "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": expiredRefresh}, "", http.StatusUnauthorized},
		// Type confusion: a valid ACCESS token is not a refresh token.
		{"access token as refresh", "POST", "/api/v1/auth/refresh", map[string]any{"refresh_token": validAccess}, "", http.StatusUnauthorized},
		{"garbage token", "GET", "/api/v1/me", nil, "not.a.jwt", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, out, _ := doReq(t, c.method, c.path, c.body, c.token)
			if st != c.want {
				t.Fatalf("%s %s = %d want %d (body: %v)", c.method, c.path, st, c.want, out)
			}
		})
	}

	// Sanity: a valid token still works after the expired ones were rejected.
	st, out, _ := doReq(t, "GET", "/api/v1/me", nil, validAccess)
	needStatus(t, st, http.StatusOK, out)
}

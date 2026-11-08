package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nexus/backend/internal/config"
	"nexus/backend/internal/ratelimit"
)

// sCfgWithLimit builds a test config with a tight rate limit.
func sCfgWithLimit(n int) config.Config {
	return config.Config{
		RateLimitPerMin:   n,
		WhisperTTLSeconds: 5,
		OTPDebug:          true,
		PaymentSigningKey: "test-payment-key",
	}
}

func TestNodeIDRoundtrip(t *testing.T) {
	for _, id := range []int64{1, 42, 99999} {
		enc := nodeIDEncode(id)
		got, ok := nodeIDDecode(enc)
		if !ok || got != id {
			t.Fatalf("roundtrip %d -> %q -> %d,%v", id, enc, got, ok)
		}
	}
	for _, bad := range []string{"", "!!!", "YQ", "d0", "dabc"} {
		if _, ok := nodeIDDecode(bad); ok {
			t.Fatalf("nodeIDDecode accepted %q", bad)
		}
	}
}

func TestAnonymousHandleAndBuckets(t *testing.T) {
	h1 := anonymousHandle(1)
	h2 := anonymousHandle(2)
	if h1 == h2 || len(h1) != len("Bee-XXXX") {
		t.Fatalf("handles: %q vs %q", h1, h2)
	}
	cases := map[float64]string{0: "500m", 499: "500m", 500: "1km", 999: "1km", 1500: "2km"}
	for m, want := range cases {
		if got := distanceBucket(m); got != want {
			t.Fatalf("distanceBucket(%v) = %q want %q", m, got, want)
		}
	}
}

func TestTrimHelpers(t *testing.T) {
	if got := trimLimit("  ab  ", 10); got != "ab" {
		t.Fatalf("trimLimit = %q", got)
	}
	if got := trimLimit("abcdef", 3); got != "abc" {
		t.Fatalf("trimLimit cut = %q", got)
	}
	if trimSpace("\t hi \n") != "hi" {
		t.Fatal("trimSpace failed")
	}
}

func TestReadJSONInvalid(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/v1/posts", strings.NewReader("{invalid"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	var dst map[string]any
	if err := readJSON(w, req, &dst); err == nil {
		t.Fatal("expected error for invalid json")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestIsDigits(t *testing.T) {
	if !isDigits("123456") {
		t.Fatal("123456 should be digits")
	}
	if isDigits("12a456") || isDigits("") {
		t.Fatal("bad digits accepted")
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	s, err := New(testStore, testJWT, sCfgWithLimit(2))
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	h := s.Routes()
	statuses := []int{}
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("POST", "/api/v1/auth/login",
			strings.NewReader(`{"phone":"+95900000000"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		statuses = append(statuses, w.Code)
	}
	if statuses[0] == http.StatusTooManyRequests || statuses[1] == http.StatusTooManyRequests {
		t.Fatalf("limited too early: %v", statuses)
	}
	if statuses[2] != http.StatusTooManyRequests {
		t.Fatalf("third request = %d want 429", statuses[2])
	}
}

func TestLimiterUnit(t *testing.T) {
	l := ratelimit.New(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !l.Allow("k") {
			t.Fatalf("request %d denied", i+1)
		}
	}
	if l.Allow("k") {
		t.Fatal("limit not enforced")
	}
	l.Reset()
	if !l.Allow("k") {
		t.Fatal("reset failed")
	}
	fast := ratelimit.New(1, 20*time.Millisecond)
	if !fast.Allow("x") || fast.Allow("x") {
		t.Fatal("window limit broken")
	}
	time.Sleep(30 * time.Millisecond)
	if !fast.Allow("x") {
		t.Fatal("window did not reset")
	}
}

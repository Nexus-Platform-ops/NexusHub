package api

// Attestation integration tests: nonce challenge (single-use, 5-minute TTL),
// dev provider verification, and the 30-day attestation record.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestAttestationFlow(t *testing.T) {
	resetDB(t)
	uid, token := makeUser(t, "Attest User")

	// Attest without a nonce → 401 (no active or expired challenge).
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Mint a nonce.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	nonce, _ := resp["nonce"].(string)
	if nonce == "" || resp["expires_in"].(float64) != 300 {
		t.Fatalf("nonce response: %v", resp)
	}
	if _, ok := resp["expires_at"].(string); !ok {
		t.Fatalf("nonce response missing expires_at: %v", resp)
	}

	// A wrong token invalidates the nonce and fails closed.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "bogus"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)

	// The consumed nonce cannot be replayed (401: start a fresh challenge).
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Fresh nonce + valid dev token → record, attested: true.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["device"] != "pixel8" || resp["provider"] != "log" || resp["verdict"] != "dev-ok" {
		t.Fatalf("attest response: %v", resp)
	}
	if resp["attested"] != true {
		t.Fatalf("attest response missing attested:true: %v", resp)
	}

	// Record kept for 30 days under the user's id.
	recordKey := fmt.Sprintf("attest:record:%d", uid)
	ttl := testStore.RDB.TTL(context.Background(), recordKey).Val()
	if ttl <= 29*24*time.Hour || ttl > 30*24*time.Hour {
		t.Fatalf("attest record TTL = %v, want ~30d", ttl)
	}
}

// TestAttestNonceRateLimit caps nonce issues at 5/hour/device: the sixth
// consecutive issue answers 429 without touching Redis.
func TestAttestNonceRateLimit(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Attest RateLimit User")
	for i := 0; i < 5; i++ {
		st, resp, _ := doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
		needStatus(t, st, http.StatusOK, resp)
	}
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusTooManyRequests, resp)
}

// TestAttestVerifyRoute covers the canonical /verify path: the client may
// send the nonce back for a cross-check; a mismatch (or a replayed/expired
// nonce) consumes the challenge and answers 401.
func TestAttestVerifyRoute(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Attest Verify User")

	// Expired nonce (key gone, same as TTL expiry) → 401.
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/attest/verify",
		map[string]any{"nonce": "stale", "platform_token": "dev:iphone15"}, token)
	needStatus(t, st, http.StatusUnauthorized, resp)

	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	nonce, _ := resp["nonce"].(string)

	// Nonce cross-check mismatch → 401 and the challenge is consumed.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/verify",
		map[string]any{"nonce": "not-the-issued-nonce", "platform_token": "dev:iphone15"}, token)
	needStatus(t, st, http.StatusUnauthorized, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/verify",
		map[string]any{"nonce": nonce, "platform_token": "dev:iphone15"}, token)
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Fresh nonce + matching cross-check + platform_token → attested.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	nonce, _ = resp["nonce"].(string)
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/verify",
		map[string]any{"nonce": nonce, "platform_token": "dev:iphone15"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["attested"] != true || resp["device"] != "iphone15" {
		t.Fatalf("verify response: %v", resp)
	}
}

// TestDiscoveryAttestationGate: with a real provider configured, radar and
// swipe answer 403 until an attestation record exists, and pass once it
// does. With the development "log" provider the gate is a no-op.
func TestDiscoveryAttestationGate(t *testing.T) {
	resetDB(t)
	uid, token := makeAdultUser(t, "Gate User")

	// Development provider: gate is a no-op even with no record.
	st, resp, _ := doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, token)
	needStatus(t, st, http.StatusOK, resp)

	// Real provider, no record → 403 on both discovery endpoints.
	orig := testServer.Cfg.AttestationProvider
	testServer.Cfg.AttestationProvider = "playintegrity"
	defer func() { testServer.Cfg.AttestationProvider = orig }()

	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, token)
	needStatus(t, st, http.StatusForbidden, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/dating/swipe",
		map[string]any{"node_id": "dzz", "action": "like"}, token)
	needStatus(t, st, http.StatusForbidden, resp)

	// Fresh record → gate passes.
	if err := testStore.RDB.Set(context.Background(),
		attestRecordKey(uid), `{"provider":"playintegrity"}`, attestRecordTTL).Err(); err != nil {
		t.Fatalf("seed record: %v", err)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, token)
	needStatus(t, st, http.StatusOK, resp)
}

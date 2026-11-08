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

	// Attest without a nonce → 400.
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Mint a nonce.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	nonce, _ := resp["nonce"].(string)
	if nonce == "" || resp["expires_in"].(float64) != 300 {
		t.Fatalf("nonce response: %v", resp)
	}

	// A wrong token invalidates the nonce and fails closed.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "bogus"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)

	// The consumed nonce cannot be replayed.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Fresh nonce + valid dev token → record.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest/nonce", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/attest",
		map[string]any{"token": "dev:pixel8"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["device"] != "pixel8" || resp["provider"] != "log" || resp["verdict"] != "dev-ok" {
		t.Fatalf("attest response: %v", resp)
	}

	// Record kept for 30 days under the user's id.
	recordKey := fmt.Sprintf("attest:record:%d", uid)
	ttl := testStore.RDB.TTL(context.Background(), recordKey).Val()
	if ttl <= 29*24*time.Hour || ttl > 30*24*time.Hour {
		t.Fatalf("attest record TTL = %v, want ~30d", ttl)
	}
}

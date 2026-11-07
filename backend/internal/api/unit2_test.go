package api

// Edge-case coverage: validation branches, malformed path values and
// bounds that the happy-path tests do not reach.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"nexus/backend/internal/store"
)

func TestEdgeCaseValidation(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	uidA, tokenA := makeUser(t, "Edge A")
	uidB, tokenB := makeUser(t, "Edge B")
	makeAdult(t, tokenA)
	makeAdult(t, tokenB)

	// Unknown JSON fields are rejected (strict decoding).
	st, _, _ := doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": "hi", "hacked_field": "x"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Malformed path id.
	st, _, _ = doReq(t, "GET", "/api/v1/posts/abc", nil, tokenA)
	needStatus(t, st, http.StatusNotFound, nil)

	// Top-up bounds.
	st, _, _ = doReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 2000000, "request_id": "req-40000001"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Negative product price.
	st, _, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Neg", "price_coins": -5}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Refresh with empty token.
	st, _, _ = doReq(t, "POST", "/api/v1/auth/refresh",
		map[string]any{"refresh_token": ""}, "")
	needStatus(t, st, http.StatusUnauthorized, nil)

	// Whisper body over the limit.
	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		_, err := q.Exec(ctx,
			`INSERT INTO dating.matches (a_id, b_id)
			 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))`,
			uidA, uidB)
		return err
	}); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	st, _, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": strings.Repeat("a", 501)}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Radar radius is clamped, not rejected.
	st, resp, _ := doReq(t, "GET",
		"/api/v1/radar?lat=16.8&lng=96.15&radius=99999", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)

	// Whisper id path bounds.
	st, _, _ = doReq(t, "GET", "/api/v1/whisper/"+strings.Repeat("x", 80), nil, tokenA)
	needStatus(t, st, http.StatusNotFound, nil)

	// Dev top-up validation ("dev-" prefix keeps the id below minimum).
	st, _, _ = doReq(t, "POST", "/api/v1/coins/topup/dev",
		map[string]any{"amount": 500, "request_id": ""}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Dating bio over the limit.
	st, _, _ = doReq(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": "EdgeNick", "bio": strings.Repeat("b", 501)}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Empty ledger list.
	st, resp, _ = doReq(t, "GET", "/api/v1/coins/ledger", nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)

	// Invalid chat mode.
	st, _, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "family"}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Feed limit clamping.
	st, _, _ = doReq(t, "GET", "/api/v1/posts?limit=500", nil, tokenA)
	needStatus(t, st, http.StatusOK, nil)

	// Order requires a positive product id.
	st, _, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": 0}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)

	// Over-long top-up request id.
	st, _, _ = doReq(t, "POST", "/api/v1/coins/topup",
		map[string]any{"amount": 100, "request_id": strings.Repeat("r", 100)}, tokenA)
	needStatus(t, st, http.StatusBadRequest, nil)
}

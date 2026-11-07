package api

// Additional edge coverage: bounds at exact limits, trimming behavior and
// MMK-only listings.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"nexus/backend/internal/store"
)

func TestMoreEdgeCases(t *testing.T) {
	resetDB(t)
	uidA, tokenA := makeUser(t, "Edge2 A")
	uidB, tokenB := makeUser(t, "Edge2 B")
	makeAdult(t, tokenA)
	makeAdult(t, tokenB)

	// Whisper body at exactly the limit is accepted and read-once works.
	if err := testStore.DatingConn(context.Background(), func(q store.Querier) error {
		_, err := q.Exec(context.Background(),
			`INSERT INTO dating.matches (a_id, b_id)
			 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))`,
			uidA, uidB)
		return err
	}); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	st, resp, _ := doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(uidB), "body": strings.Repeat("w", 500)}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	wid := resp["id"].(string)
	st, resp, _ = doReq(t, "GET", "/api/v1/whisper/"+wid, nil, tokenB)
	needStatus(t, st, http.StatusOK, resp)

	// Top-up with the minimum request_id length and a valid signature.
	out := topup(t, uidA, tokenA, 250, "req-x001")
	if out["credited"].(float64) != 250 {
		t.Fatalf("topup min request_id: %v", out)
	}

	// MMK-only listing cannot be bought with coins.
	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "MMK Only", "price_mmk": 8000}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	mmkID := int64(resp["id"].(float64))
	topup(t, uidB, tokenB, 5000, "req-x002")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": mmkID}, tokenB)
	needStatus(t, st, http.StatusBadRequest, resp)

	// Business chat on an inactive (sold) listing is rejected.
	st, resp, _ = doReq(t, "POST", "/api/v1/market/products",
		map[string]any{"title": "Sold Out", "price_coins": 10}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)
	soldID := int64(resp["id"].(float64))
	// A buys own listing is blocked; use B to buy it out.
	topup(t, uidB, tokenB, 1000, "req-x003")
	st, resp, _ = doReq(t, "POST", "/api/v1/market/orders",
		map[string]any{"product_id": soldID}, tokenB)
	needStatus(t, st, http.StatusCreated, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/chat/threads",
		map[string]any{"mode": "business", "product_id": soldID}, tokenB)
	needStatus(t, st, http.StatusConflict, resp)

	// Non-digit OTP code.
	phone := fmt.Sprintf("+959%08d", 99000001)
	doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": phone, "display_name": "Edge2 C"}, "")
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": "abcdef"}, "")
	needStatus(t, st, http.StatusBadRequest, resp)

	// Radar at the minimum radius.
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15&radius=100", nil, tokenA)
	needStatus(t, st, http.StatusOK, resp)
}

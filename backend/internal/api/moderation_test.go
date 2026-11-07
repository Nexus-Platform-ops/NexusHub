package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"nexus/backend/internal/store"
)

func makeAdult(t *testing.T, token string) {
	t.Helper()
	st, resp, _ := doReq(t, "PUT", "/api/v1/me/birthdate",
		map[string]any{"date_of_birth": "1995-04-12"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["adult"] != true || resp["stored"] != true {
		t.Fatalf("adult birth date not stored: %v", resp)
	}
}

func makeAdultUser(t *testing.T, name string) (int64, string) {
	t.Helper()
	uid, token := makeUser(t, name)
	makeAdult(t, token)
	return uid, token
}

func makeDatingReady(t *testing.T, uid int64, token, nickname string) {
	t.Helper()
	st, resp, _ := doReq(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": nickname, "bio": "test"}, token)
	needStatus(t, st, http.StatusOK, resp)
	// Radar registration also proves the caller's presence row is written.
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, token)
	needStatus(t, st, http.StatusOK, resp)
}

func nodeIDSet(t *testing.T, resp map[string]any) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	nodes, _ := resp["nodes"].([]any)
	for _, n := range nodes {
		m, ok := n.(map[string]any)
		if !ok {
			continue
		}
		if id, ok := m["node_id"].(string); ok {
			out[id] = true
		}
	}
	return out
}

func TestAgeGateRequires18Plus(t *testing.T) {
	resetDB(t)
	uid, token := makeUser(t, "Minor Test")

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{"GET", "/api/v1/dating/profile", nil},
		{"PUT", "/api/v1/dating/profile", map[string]any{"nickname": "Minor"}},
		{"GET", "/api/v1/radar?lat=16.8&lng=96.15", nil},
		{"POST", "/api/v1/dating/swipe", map[string]any{"node_id": nodeIDEncode(999), "action": "like"}},
		{"POST", "/api/v1/whisper", map[string]any{"to": nodeIDEncode(999), "body": "hi"}},
		{"GET", "/api/v1/whisper/abc", nil},
	} {
		st, resp, _ := doReq(t, tc.method, tc.path, tc.body, token)
		needStatus(t, st, http.StatusForbidden, resp)
	}

	// Under-18: rejected and NOT persisted.
	st, resp, _ := doReq(t, "PUT", "/api/v1/me/birthdate",
		map[string]any{"date_of_birth": "2015-01-01"}, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["adult"] != false || resp["stored"] != false {
		t.Fatalf("minor birth date must not be stored: %v", resp)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/dating/profile", nil, token)
	needStatus(t, st, http.StatusForbidden, resp)

	// Invalid / future dates are rejected outright.
	for _, bad := range []string{"not-a-date", "2999-01-01", "1800-01-01"} {
		st, resp, _ := doReq(t, "PUT", "/api/v1/me/birthdate",
			map[string]any{"date_of_birth": bad}, token)
		needStatus(t, st, http.StatusBadRequest, resp)
	}

	// 18+ date unlocks the dating surface (404 = gate passed, no profile yet).
	st, resp, _ = doReq(t, "PUT", "/api/v1/me/birthdate",
		map[string]any{"date_of_birth": "1995-04-12"}, token)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/dating/profile", nil, token)
	needStatus(t, st, http.StatusNotFound, resp)

	st, resp, _ = doReq(t, "GET", "/api/v1/me", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["adult"] != true || resp["id"] != float64(uid) {
		t.Fatalf("/me adult flag wrong: %v", resp)
	}
}

func TestBlockSuppressesRadarAndWhisper(t *testing.T) {
	resetDB(t)
	type account struct {
		uid   int64
		token string
	}
	names := []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo"}
	users := make([]account, 0, len(names))
	for _, n := range names {
		uid, token := makeAdultUser(t, n)
		makeDatingReady(t, uid, token, n+" Bee")
		users = append(users, account{uid, token})
	}
	a, b := users[0], users[1]

	st, resp, _ := doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, a.token)
	needStatus(t, st, http.StatusOK, resp)
	if !nodeIDSet(t, resp)[nodeIDEncode(b.uid)] {
		t.Fatalf("radar should include Bravo before block: %v", resp)
	}

	st, resp, _ = doReq(t, "POST", "/api/v1/blocks",
		map[string]any{"node_id": nodeIDEncode(b.uid)}, a.token)
	needStatus(t, st, http.StatusCreated, resp)

	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, a.token)
	needStatus(t, st, http.StatusOK, resp)
	if nodeIDSet(t, resp)[nodeIDEncode(b.uid)] {
		t.Fatalf("radar still exposes blocked user: %v", resp)
	}
	// Blocks are symmetric for visibility.
	st, resp, _ = doReq(t, "GET", "/api/v1/radar?lat=16.8&lng=96.15", nil, b.token)
	needStatus(t, st, http.StatusOK, resp)
	if nodeIDSet(t, resp)[nodeIDEncode(a.uid)] {
		t.Fatalf("blocker still appears on blocked user's radar: %v", resp)
	}

	// Even an existing mutual match cannot bypass a block.
	ctx := context.Background()
	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		_, err := q.Exec(ctx,
			`INSERT INTO dating.matches (a_id, b_id)
			 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))
			 ON CONFLICT DO NOTHING`, a.uid, b.uid)
		return err
	}); err != nil {
		t.Fatalf("seed match: %v", err)
	}
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(b.uid), "body": "hello"}, a.token)
	needStatus(t, st, http.StatusForbidden, resp)
	if resp["error"] != "user is blocked" {
		t.Fatalf("whisper block error = %v", resp["error"])
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/blocks", nil, a.token)
	needStatus(t, st, http.StatusOK, resp)
	blocks, _ := resp["blocks"].([]any)
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %v", resp)
	}
	if m, _ := blocks[0].(map[string]any); m["node_id"] != nodeIDEncode(b.uid) {
		t.Fatalf("block list node mismatch: %v", blocks[0])
	}

	// Unblock restores reachability (match must be re-seeded because the
	// block removed it).
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/"+nodeIDEncode(b.uid), nil, a.token)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/"+nodeIDEncode(b.uid), nil, a.token)
	needStatus(t, st, http.StatusOK, resp)

	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		_, err := q.Exec(ctx,
			`INSERT INTO dating.matches (a_id, b_id)
			 VALUES (LEAST($1::bigint,$2::bigint), GREATEST($1::bigint,$2::bigint))
			 ON CONFLICT DO NOTHING`, a.uid, b.uid)
		return err
	}); err != nil {
		t.Fatalf("re-seed match: %v", err)
	}
	st, resp, _ = doReq(t, "POST", "/api/v1/whisper",
		map[string]any{"to": nodeIDEncode(b.uid), "body": "hello again"}, a.token)
	needStatus(t, st, http.StatusCreated, resp)

	// Invalid block requests.
	for _, tc := range []struct {
		body any
		want int
	}{
		{map[string]any{"node_id": nodeIDEncode(a.uid)}, http.StatusBadRequest},
		{map[string]any{"node_id": "not-a-node"}, http.StatusBadRequest},
		{map[string]any{"node_id": nodeIDEncode(987654321)}, http.StatusNotFound},
	} {
		st, resp, _ := doReq(t, "POST", "/api/v1/blocks", tc.body, a.token)
		needStatus(t, st, tc.want, resp)
	}
	st, resp, _ = doReq(t, "DELETE", "/api/v1/blocks/not-a-node", nil, a.token)
	needStatus(t, st, http.StatusNotFound, resp)
}

func TestReports(t *testing.T) {
	resetDB(t)
	uid, token := makeUser(t, "Reporter")
	targetUID, _ := makeUser(t, "Target")

	st, resp, _ := doReq(t, "POST", "/api/v1/reports", map[string]any{
		"target_kind": "user", "target_id": nodeIDEncode(targetUID),
		"reason": "harassment", "details": "sends threats",
	}, token)
	needStatus(t, st, http.StatusCreated, resp)
	if resp["status"] != "open" {
		t.Fatalf("report status = %v", resp["status"])
	}
	reportID := int64(resp["id"].(float64))
	var storedTarget string
	if err := testStore.Social.QueryRow(context.Background(),
		`SELECT target_id FROM social.reports WHERE id=$1`, reportID).Scan(&storedTarget); err != nil {
		t.Fatalf("read report: %v", err)
	}
	if storedTarget != strconv.FormatInt(targetUID, 10) {
		t.Fatalf("user report stored target %q, want %d", storedTarget, targetUID)
	}

	// Post reports keep the raw numeric string.
	st, resp, _ = doReq(t, "POST", "/api/v1/reports", map[string]any{
		"target_kind": "post", "target_id": "123", "reason": "SPAM",
	}, token)
	needStatus(t, st, http.StatusCreated, resp)

	for _, tc := range []struct {
		body any
		want int
	}{
		{map[string]any{"target_kind": "planet", "target_id": "1", "reason": "spam"}, http.StatusBadRequest},
		{map[string]any{"target_kind": "post", "target_id": "1", "reason": "boredom"}, http.StatusBadRequest},
		{map[string]any{"target_kind": "post", "target_id": "", "reason": "spam"}, http.StatusBadRequest},
		{map[string]any{"target_kind": "user", "target_id": nodeIDEncode(uid), "reason": "spam"}, http.StatusBadRequest},
		{map[string]any{"target_kind": "user", "target_id": nodeIDEncode(999999), "reason": "spam"}, http.StatusNotFound},
	} {
		st, resp, _ := doReq(t, "POST", "/api/v1/reports", tc.body, token)
		needStatus(t, st, tc.want, resp)
	}

	st, resp, _ = doReq(t, "POST", "/api/v1/reports",
		map[string]any{"target_kind": "post", "target_id": "1", "reason": "spam"}, "")
	needStatus(t, st, http.StatusUnauthorized, resp)
}

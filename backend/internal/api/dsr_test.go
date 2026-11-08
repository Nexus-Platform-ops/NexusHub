package api

// DSR integration tests: export (async job, encrypted at rest, ownership
// enforced), and confirmed deletion (single-use token, anonymization).

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"nexus/backend/internal/store"
)

// pollDSRStatus polls the status endpoint until the job leaves "processing".
func pollDSRStatus(t *testing.T, token, jobID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, resp, _ := doReq(t, "GET", "/api/v1/dsr/status?job_id="+jobID, nil, token)
		needStatus(t, st, http.StatusOK, resp)
		if resp["status"] != "processing" {
			return resp
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("DSR job %s still processing after 5s", jobID)
	return nil
}

func TestDSRExportFlow(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	_, tokenA := makeUser(t, "Export User")
	_, tokenB := makeUser(t, "Bystander")

	st, resp, _ := doReq(t, "PUT", "/api/v1/profile",
		map[string]any{"real_name": "Thandar Export", "township": "Kamayut"}, tokenA)
	needStatus(t, st, http.StatusOK, resp)

	const marker = "EXPORT_MARKER_POST_77"
	st, resp, _ = doReq(t, "POST", "/api/v1/posts",
		map[string]any{"content": marker, "township": "Kamayut"}, tokenA)
	needStatus(t, st, http.StatusCreated, resp)

	// Request the export.
	st, resp, _ = doReq(t, "GET", "/api/v1/dsr/export", nil, tokenA)
	needStatus(t, st, http.StatusAccepted, resp)
	jobID, _ := resp["job_id"].(string)
	if jobID == "" || resp["status"] != "processing" {
		t.Fatalf("export request: %v", resp)
	}

	// Another user cannot see or download the job.
	st, resp, _ = doReq(t, "GET", "/api/v1/dsr/status?job_id="+jobID, nil, tokenB)
	needStatus(t, st, http.StatusNotFound, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/dsr/export/"+jobID, nil, tokenB)
	needStatus(t, st, http.StatusNotFound, resp)

	// Poll until done, then download.
	final := pollDSRStatus(t, tokenA, jobID)
	if final["status"] != "done" {
		t.Fatalf("export job failed: %v", final)
	}
	st, _, raw := doReq(t, "GET", "/api/v1/dsr/export/"+jobID, nil, tokenA)
	needStatus(t, st, http.StatusOK, nil)
	if !strings.Contains(string(raw), marker) ||
		!strings.Contains(string(raw), "Thandar Export") {
		t.Fatalf("export missing seeded data: %s", raw)
	}

	// The Redis copy is the envelope, not plaintext.
	stored, err := testStore.RDB.Get(ctx, "dsr:export:"+jobID).Result()
	if err != nil {
		t.Fatalf("export blob missing: %v", err)
	}
	if strings.Contains(stored, marker) || !strings.Contains(stored, `"ct"`) {
		t.Fatalf("export stored as plaintext or malformed: %s", stored)
	}

	// A completed job no longer blocks a new request.
	st, resp, _ = doReq(t, "GET", "/api/v1/dsr/export", nil, tokenA)
	needStatus(t, st, http.StatusAccepted, resp)
}

func TestDSRDeleteFlow(t *testing.T) {
	resetDB(t)
	ctx := context.Background()
	uid, token := makeAdultUser(t, "Delete User")
	makeDatingReady(t, uid, token, "DeleteNick") // dating profile + radar presence

	st, resp, _ := doReq(t, "PUT", "/api/v1/profile",
		map[string]any{"real_name": "Secret Realname"}, token)
	needStatus(t, st, http.StatusOK, resp)

	// Request deletion → token via dev parity (OTP_DEBUG).
	st, resp, _ = doReq(t, "DELETE", "/api/v1/dsr/delete", nil, token)
	needStatus(t, st, http.StatusAccepted, resp)
	jobID, _ := resp["job_id"].(string)
	confirm, _ := resp["dev_confirm_token"].(string)
	if jobID == "" || confirm == "" || resp["status"] != "awaiting_confirmation" {
		t.Fatalf("delete request: %v", resp)
	}

	// Wrong token rejected; the right one starts processing.
	st, resp, _ = doReq(t, "POST", "/api/v1/dsr/delete/confirm",
		map[string]any{"job_id": jobID, "token": strings.Repeat("0", 64)}, token)
	needStatus(t, st, http.StatusBadRequest, resp)
	st, resp, _ = doReq(t, "POST", "/api/v1/dsr/delete/confirm",
		map[string]any{"job_id": jobID, "token": confirm}, token)
	needStatus(t, st, http.StatusAccepted, resp)

	final := pollDSRStatus(t, token, jobID)
	if final["status"] != "done" {
		t.Fatalf("delete job failed: %v", final)
	}

	// Account anonymized.
	var phone, displayName string
	var verified bool
	err := testStore.Social.QueryRow(ctx,
		`SELECT phone, display_name, phone_verified FROM social.users WHERE id=$1`, uid).
		Scan(&phone, &displayName, &verified)
	if err != nil {
		t.Fatalf("users row vanished: %v", err)
	}
	if phone != fmt.Sprintf("+deleted-%d", uid) || displayName != "Deleted user" || verified {
		t.Fatalf("anonymization incomplete: %q %q verified=%v", phone, displayName, verified)
	}
	var realName string
	if err := testStore.Social.QueryRow(ctx,
		`SELECT real_name FROM social.user_profiles WHERE user_id=$1`, uid).Scan(&realName); err != nil {
		t.Fatalf("profile row: %v", err)
	}
	if realName != "" {
		t.Fatalf("real_name not scrubbed: %q", realName)
	}
	var identCount int
	if err := testStore.Social.QueryRow(ctx,
		`SELECT count(*) FROM identity.social_identities WHERE user_id=$1`, uid).Scan(&identCount); err != nil {
		t.Fatalf("identity count: %v", err)
	}
	if identCount != 0 {
		t.Fatalf("linked identities not erased: %d", identCount)
	}

	// Dating data erased through the flagged connection.
	var datingCount int
	if err := testStore.DatingConn(ctx, func(q store.Querier) error {
		return q.QueryRow(ctx, `SELECT count(*) FROM dating.dating_profiles WHERE user_id=$1`, uid).Scan(&datingCount)
	}); err != nil {
		t.Fatalf("dating count: %v", err)
	}
	if datingCount != 0 {
		t.Fatalf("dating profile not erased: %d", datingCount)
	}

	// After completion a new job is allowed.
	st, resp, _ = doReq(t, "DELETE", "/api/v1/dsr/delete", nil, token)
	needStatus(t, st, http.StatusAccepted, resp)
}

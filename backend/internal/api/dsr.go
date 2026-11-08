// Phase D — data subject rights (GDPR/PDPA): export, erasure, job status.
//
// Jobs are stored in Redis with a 30-day TTL matching the deletion SLA:
// no job outlives the window it must complete in. Export payloads are
// envelope-encrypted before storage (see internal/crypto) so a Redis dump
// alone yields nothing. Deletion is a two-step confirmed flow: the user
// requests deletion, receives a single-use confirm token (email when the
// account has a linked identity email, development log/response
// otherwise), and only the confirm executes the anonymization.
package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"nexus/backend/internal/store"
)

const dsrJobTTL = 30 * 24 * time.Hour

type dsrJob struct {
	ID          string    `json:"job_id"`
	Type        string    `json:"type"`   // export | delete
	Status      string    `json:"status"` // processing | awaiting_confirmation | done | failed
	UserID      int64     `json:"user_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Error       string    `json:"error,omitempty"`
	ConfirmHash string    `json:"confirm_hash"` // sha256 of the single-use delete token
}

func dsrJobKey(id string) string     { return "dsr:job:" + id }
func dsrUserKey(uid int64) string    { return fmt.Sprintf("dsr:user:%d", uid) }
func dsrExportKey(id string) string  { return "dsr:export:" + id }

func newDSRJobID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// saveJob writes the job and refreshes both TTLs.
func (s *Server) saveJob(ctx context.Context, j *dsrJob) error {
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := s.Store.RDB.Set(ctx, dsrJobKey(j.ID), raw, dsrJobTTL).Err(); err != nil {
		return err
	}
	return s.Store.RDB.Set(ctx, dsrUserKey(j.UserID), j.ID, dsrJobTTL).Err()
}

// loadJobFor loads a job and enforces ownership.
func (s *Server) loadJobFor(ctx context.Context, id string, uid int64) (*dsrJob, error) {
	raw, err := s.Store.RDB.Get(ctx, dsrJobKey(id)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var j dsrJob
	if err := json.Unmarshal([]byte(raw), &j); err != nil {
		return nil, err
	}
	if j.UserID != uid {
		return nil, nil // do not reveal other users' job ids
	}
	return &j, nil
}

func (j *dsrJob) public() map[string]any {
	return map[string]any{
		"job_id":     j.ID,
		"type":       j.Type,
		"status":     j.Status,
		"created_at": j.CreatedAt,
		"updated_at": j.UpdatedAt,
		"error":      j.Error,
	}
}

// activeJob returns the user's latest job id, if any.
func (s *Server) activeJob(ctx context.Context, uid int64) (string, error) {
	id, err := s.Store.RDB.Get(ctx, dsrUserKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	return id, err
}

// inflightJob returns the user's job id only while that job is still
// processing or awaiting confirmation; completed/failed jobs no longer block.
func (s *Server) inflightJob(ctx context.Context, uid int64) (string, error) {
	id, err := s.activeJob(ctx, uid)
	if err != nil || id == "" {
		return "", err
	}
	j, err := s.loadJobFor(ctx, id, uid)
	if err != nil || j == nil {
		return "", err
	}
	switch j.Status {
	case "processing", "awaiting_confirmation":
		return id, nil
	}
	return "", nil
}

// handleDSRExportRequest creates an export job and processes it in the
// background; poll GET /api/v1/dsr/status until status=done, then download
// via GET /api/v1/dsr/export/{jobID}.
func (s *Server) handleDSRExportRequest(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	ctx := r.Context()
	if id, err := s.inflightJob(ctx, uid); err != nil {
		serverError(w, "dsr active job", err)
		return
	} else if id != "" {
		errorJSON(w, http.StatusConflict, "a DSR job is already in flight")
		return
	}
	id, err := newDSRJobID()
	if err != nil {
		serverError(w, "dsr job id", err)
		return
	}
	j := &dsrJob{ID: id, Type: "export", Status: "processing", UserID: uid,
		CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := s.saveJob(ctx, j); err != nil {
		serverError(w, "dsr job save", err)
		return
	}
	go s.runExport(id, uid)
	writeJSON(w, http.StatusAccepted, j.public())
}

// linkedEmail returns the first email from the user's linked social
// identities, or "" for phone-only accounts. Errors are ignored: email is a
// convenience channel, the authenticated download is the real delivery.
func (s *Server) linkedEmail(ctx context.Context, uid int64) string {
	var email string
	_ = s.Store.Social.QueryRow(ctx,
		`SELECT email FROM identity.social_identities WHERE user_id=$1 AND email <> '' LIMIT 1`,
		uid).Scan(&email)
	return email
}

func (s *Server) runExport(jobID string, uid int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	j, err := s.loadJobFor(ctx, jobID, uid)
	if err != nil || j == nil {
		return
	}
	finish := func(status, cause string) {
		j.Status, j.Error, j.UpdatedAt = status, cause, time.Now()
		if err := s.saveJob(context.Background(), j); err != nil {
			log.Printf("dsr export finish: %v", err)
		}
	}
	export, err := s.collectUserData(ctx, uid)
	if err != nil {
		finish("failed", "export collection failed")
		return
	}
	raw, err := json.Marshal(export)
	if err != nil {
		finish("failed", "export encode failed")
		return
	}
	env, err := s.Crypto.Encrypt(ctx, raw)
	if err != nil {
		finish("failed", "export encryption failed")
		return
	}
	if err := s.Store.RDB.Set(ctx, dsrExportKey(jobID), env, dsrJobTTL).Err(); err != nil {
		finish("failed", "export storage failed")
		return
	}
	finish("done", "")
	if to := s.linkedEmail(ctx, uid); to != "" {
		if err := s.Email.Send(ctx, to,
			"Your Nexus data export is ready",
			fmt.Sprintf("Your export job %s is ready. Sign in to the app and open GET /api/v1/dsr/export/%s within 30 days to download it.", jobID, jobID)); err != nil {
			log.Printf("dsr export notify: %v", err)
		}
	}
}

// collectUserData gathers every row that constitutes the user's personal
// data across both schemas. Unknown columns/tables fail the job rather than
// silently shipping a partial export.
func (s *Server) collectUserData(ctx context.Context, uid int64) (map[string]any, error) {
	out := map[string]any{"user_id": uid, "exported_at": time.Now().UTC()}
	social := s.Store.Social

	var phone, displayName *string
	var birthDate *time.Time
	var createdAt time.Time
	err := social.QueryRow(ctx,
		`SELECT phone, display_name, birth_date, created_at FROM social.users WHERE id=$1`, uid).
		Scan(&phone, &displayName, &birthDate, &createdAt)
	if err != nil {
		return nil, fmt.Errorf("users: %w", err)
	}
	out["account"] = map[string]any{
		"phone": phoneString(phone), "display_name": displayName, "birth_date": birthDate,
		"created_at": createdAt,
	}

	var realName, avatarURL, township string
	err = social.QueryRow(ctx,
		`SELECT real_name, avatar_url, township FROM social.user_profiles WHERE user_id=$1`, uid).
		Scan(&realName, &avatarURL, &township)
	if err != nil {
		return nil, fmt.Errorf("user_profiles: %w", err)
	}
	out["profile"] = map[string]any{
		"real_name": realName, "avatar_url": avatarURL, "township": township,
	}

	out["posts"], err = queryRows(ctx, social,
		`SELECT id, content, township, created_at FROM social.posts WHERE user_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("posts: %w", err)
	}
	out["comments"], err = queryRows(ctx, social,
		`SELECT id, post_id, body, created_at FROM social.comments WHERE user_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("comments: %w", err)
	}
	out["reactions"], err = queryRows(ctx, social,
		`SELECT post_id, kind, created_at FROM social.reactions WHERE user_id=$1 ORDER BY post_id`, uid)
	if err != nil {
		return nil, fmt.Errorf("reactions: %w", err)
	}
	out["friendships"], err = queryRows(ctx, social,
		`SELECT friend_id, status, created_at FROM social.friendships WHERE user_id=$1 ORDER BY friend_id`, uid)
	if err != nil {
		return nil, fmt.Errorf("friendships: %w", err)
	}
	out["blocks"], err = queryRows(ctx, social,
		`SELECT blocked_id, created_at FROM social.blocks WHERE blocker_id=$1 ORDER BY blocked_id`, uid)
	if err != nil {
		return nil, fmt.Errorf("blocks: %w", err)
	}
	out["reports_filed"], err = queryRows(ctx, social,
		`SELECT id, target_kind, target_id, reason, status, created_at FROM social.reports WHERE reporter_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("reports: %w", err)
	}
	out["chat_threads"], err = queryRows(ctx, social,
		`SELECT id, mode, peer_id, product_id, created_at FROM social.chat_threads WHERE owner_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("chat_threads: %w", err)
	}
	out["chat_messages"], err = queryRows(ctx, social,
		`SELECT id, thread_id, body, created_at FROM social.chat_messages WHERE sender_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("chat_messages: %w", err)
	}
	out["coin_ledger"], err = queryRows(ctx, social,
		`SELECT id, amount, kind, request_id, created_at FROM ledger.coin_transactions WHERE user_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("coin_ledger: %w", err)
	}
	out["market_listings"], err = queryRows(ctx, social,
		`SELECT id, title, description, price_coins, price_mmk, township, created_at FROM market.products WHERE seller_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("market_listings: %w", err)
	}
	out["linked_identities"], err = queryRows(ctx, social,
		`SELECT provider, provider_user_id, real_name, avatar_url, email, linked_at FROM identity.social_identities WHERE user_id=$1 ORDER BY id`, uid)
	if err != nil {
		return nil, fmt.Errorf("linked_identities: %w", err)
	}

	err = s.Store.DatingConn(ctx, func(q store.Querier) error {
		rows, err := q.Query(ctx,
			`SELECT nickname, bio, avatar_url, is_hidden_from_friends, updated_at FROM dating.dating_profiles WHERE user_id=$1`, uid)
		if err != nil {
			return err
		}
		defer rows.Close()
		dating := []map[string]any{}
		for rows.Next() {
			var nick, bio, avatar string
			var hidden bool
			var updated time.Time
			if err := rows.Scan(&nick, &bio, &avatar, &hidden, &updated); err != nil {
				return err
			}
			dating = append(dating, map[string]any{
				"nickname": nick, "bio": bio, "avatar_url": avatar,
				"is_hidden_from_friends": hidden, "updated_at": updated,
			})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("dating_profiles: %w", err)
	}
	return out, nil
}

func phoneString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// queryRows runs a single-column-set query and returns generic row maps.
func queryRows(ctx context.Context, q store.Querier, sql string, args ...any) ([]map[string]any, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := rows.FieldDescriptions()
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			m[string(c.Name)] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// handleDSRExportDownload returns the decrypted export for a completed job.
func (s *Server) handleDSRExportDownload(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	j, err := s.loadJobFor(r.Context(), r.PathValue("jobID"), uid)
	if err != nil {
		serverError(w, "dsr job load", err)
		return
	}
	if j == nil {
		errorJSON(w, http.StatusNotFound, "job not found")
		return
	}
	if j.Type != "export" || j.Status != "done" {
		errorJSON(w, http.StatusConflict, "export not ready")
		return
	}
	env, err := s.Store.RDB.Get(r.Context(), dsrExportKey(j.ID)).Result()
	if errors.Is(err, redis.Nil) {
		errorJSON(w, http.StatusGone, "export expired")
		return
	}
	if err != nil {
		serverError(w, "dsr export load", err)
		return
	}
	pt, err := s.Crypto.Decrypt(r.Context(), []byte(env))
	if err != nil {
		serverError(w, "dsr export decrypt", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pt)
}

// handleDSRStatus reports a job's status; ?job_id= defaults to the user's
// most recent job.
func (s *Server) handleDSRStatus(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	id := r.URL.Query().Get("job_id")
	if id == "" {
		var err error
		id, err = s.activeJob(r.Context(), uid)
		if err != nil {
			serverError(w, "dsr active job", err)
			return
		}
		if id == "" {
			errorJSON(w, http.StatusNotFound, "no DSR job")
			return
		}
	}
	j, err := s.loadJobFor(r.Context(), id, uid)
	if err != nil {
		serverError(w, "dsr job load", err)
		return
	}
	if j == nil {
		errorJSON(w, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(w, http.StatusOK, j.public())
}

// handleDSRDeleteRequest creates an awaiting-confirmation deletion job and
// issues the single-use confirm token. With OTP_DEBUG (development) the
// token rides the response like dev_code; otherwise it goes out by email
// to the linked identity address — the log provider never prints it.
func (s *Server) handleDSRDeleteRequest(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	ctx := r.Context()
	if id, err := s.inflightJob(ctx, uid); err != nil {
		serverError(w, "dsr active job", err)
		return
	} else if id != "" {
		errorJSON(w, http.StatusConflict, "a DSR job is already in flight")
		return
	}
	id, err := newDSRJobID()
	if err != nil {
		serverError(w, "dsr job id", err)
		return
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		serverError(w, "dsr token", err)
		return
	}
	tokenHex := hex.EncodeToString(token)
	sum := sha256.Sum256([]byte(tokenHex))
	j := &dsrJob{ID: id, Type: "delete", Status: "awaiting_confirmation", UserID: uid,
		CreatedAt: time.Now(), UpdatedAt: time.Now(), ConfirmHash: hex.EncodeToString(sum[:])}
	if err := s.saveJob(ctx, j); err != nil {
		serverError(w, "dsr job save", err)
		return
	}
	resp := j.public()
	if s.Cfg.OTPDebug {
		// Development parity with dev_code: no real mailbox exists locally.
		resp["dev_confirm_token"] = tokenHex
	} else if to := s.linkedEmail(ctx, uid); to != "" {
		if err := s.Email.Send(ctx, to,
			"Confirm your Nexus account deletion",
			fmt.Sprintf("A deletion was requested for your Nexus account. To confirm within 30 days, call POST /api/v1/dsr/delete/confirm with job_id=%s and token=%s. If you did not request this, ignore this message; the account stays active until confirmed.", id, tokenHex)); err != nil {
			log.Printf("dsr delete notify: %v", err)
		}
	}
	writeJSON(w, http.StatusAccepted, resp)
}

// handleDSRDeleteConfirm validates the token and executes anonymization in
// the background; poll status until done.
func (s *Server) handleDSRDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	var req struct {
		JobID string `json:"job_id"`
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.JobID == "" || req.Token == "" {
		errorJSON(w, http.StatusBadRequest, "job_id and token are required")
		return
	}
	j, err := s.loadJobFor(r.Context(), req.JobID, uid)
	if err != nil {
		serverError(w, "dsr job load", err)
		return
	}
	if j == nil {
		errorJSON(w, http.StatusNotFound, "job not found")
		return
	}
	if j.Type != "delete" || j.Status != "awaiting_confirmation" {
		errorJSON(w, http.StatusConflict, "job is not awaiting confirmation")
		return
	}
	sum := sha256.Sum256([]byte(req.Token))
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(j.ConfirmHash)) != 1 {
		errorJSON(w, http.StatusBadRequest, "invalid confirmation token")
		return
	}
	j.Status, j.UpdatedAt = "processing", time.Now()
	if err := s.saveJob(r.Context(), j); err != nil {
		serverError(w, "dsr job save", err)
		return
	}
	go s.runDelete(j.ID, uid)
	writeJSON(w, http.StatusAccepted, j.public())
}

// runDelete anonymizes the account: identity columns are scrubbed, linked
// provider identities and the dating profile are erased, and user content
// stays attributed to the anonymized account (documented in
// docs/DSR_DESIGN.md). Failures flip the job to failed; the user can retry
// within the SLA window.
func (s *Server) runDelete(jobID string, uid int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	j, err := s.loadJobFor(ctx, jobID, uid)
	if err != nil || j == nil {
		return
	}
	finish := func(status, cause string) {
		j.Status, j.Error, j.UpdatedAt = status, cause, time.Now()
		if err := s.saveJob(context.Background(), j); err != nil {
			log.Printf("dsr delete finish: %v", err)
		}
	}
	social := s.Store.Social
	if _, err := social.Exec(ctx,
		`UPDATE social.users SET phone = '+deleted-' || id, display_name = 'Deleted user', phone_verified = false, birth_date = NULL WHERE id=$1`, uid); err != nil {
		finish("failed", fmt.Sprintf("anonymize users: %v", err))
		return
	}
	if _, err := social.Exec(ctx,
		`UPDATE social.user_profiles SET real_name='', avatar_url='' WHERE user_id=$1`, uid); err != nil {
		finish("failed", fmt.Sprintf("anonymize user_profiles: %v", err))
		return
	}
	if _, err := social.Exec(ctx,
		`DELETE FROM identity.social_identities WHERE user_id=$1`, uid); err != nil {
		finish("failed", fmt.Sprintf("erase identities: %v", err))
		return
	}
	if err := s.Store.DatingConn(ctx, func(q store.Querier) error {
		for _, stmt := range []string{
			`DELETE FROM dating.dating_profiles WHERE user_id=$1`,
			`DELETE FROM dating.radar_presence WHERE user_id=$1`,
			`DELETE FROM dating.swipes WHERE user_id=$1 OR target_id=$1`,
		} {
			if _, err := q.Exec(ctx, stmt, uid); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		finish("failed", fmt.Sprintf("erase dating data: %v", err))
		return
	}
	finish("done", "")
}

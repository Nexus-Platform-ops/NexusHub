package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"nexus/backend/internal/store"
)

const minimumDatingAge = 18

var reportKinds = map[string]bool{
	"user": true, "post": true, "product": true, "whisper": true,
}

var reportReasons = map[string]bool{
	"harassment": true, "spam": true, "scam": true, "nudity": true,
	"underage": true, "impersonation": true, "other": true,
}

// ageOn returns whole years between dob and now (calendar arithmetic).
func ageOn(dob, now time.Time) int {
	years := now.Year() - dob.Year()
	if now.Month() < dob.Month() || (now.Month() == dob.Month() && now.Day() < dob.Day()) {
		years--
	}
	return years
}

// isAdult reads the stored birth date. The dating role cannot see this
// column (social.users grants are exclusive to nexus_social).
func (s *Server) isAdult(ctx context.Context, uid int64) (bool, error) {
	var adult bool
	err := s.Store.Social.QueryRow(ctx,
		`SELECT birth_date IS NOT NULL
		        AND birth_date <= (CURRENT_DATE - INTERVAL '18 years')
		 FROM social.users WHERE id=$1`, uid).Scan(&adult)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, errNotFound
	}
	return adult, err
}

// requireAdult writes a 403 unless the caller has verified an 18+ birth date.
func (s *Server) requireAdult(w http.ResponseWriter, r *http.Request) bool {
	uid, _ := userID(r)
	adult, err := s.isAdult(r.Context(), uid)
	if errors.Is(err, errNotFound) {
		errorJSON(w, http.StatusNotFound, "account not found")
		return false
	}
	if err != nil {
		serverError(w, "check age", err)
		return false
	}
	if !adult {
		errorJSON(w, http.StatusForbidden, "dating features require 18+ verification")
		return false
	}
	return true
}

// handlePutBirthDate stores an 18+ birth date. Under-18 dates are rejected
// without persisting anything (minors' birth dates are not retained).
func (s *Server) handlePutBirthDate(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		DateOfBirth string `json:"date_of_birth"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	dob, err := time.Parse("2006-01-02", strings.TrimSpace(req.DateOfBirth))
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "date_of_birth must be YYYY-MM-DD")
		return
	}
	now := time.Now().UTC()
	if dob.After(now) || dob.Year() < 1900 {
		errorJSON(w, http.StatusBadRequest, "date_of_birth out of range")
		return
	}
	if ageOn(dob, now) < minimumDatingAge {
		writeJSON(w, http.StatusOK, map[string]any{
			"adult": false, "stored": false, "minimum_age": minimumDatingAge,
		})
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`UPDATE social.users SET birth_date=$2 WHERE id=$1`,
		uid, dob.Format("2006-01-02")); err != nil {
		serverError(w, "save birth date", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"adult": true, "stored": true, "minimum_age": minimumDatingAge,
	})
}

// handleCreateReport records an abuse report for moderation review.
func (s *Server) handleCreateReport(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		TargetKind string `json:"target_kind"`
		TargetID   string `json:"target_id"`
		Reason     string `json:"reason"`
		Details    string `json:"details"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.TargetKind = strings.ToLower(trimSpace(req.TargetKind))
	req.Reason = strings.ToLower(trimSpace(req.Reason))
	if !reportKinds[req.TargetKind] {
		errorJSON(w, http.StatusBadRequest, "target_kind must be user, post, product or whisper")
		return
	}
	if !reportReasons[req.Reason] {
		errorJSON(w, http.StatusBadRequest, "reason is not a supported report category")
		return
	}
	req.TargetID = trimLimit(req.TargetID, 64)
	if req.TargetID == "" {
		errorJSON(w, http.StatusBadRequest, "target_id is required")
		return
	}
	req.Details = trimLimit(req.Details, 1000)

	targetID := req.TargetID
	if req.TargetKind == "user" {
		target, ok := nodeIDDecode(req.TargetID)
		if !ok || target == uid {
			errorJSON(w, http.StatusBadRequest, "invalid target_id")
			return
		}
		var exists bool
		if err := s.Store.Social.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM social.users WHERE id=$1)`, target).Scan(&exists); err != nil {
			serverError(w, "lookup report target", err)
			return
		}
		if !exists {
			errorJSON(w, http.StatusNotFound, "user not found")
			return
		}
		targetID = strconv.FormatInt(target, 10)
	}

	var id int64
	var status string
	if err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO social.reports (reporter_id, target_kind, target_id, reason, details)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, status`,
		uid, req.TargetKind, targetID, req.Reason, req.Details).Scan(&id, &status); err != nil {
		serverError(w, "create report", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "status": status})
}

// handleBlock suppresses another account everywhere (radar, swipe, whisper)
// by writing both the block row and cleaning up any existing match.
func (s *Server) handleBlock(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req struct {
		NodeID string `json:"node_id"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	target, ok := nodeIDDecode(strings.TrimSpace(req.NodeID))
	if !ok || target == uid {
		errorJSON(w, http.StatusBadRequest, "invalid node_id")
		return
	}
	var exists bool
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM social.users WHERE id=$1)`, target).Scan(&exists); err != nil {
		serverError(w, "lookup block target", err)
		return
	}
	if !exists {
		errorJSON(w, http.StatusNotFound, "user not found")
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`INSERT INTO social.blocks (blocker_id, blocked_id) VALUES ($1,$2)
		 ON CONFLICT DO NOTHING`, uid, target); err != nil {
		serverError(w, "block user", err)
		return
	}
	// Best-effort dating cleanup: the explicit block check in swipe/whisper
	// is the enforcement; removing swipes/matches is hygiene.
	if err := s.Store.DatingConn(r.Context(), func(q store.Querier) error {
		ctx := r.Context()
		if _, err := q.Exec(ctx,
			`DELETE FROM dating.matches
			 WHERE a_id = LEAST($1::bigint,$2::bigint)
			   AND b_id = GREATEST($1::bigint,$2::bigint)`, uid, target); err != nil {
			return err
		}
		_, err := q.Exec(ctx,
			`DELETE FROM dating.swipes
			 WHERE (user_id=$1 AND target_id=$2) OR (user_id=$2 AND target_id=$1)`, uid, target)
		return err
	}); err != nil {
		log.Printf("block cleanup failed (block is still enforced): %v", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"blocked": true, "node_id": nodeIDEncode(target),
	})
}

// handleListBlocks lists the caller's blocked accounts as opaque node ids.
func (s *Server) handleListBlocks(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT blocked_id, created_at FROM social.blocks
		 WHERE blocker_id=$1 ORDER BY created_at DESC`, uid)
	if err != nil {
		serverError(w, "list blocks", err)
		return
	}
	defer rows.Close()
	blocks := make([]map[string]any, 0)
	for rows.Next() {
		var id int64
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			serverError(w, "scan block", err)
			return
		}
		blocks = append(blocks, map[string]any{
			"node_id": nodeIDEncode(id), "created_at": at.UTC().Format(time.RFC3339),
		})
	}
	if err := rows.Err(); err != nil {
		serverError(w, "list blocks", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"blocks": blocks})
}

// handleUnblock removes a block; idempotent.
func (s *Server) handleUnblock(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	target, ok := nodeIDDecode(r.PathValue("nodeID"))
	if !ok {
		errorJSON(w, http.StatusNotFound, "resource not found")
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`DELETE FROM social.blocks WHERE blocker_id=$1 AND blocked_id=$2`,
		uid, target); err != nil {
		serverError(w, "unblock user", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"blocked": false, "node_id": nodeIDEncode(target),
	})
}

// blockPairExpr is reused by dating queries made through the dating role.
const blockPairExpr = `EXISTS (
	SELECT 1 FROM social.blocked_ids b
	WHERE (b.blocker_id=$1 AND b.blocked_id=$2)
	   OR (b.blocker_id=$2 AND b.blocked_id=$1))`

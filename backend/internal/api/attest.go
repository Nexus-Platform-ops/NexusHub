package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"nexus/backend/internal/attest"
)

const (
	// Nonces are single-use and short-lived; records are the evidence trail.
	attestNonceTTL  = 5 * time.Minute
	attestRecordTTL = 30 * 24 * time.Hour
)

func attestNonceKey(uid int64) string  { return fmt.Sprintf("attest:nonce:%d", uid) }
func attestRecordKey(uid int64) string { return fmt.Sprintf("attest:record:%d", uid) }

// handleAttestNonce issues a fresh single-use nonce. The client embeds it in
// its Play Integrity / App Attest request so the later verdict can be bound
// to this exact challenge (anti-replay).
func (s *Server) handleAttestNonce(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		serverError(w, "attest nonce", err)
		return
	}
	nonce := base64.RawURLEncoding.EncodeToString(raw)
	if err := s.Store.RDB.Set(r.Context(), attestNonceKey(uid), nonce, attestNonceTTL).Err(); err != nil {
		serverError(w, "attest nonce store", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"nonce":      nonce,
		"expires_in": int(attestNonceTTL.Seconds()),
	})
}

// handleAttest consumes the nonce (GetDel: single use) and verifies the
// device token against the configured provider. Success records the
// attestation on the user for 30 days.
func (s *Server) handleAttest(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Token == "" {
		errorJSON(w, http.StatusBadRequest, "token is required")
		return
	}
	nonce, err := s.Store.RDB.GetDel(r.Context(), attestNonceKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		errorJSON(w, http.StatusBadRequest, "no active nonce — request one first")
		return
	}
	if err != nil {
		serverError(w, "attest nonce load", err)
		return
	}
	res, err := s.Attest.Verify(r.Context(), req.Token, nonce)
	if errors.Is(err, attest.ErrInvalidToken) {
		errorJSON(w, http.StatusBadRequest, "attestation verification failed")
		return
	}
	if errors.Is(err, attest.ErrUnavailable) {
		errorJSON(w, http.StatusNotImplemented, "attestation provider not available")
		return
	}
	if err != nil {
		providerError(w, "attest verify", "attestation verification unavailable", err)
		return
	}
	rec, err := json.Marshal(res)
	if err != nil {
		serverError(w, "attest record encode", err)
		return
	}
	if err := s.Store.RDB.Set(r.Context(), attestRecordKey(uid), rec, attestRecordTTL).Err(); err != nil {
		serverError(w, "attest record store", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"provider": res.Provider,
		"device":   res.DeviceID,
		"verdict":  res.Verdict,
	})
}

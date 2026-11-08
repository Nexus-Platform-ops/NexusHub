package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"nexus/backend/internal/attest"
)

const (
	// Nonces are single-use and short-lived; records are the evidence trail.
	attestNonceTTL     = 5 * time.Minute
	attestRecordTTL    = 30 * 24 * time.Hour
	attestNoncePerHour = 5 // nonce issues per hour per device (user)
)

func attestNonceKey(uid int64) string  { return fmt.Sprintf("attest:nonce:%d", uid) }
func attestRecordKey(uid int64) string { return fmt.Sprintf("attest:record:%d", uid) }

// handleAttestNonce issues a fresh single-use nonce. The client embeds it in
// its Play Integrity / DeviceCheck request so the later verdict can be bound
// to this exact challenge (anti-replay). Issues are capped per hour per
// device so a leaked session cannot mint an unlimited challenge stream.
func (s *Server) handleAttestNonce(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	if !s.AttestRL.Allow("uid:" + strconv.FormatInt(uid, 10)) {
		errorJSON(w, http.StatusTooManyRequests, "attest nonce rate limit exceeded (5 per hour)")
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
		"expires_at": time.Now().Add(attestNonceTTL).UTC().Format(time.RFC3339),
	})
}

// handleAttest consumes the nonce (GetDel: single use) and verifies the
// device token against the configured provider. Success records the
// attestation on the user for 30 days. A missing/expired/replayed nonce is
// a 401: the client must start a fresh challenge.
func (s *Server) handleAttest(w http.ResponseWriter, r *http.Request) {
	uid, ok := userID(r)
	if !ok {
		errorJSON(w, http.StatusUnauthorized, "missing user")
		return
	}
	var req struct {
		Token         string `json:"token"`          // legacy field
		PlatformToken string `json:"platform_token"` // canonical client field
		Nonce         string `json:"nonce"`          // optional: cross-check against the stored challenge
	}
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	token := req.Token
	if token == "" {
		token = req.PlatformToken
	}
	if token == "" {
		errorJSON(w, http.StatusBadRequest, "token is required")
		return
	}
	nonce, err := s.Store.RDB.GetDel(r.Context(), attestNonceKey(uid)).Result()
	if errors.Is(err, redis.Nil) {
		errorJSON(w, http.StatusUnauthorized, "no active or expired nonce — request a fresh one")
		return
	}
	if err != nil {
		serverError(w, "attest nonce load", err)
		return
	}
	if req.Nonce != "" && req.Nonce != nonce {
		errorJSON(w, http.StatusUnauthorized, "nonce mismatch — request a fresh one")
		return
	}
	res, err := s.Attest.Verify(r.Context(), token, nonce)
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
		"attested": true,
		"provider": res.Provider,
		"device":   res.DeviceID,
		"verdict":  res.Verdict,
	})
}

// attested gates dating discovery on a fresh attestation record. With the
// development "log" provider (or an unset provider, which attestation.New
// resolves to the same log stub) it is a no-op — local builds and CI have no
// vendor tokens. With any real provider a missing record answers 403 so
// unattested devices neither appear in nor consume discovery.
func (s *Server) attested(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if provider := s.Cfg.AttestationProvider; provider != "" && provider != attest.Log {
			uid, ok := userID(r)
			if !ok {
				errorJSON(w, http.StatusUnauthorized, "missing user")
				return
			}
			n, err := s.Store.RDB.Exists(r.Context(), attestRecordKey(uid)).Result()
			if err != nil {
				serverError(w, "attest gate", err)
				return
			}
			if n == 0 {
				errorJSON(w, http.StatusForbidden, "device attestation required for discovery")
				return
			}
		}
		next.ServeHTTP(w, r)
	}
}

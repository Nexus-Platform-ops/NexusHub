package api

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"time"
)

type ctxKey int

const (
	ctxUserID     ctxKey = iota
	healthTimeout        = 3 * time.Second
	maxBodyBytes         = 1 << 20
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errorJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serverError logs the cause, reports it to Sentry when enabled, and returns
// a generic 500 (no internals leaked).
func serverError(w http.ResponseWriter, what string, err error) {
	log.Printf("server error: %s: %v", what, err)
	captureError(w, what, err)
	errorJSON(w, http.StatusInternalServerError, "internal server error")
}

// providerError reports an outbound provider failure without leaking vendor
// details to the client.
func providerError(w http.ResponseWriter, what, msg string, err error) {
	log.Printf("provider error: %s: %v", what, err)
	captureError(w, what, err)
	errorJSON(w, http.StatusBadGateway, msg)
}

func captureError(w http.ResponseWriter, what string, err error) {
	if c, ok := w.(interface{ captureError(string, error) }); ok {
		c.captureError(what, err)
	}
}

// readJSON decodes a bounded, strict JSON request body.
func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return err
	}
	return nil
}

// subtleConstantTimeCompare is a string convenience wrapper over
// crypto/subtle for signature checks.
func subtleConstantTimeCompare(a, b string) int {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}

// userID returns the authenticated caller id from the request context.
func userID(r *http.Request) (int64, bool) {
	v, ok := r.Context().Value(ctxUserID).(int64)
	return v, ok
}

func trimLimit(s string, max int) string {
	s = trimSpace(s)
	if len(s) > max {
		return s[:max]
	}
	return s
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}

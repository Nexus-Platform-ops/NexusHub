package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"nexus/backend/internal/auth"
	"nexus/backend/internal/config"
	"nexus/backend/internal/payments"
	"nexus/backend/internal/ratelimit"
	"nexus/backend/internal/sms"
	"nexus/backend/internal/store"
)

// Server wires the HTTP surface to the stores and outbound providers.
type Server struct {
	Store    *store.Store
	JWT      *auth.JWT
	RL       *ratelimit.Limiter
	Cfg      config.Config
	SMS      sms.Sender
	Payments payments.Provider
}

// New builds a Server with its own limiter and provider clients.
func New(st *store.Store, jwt *auth.JWT, cfg config.Config) *Server {
	return &Server{
		Store: st,
		JWT:   jwt,
		RL:    ratelimit.New(cfg.RateLimitPerMin, time.Minute),
		Cfg:   cfg,
		SMS: sms.New(sms.Config{
			Provider: cfg.SMSProvider,
			APIURL:   cfg.SMSAPIURL,
			APIKey:   cfg.SMSAPIKey,
			SenderID: cfg.SMSSenderID,
			Timeout:  10 * time.Second,
		}),
		Payments: payments.New(payments.Config{
			Provider: cfg.PaymentProvider,
			APIURL:   cfg.PaymentAPIURL,
			APIKey:   cfg.PaymentAPIKey,
			Timeout:  10 * time.Second,
		}),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/health", s.handleHealth)

	pub := func(h http.HandlerFunc) http.Handler { return s.ipLimited(h) }
	mux.Handle("POST /api/v1/auth/register", pub(s.handleRegister))
	mux.Handle("POST /api/v1/auth/verify", pub(s.handleVerify))
	mux.Handle("POST /api/v1/auth/login", pub(s.handleLogin))
	mux.Handle("POST /api/v1/auth/refresh", pub(s.handleRefresh))

	authed := func(h http.HandlerFunc) http.Handler { return s.requireAuth(s.ipLimited(h)) }

	// Social logins (Facebook / TikTok OAuth2, Telegram signed widget).
	// Real provider identity is stored ONLY in the identity schema; it
	// never reaches social.*, dating.* or radar payloads (see
	// social_login_test.go isolation tests).
	mux.Handle("GET /api/v1/auth/social/{provider}/start", pub(s.handleSocialStart))
	mux.Handle("GET /api/v1/auth/social/{provider}/callback", pub(s.handleSocialCallback))
	mux.Handle("POST /api/v1/auth/social/telegram/callback", pub(s.handleTelegramLogin))
	mux.Handle("GET /api/v1/auth/social/links", authed(s.handleSocialLinks))
	mux.Handle("DELETE /api/v1/auth/social/link/{provider}", authed(s.handleSocialUnlink))

	mux.Handle("GET /api/v1/me", authed(s.handleMe))
	mux.Handle("GET /api/v1/profile", authed(s.handleGetProfile))
	mux.Handle("PUT /api/v1/profile", authed(s.handlePutProfile))

	// Safety: 18+ age gate, abuse reports and account blocks.
	mux.Handle("PUT /api/v1/me/birthdate", authed(s.handlePutBirthDate))
	mux.Handle("POST /api/v1/reports", authed(s.handleCreateReport))
	mux.Handle("POST /api/v1/blocks", authed(s.handleBlock))
	mux.Handle("GET /api/v1/blocks", authed(s.handleListBlocks))
	mux.Handle("DELETE /api/v1/blocks/{nodeID}", authed(s.handleUnblock))

	// Social feed (user_profiles schema only — dating data can never appear).
	mux.Handle("POST /api/v1/posts", authed(s.handleCreatePost))
	mux.Handle("GET /api/v1/posts", authed(s.handleListPosts))
	mux.Handle("GET /api/v1/posts/{id}", authed(s.handleGetPost))
	mux.Handle("POST /api/v1/posts/{id}/reactions", authed(s.handleReact))
	mux.Handle("GET /api/v1/posts/{id}/comments", authed(s.handleListComments))
	mux.Handle("POST /api/v1/posts/{id}/comments", authed(s.handleAddComment))

	// Dating (isolated dating schema + anonymous radar + whisper TTL).
	mux.Handle("GET /api/v1/dating/profile", authed(s.handleGetDatingProfile))
	mux.Handle("PUT /api/v1/dating/profile", authed(s.handlePutDatingProfile))
	mux.Handle("GET /api/v1/radar", authed(s.handleRadar))
	mux.Handle("POST /api/v1/dating/swipe", authed(s.handleSwipe))
	mux.Handle("POST /api/v1/whisper", authed(s.handleWhisperSend))
	mux.Handle("GET /api/v1/whisper/{id}", authed(s.handleWhisperRead))

	// Coins ledger (server-side, idempotent signed top-ups).
	mux.Handle("GET /api/v1/coins/balance", authed(s.handleCoinBalance))
	mux.Handle("POST /api/v1/coins/topup", authed(s.handleCoinTopup))
	mux.Handle("POST /api/v1/coins/topup/dev", authed(s.handleCoinTopupDev))
	mux.Handle("GET /api/v1/coins/ledger", authed(s.handleCoinLedger))

	// Provider-backed top-ups: the client picks a server-priced package, gets
	// a checkout URL, and the signed provider webhook credits the ledger.
	mux.Handle("POST /api/v1/coins/intents", authed(s.handleCreatePaymentIntent))
	mux.Handle("GET /api/v1/coins/intents/{id}", authed(s.handleGetPaymentIntent))
	// Deliberately not IP-limited: a dropped callback means a paid user does
	// not receive coins. The HMAC signature and timestamp window are the guard.
	mux.HandleFunc("POST /api/v1/payments/webhook", s.handlePaymentWebhook)

	// Hyper-local marketplace.
	mux.Handle("GET /api/v1/market/products", authed(s.handleListProducts))
	mux.Handle("POST /api/v1/market/products", authed(s.handleCreateProduct))
	mux.Handle("POST /api/v1/market/orders", authed(s.handleCreateOrder))

	// Unified chat (personal vs business).
	mux.Handle("POST /api/v1/chat/threads", authed(s.handleCreateThread))
	mux.Handle("GET /api/v1/chat/threads", authed(s.handleListThreads))
	mux.Handle("GET /api/v1/chat/threads/{id}/messages", authed(s.handleListMessages))
	mux.Handle("POST /api/v1/chat/threads/{id}/messages", authed(s.handlePostMessage))

	var handler http.Handler = mux
	if s.Cfg.Env == config.EnvDevelopment {
		handler = devCORS(handler)
	}
	return logMiddleware(handler)
}

// devCORS lets a browser build served from localhost call the API during
// local development. It is never installed outside development: production
// stays same-origin behind Caddy.
func devCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isLocalOrigin(origin) {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

// ipLimited applies the per-IP fixed-window limiter.
func (s *Server) ipLimited(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.RL.Allow("ip:" + clientIP(r)) {
			errorJSON(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.Index(fwd, ","); i > 0 {
			fwd = fwd[:i]
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// requireAuth validates the Bearer token, enforces the per-user limiter and
// injects the caller id into the request context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			errorJSON(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.JWT.Verify(strings.TrimPrefix(header, "Bearer "), "access")
		if err != nil {
			errorJSON(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		uid, err := auth.UserID(claims)
		if err != nil {
			errorJSON(w, http.StatusUnauthorized, "invalid token subject")
			return
		}
		if !s.RL.Allow("user:" + strconv.FormatInt(uid, 10)) {
			errorJSON(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, uid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		if sentry.CurrentHub().Client() != nil {
			hub := sentry.CurrentHub().Clone()
			// Tags only: query strings can carry coordinates and other
			// user-controlled values that must not leave the process.
			hub.Scope().SetTag("http.method", r.Method)
			hub.Scope().SetTag("http.path", r.URL.Path)
			sw.hub = hub
		}
		start := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				if sw.hub != nil {
					sw.hub.Recover(recovered)
				}
				log.Printf("panic: %v\n%s", recovered, debug.Stack())
				if !sw.wroteHeader {
					errorJSON(sw, http.StatusInternalServerError, "internal server error")
				}
				sw.captured = true
			}
			if sw.status >= 500 && !sw.captured && sw.hub != nil && r.URL.Path != "/api/v1/health" {
				sw.hub.CaptureMessage(fmt.Sprintf("%s %s -> %d", r.Method, r.URL.Path, sw.status))
			}
			log.Printf("%s %s -> %d (%s)", r.Method, r.URL.Path, sw.status, time.Since(start).Round(time.Millisecond))
		}()
		next.ServeHTTP(sw, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	hub         *sentry.Hub
	captured    bool
}

func (w *statusWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// captureError sends the handler's underlying error to Sentry with the
// request scope attached. No-op when reporting is disabled.
func (w *statusWriter) captureError(what string, err error) {
	if w.captured || w.hub == nil {
		return
	}
	w.captured = true
	w.hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("handler", what)
		scope.SetLevel(sentry.LevelError)
		w.hub.CaptureException(err)
	})
}

// handleHealth reports database and Redis readiness.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), healthTimeout)
	defer cancel()
	dbOK := s.Store.Social.Ping(ctx) == nil && s.Store.Dating.Ping(ctx) == nil
	rdOK := s.Store.RDB.Ping(ctx).Err() == nil
	status := http.StatusOK
	if !dbOK || !rdOK {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]bool{"ok": dbOK && rdOK, "db": dbOK, "redis": rdOK})
}

// pathInt64 parses a {id} path value, writing a 404 on failure.
func pathInt64(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	v, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil || v <= 0 {
		errorJSON(w, http.StatusNotFound, "resource not found")
		return 0, false
	}
	return v, true
}

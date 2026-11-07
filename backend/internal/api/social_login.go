package api

// Social login endpoints (Facebook / TikTok OAuth2, Telegram signed widget).
//
// ISOLATION CONTRACT (critical): the real name, avatar and e-mail returned
// by a provider are persisted ONLY in identity.social_identities (a schema
// the nexus_dating role holds zero privileges on). They are never copied
// into social.users, social.user_profiles, the dating schema, or any
// response from the Dating Profile or Anonymous Radar features. New social
// accounts get a generated pseudonymous display name (Nexus-XXXXXXXX) so
// the provider identity cannot leak even by accident. The isolation tests
// in social_login_test.go enforce this end to end.

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"nexus/backend/internal/auth"
	"nexus/backend/internal/socialprov"
)

const oauthStateTTL = 10 * time.Minute

var errSocialConflict = errors.New("identity already linked to another account")

// socialProviders is the set accepted by the route handlers.
var socialProviders = map[string]bool{socialprov.Facebook: true, socialprov.TikTok: true}

// provCfg maps the server config onto a socialprov.Config.
func (s *Server) provCfg() socialprov.Config {
	return socialprov.Config{
		FacebookAppID:      s.Cfg.FacebookAppID,
		FacebookAppSecret:  s.Cfg.FacebookAppSecret,
		FacebookGraphBase:  s.Cfg.FBGraphBase,
		FacebookOAuthBase:  s.Cfg.FBOAuthBase,
		TikTokClientKey:    s.Cfg.TikTokClientKey,
		TikTokClientSecret: s.Cfg.TikTokClientSecret,
		TikTokAPIBase:      s.Cfg.TikTokAPIBase,
		TikTokAuthBase:     s.Cfg.TikTokAuthBase,
		TelegramBotToken:   s.Cfg.TelegramBotToken,
		RedirectBase:       s.Cfg.SocialRedirectBase,
	}
}

// socialRedirect builds the absolute OAuth redirect URI for a provider.
func (s *Server) socialRedirect(provider string) string {
	base := s.Cfg.SocialRedirectBase
	if base == "" {
		base = "http://127.0.0.1:8080/api/v1/auth/social"
	}
	return strings.TrimRight(base, "/") + "/" + provider + "/callback"
}

// randomToken returns n random bytes hex-encoded.
func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := cryptoRead(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// handleSocialStart issues a one-time state (plus a PKCE verifier for TikTok)
// and returns the provider authorization URL.
func (s *Server) handleSocialStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	cfg := s.provCfg()
	if !socialProviders[provider] || !cfg.Enabled(provider) {
		errorJSON(w, http.StatusServiceUnavailable, "social provider not configured")
		return
	}
	state := randomToken(16)
	if state == "" {
		serverError(w, "generate oauth state", errors.New("rand failed"))
		return
	}
	rec := oauthStateRec{Provider: provider}
	redirect := s.socialRedirect(provider)
	if provider == socialprov.TikTok {
		raw := make([]byte, 32)
		if _, err := cryptoRead(raw); err != nil {
			serverError(w, "generate pkce verifier", err)
			return
		}
		rec.Verifier = base64.RawURLEncoding.EncodeToString(raw)
		sum := sha256.Sum256([]byte(rec.Verifier))
		rec.Challenge = base64.RawURLEncoding.EncodeToString(sum[:])
	}
	if err := s.Store.RDB.Set(r.Context(), "oauthstate:"+state, oauthStateJSON(rec), oauthStateTTL).Err(); err != nil {
		serverError(w, "store oauth state", err)
		return
	}
	var authorizeURL string
	if provider == socialprov.Facebook {
		authorizeURL = cfg.FacebookStartURL(redirect, state)
	} else {
		authorizeURL = cfg.TikTokStartURL(redirect, state, rec.Challenge)
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"provider": provider, "authorize_url": authorizeURL, "state": state,
	})
}

type oauthStateRec struct {
	Provider  string `json:"provider"`
	Verifier  string `json:"verifier,omitempty"`
	Challenge string `json:"challenge,omitempty"`
	Redirect  string `json:"redirect,omitempty"`
}

func oauthStateJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// handleSocialCallback consumes the state, exchanges the code, fetches the
// real identity and finishes either a new/login session or (when a Bearer
// token is present) a link to the authenticated account.
func (s *Server) handleSocialCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	cfg := s.provCfg()
	if !socialProviders[provider] || !cfg.Enabled(provider) {
		errorJSON(w, http.StatusServiceUnavailable, "social provider not configured")
		return
	}
	q := r.URL.Query()
	state, code := q.Get("state"), q.Get("code")
	if state == "" || code == "" {
		errorJSON(w, http.StatusBadRequest, "state and code are required")
		return
	}
	var rec oauthStateRec
	val, err := s.Store.RDB.GetDel(r.Context(), "oauthstate:"+state).Result()
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "oauth state expired or invalid")
		return
	}
	if err := json.Unmarshal([]byte(val), &rec); err != nil || rec.Provider != provider {
		errorJSON(w, http.StatusBadRequest, "oauth state does not match provider")
		return
	}
	redirect := rec.Redirect
	if redirect == "" {
		redirect = s.socialRedirect(provider)
	}
	var ident *socialprov.Identity
	switch provider {
	case socialprov.Facebook:
		var token string
		if token, err = cfg.FacebookExchange(r.Context(), code, redirect); err != nil {
			socialProviderError(w, err)
			return
		}
		ident, err = cfg.FacebookUserInfo(r.Context(), token)
	default:
		var token string
		if token, err = cfg.TikTokExchange(r.Context(), code, redirect, rec.Verifier); err != nil {
			socialProviderError(w, err)
			return
		}
		ident, err = cfg.TikTokUserInfo(r.Context(), token)
	}
	if err != nil {
		socialProviderError(w, err)
		return
	}
	s.finishSocialLogin(w, r, ident)
}

// handleTelegramLogin validates the Login Widget payload posted by the
// browser (id, first_name, auth_date, hash, ...).
func (s *Server) handleTelegramLogin(w http.ResponseWriter, r *http.Request) {
	cfg := s.provCfg()
	if !cfg.Enabled(socialprov.Telegram) {
		errorJSON(w, http.StatusServiceUnavailable, "social provider not configured")
		return
	}
	var req map[string]string
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	fields := url.Values{}
	for k, v := range req {
		fields.Set(k, v)
	}
	ident, err := cfg.TelegramVerify(fields)
	if err != nil {
		socialProviderError(w, err)
		return
	}
	s.finishSocialLogin(w, r, ident)
}

// finishSocialLogin links the identity to the bearer-token caller when one
// is present, otherwise logs in (or signs up) by the identity itself, then
// issues the standard token pair.
func (s *Server) finishSocialLogin(w http.ResponseWriter, r *http.Request, ident *socialprov.Identity) {
	var linkUID *int64
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		if claims, err := s.JWT.Verify(strings.TrimPrefix(header, "Bearer "), "access"); err == nil {
			if uid, uerr := auth.UserID(claims); uerr == nil {
				linkUID = &uid
			}
		}
	}
	uid, err := s.upsertSocialUser(r.Context(), ident, linkUID)
	if err != nil {
		if errors.Is(err, errSocialConflict) {
			errorJSON(w, http.StatusConflict, "identity already linked to another account")
			return
		}
		serverError(w, "upsert social user", err)
		return
	}
	var phone, display string
	var verified bool
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(phone,''), display_name, phone_verified FROM social.users WHERE id=$1`, uid).
		Scan(&phone, &display, &verified); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "account not found")
		} else {
			serverError(w, "load social account", err)
		}
		return
	}
	access, err := s.JWT.Sign(uid, "access", auth.AccessTTL)
	if err != nil {
		serverError(w, "sign access token", err)
		return
	}
	refresh, err := s.JWT.Sign(uid, "refresh", auth.RefreshTTL)
	if err != nil {
		serverError(w, "sign refresh token", err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		User:         userJSON{ID: uid, Phone: phone, DisplayName: display, Verified: verified},
	})
}

// upsertSocialUser persists the link and returns the app user id. Real
// identity fields go ONLY into identity.social_identities; the social.users
// display name is a generated pseudonym, never the provider name.
func (s *Server) upsertSocialUser(ctx context.Context, ident *socialprov.Identity, linkUID *int64) (int64, error) {
	pool := s.Store.Social
	var uid int64
	err := pool.QueryRow(ctx,
		`SELECT user_id FROM identity.social_identities
		 WHERE provider=$1 AND provider_user_id=$2`, ident.Provider, ident.ProviderUserID).
		Scan(&uid)
	if err == nil {
		if linkUID != nil && uid != *linkUID {
			return 0, errSocialConflict
		}
		return uid, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	if linkUID != nil {
		if _, err := pool.Exec(ctx,
			`INSERT INTO identity.social_identities
			 (user_id, provider, provider_user_id, real_name, avatar_url, email)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			*linkUID, ident.Provider, ident.ProviderUserID, ident.RealName, ident.AvatarURL, ident.Email); err != nil {
			return 0, err
		}
		return *linkUID, nil
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	display := pseudonym(ident.Provider, ident.ProviderUserID)
	if err := tx.QueryRow(ctx,
		`INSERT INTO social.users (phone, display_name) VALUES (NULL,$1) RETURNING id`,
		display).Scan(&uid); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO identity.social_identities
		 (user_id, provider, provider_user_id, real_name, avatar_url, email)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		uid, ident.Provider, ident.ProviderUserID, ident.RealName, ident.AvatarURL, ident.Email); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return uid, nil
}

// pseudonym derives a stable, non-identifying display name. It deliberately
// ignores every real-identity field of the provider payload.
func pseudonym(provider, providerUserID string) string {
	sum := sha256.Sum256([]byte("nexus-identity:" + provider + ":" + providerUserID))
	return "Nexus-" + hex.EncodeToString(sum[:4])
}

// handleSocialLinks lists the caller's linked providers. For privacy only
// the provider, provider id and link time are returned — never the real
// name or avatar.
func (s *Server) handleSocialLinks(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT provider, provider_user_id, linked_at FROM identity.social_identities
		 WHERE user_id=$1 ORDER BY linked_at`, uid)
	if err != nil {
		serverError(w, "list social links", err)
		return
	}
	defer rows.Close()
	type link struct {
		Provider       string    `json:"provider"`
		ProviderUserID string    `json:"provider_user_id"`
		LinkedAt       time.Time `json:"linked_at"`
	}
	links := []link{}
	for rows.Next() {
		var l link
		if err := rows.Scan(&l.Provider, &l.ProviderUserID, &l.LinkedAt); err != nil {
			serverError(w, "scan social link", err)
			return
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate social links", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": links})
}

// handleSocialUnlink removes a provider link. An account with no verified
// phone cannot lose its only login (it would become unreachable).
func (s *Server) handleSocialUnlink(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	provider := r.PathValue("provider")
	if !socialProviders[provider] && provider != socialprov.Telegram {
		errorJSON(w, http.StatusBadRequest, "unknown social provider")
		return
	}
	var phone string
	var verified bool
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(phone,''), phone_verified FROM social.users WHERE id=$1`, uid).
		Scan(&phone, &verified); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			errorJSON(w, http.StatusNotFound, "account not found")
		} else {
			serverError(w, "load account", err)
		}
		return
	}
	if !verified || phone == "" {
		var count int
		if err := s.Store.Social.QueryRow(r.Context(),
			`SELECT count(*) FROM identity.social_identities WHERE user_id=$1`, uid).Scan(&count); err != nil {
			serverError(w, "count social links", err)
			return
		}
		if count <= 1 {
			errorJSON(w, http.StatusConflict, "cannot unlink the only login of an account without a verified phone")
			return
		}
	}
	tag, err := s.Store.Social.Exec(r.Context(),
		`DELETE FROM identity.social_identities WHERE user_id=$1 AND provider=$2`, uid, provider)
	if err != nil {
		serverError(w, "unlink provider", err)
		return
	}
	if tag.RowsAffected() == 0 {
		errorJSON(w, http.StatusNotFound, "provider not linked")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"unlinked": true})
}

// socialProviderError maps provider/client failures to HTTP statuses.
func socialProviderError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, socialprov.ErrNotConfigured):
		errorJSON(w, http.StatusServiceUnavailable, "social provider not configured")
	case errors.Is(err, socialprov.ErrBadPayload):
		errorJSON(w, http.StatusBadRequest, "invalid social login payload")
	case errors.Is(err, socialprov.ErrUpstream):
		errorJSON(w, http.StatusBadGateway, "social provider unavailable")
	default:
		serverError(w, "social login", err)
	}
}

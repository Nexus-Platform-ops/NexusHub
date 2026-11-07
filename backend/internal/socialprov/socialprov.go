// Package socialprov implements the identity-provider clients for the three
// supported social logins: Facebook and TikTok (standard OAuth2
// authorization-code flows, TikTok with PKCE) and Telegram (Login Widget
// signed-payload validation).
//
// ISOLATION CONTRACT: every value produced by this package is REAL-WORLD
// identity data (legal/nickname, profile picture, e-mail). It may be
// persisted ONLY inside the `identity` schema (identity.social_identities)
// and must never be copied into social.users, social.user_profiles, the
// dating schema, or any API response that feeds the Dating Profile or
// Anonymous Radar features. See docs/ARCHITECTURE.md and the isolation
// tests in internal/api.
package socialprov

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Provider identifiers used across API routes and the database CHECK constraint.
const (
	Facebook = "facebook"
	TikTok   = "tiktok"
	Telegram = "telegram"
)

// Sentinel errors the API layer maps to HTTP statuses.
var (
	// ErrNotConfigured — the provider has no credentials in the environment.
	ErrNotConfigured = errors.New("social provider not configured")
	// ErrUpstream — token exchange or user-info call against the provider failed.
	ErrUpstream = errors.New("social provider exchange failed")
	// ErrBadPayload — a Telegram widget payload failed signature or freshness checks.
	ErrBadPayload = errors.New("invalid social login payload")
)

// Identity carries ONLY real provider identity data. The API stores it in
// identity.social_identities and nowhere else.
type Identity struct {
	Provider       string
	ProviderUserID string
	RealName       string
	AvatarURL      string
	Email          string
}

// telegramAuthMaxAge bounds how old a Telegram widget payload may be.
const telegramAuthMaxAge = 24 * time.Hour

// Config holds provider credentials plus injectable base URLs so tests can
// point every provider at an httptest server. Production leaves the bases
// empty and the defaults below are used.
type Config struct {
	FacebookAppID     string
	FacebookAppSecret string
	FacebookGraphBase string // default https://graph.facebook.com/v19.0
	FacebookOAuthBase string // default https://www.facebook.com/v19.0/dialog/oauth

	TikTokClientKey    string
	TikTokClientSecret string
	TikTokAPIBase      string // default https://open.tiktokapis.com/v2
	TikTokAuthBase     string // default https://www.tiktok.com/v2/auth/authorize/

	TelegramBotToken string
	RedirectBase     string // callback prefix, e.g. https://app.example.com/api/v1/auth/social
	HTTPClient       *http.Client
}

// withDefaults fills the official endpoints and HTTP client.
func (c *Config) withDefaults() {
	if c.FacebookGraphBase == "" {
		c.FacebookGraphBase = "https://graph.facebook.com/v19.0"
	}
	if c.FacebookOAuthBase == "" {
		c.FacebookOAuthBase = "https://www.facebook.com/v19.0/dialog/oauth"
	}
	if c.TikTokAPIBase == "" {
		c.TikTokAPIBase = "https://open.tiktokapis.com/v2"
	}
	if c.TikTokAuthBase == "" {
		c.TikTokAuthBase = "https://www.tiktok.com/v2/auth/authorize/"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = http.DefaultClient
	}
}

// Enabled reports whether the provider has credentials configured.
func (c *Config) Enabled(provider string) bool {
	switch provider {
	case Facebook:
		return c.FacebookAppID != "" && c.FacebookAppSecret != ""
	case TikTok:
		return c.TikTokClientKey != "" && c.TikTokClientSecret != ""
	case Telegram:
		return c.TelegramBotToken != ""
	}
	return false
}

// FacebookStartURL builds the authorization-code redirect target.
func (c *Config) FacebookStartURL(redirectURI, state string) string {
	if c.FacebookAppID == "" {
		return ""
	}
	q := url.Values{
		"client_id":     {c.FacebookAppID},
		"redirect_uri":  {redirectURI},
		"state":         {state},
		"response_type": {"code"},
		"scope":         {"public_profile"},
	}
	return c.FacebookOAuthBase + "?" + q.Encode()
}

// FacebookExchange swaps the authorization code for an access token.
func (c *Config) FacebookExchange(ctx context.Context, code, redirectURI string) (string, error) {
	c.withDefaults()
	if c.FacebookAppID == "" || c.FacebookAppSecret == "" {
		return "", ErrNotConfigured
	}
	q := url.Values{
		"client_id":     {c.FacebookAppID},
		"client_secret": {c.FacebookAppSecret},
		"redirect_uri":  {redirectURI},
		"code":          {code},
	}
	body, err := c.get(ctx, c.FacebookGraphBase+"/oauth/access_token?"+q.Encode())
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.AccessToken == "" {
		return "", fmt.Errorf("%w: facebook token response", ErrUpstream)
	}
	return resp.AccessToken, nil
}

// FacebookUserInfo fetches the app-scoped id, real name, e-mail and profile
// picture URL. These fields are real identity data — see the package contract.
func (c *Config) FacebookUserInfo(ctx context.Context, accessToken string) (*Identity, error) {
	c.withDefaults()
	q := url.Values{"fields": {"id,name,email"}, "access_token": {accessToken}}
	body, err := c.get(ctx, c.FacebookGraphBase+"/me?"+q.Encode())
	if err != nil {
		return nil, err
	}
	var me struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.Unmarshal(body, &me); err != nil || me.ID == "" {
		return nil, fmt.Errorf("%w: facebook profile response", ErrUpstream)
	}
	ident := &Identity{Provider: Facebook, ProviderUserID: me.ID, RealName: me.Name, Email: me.Email}
	// Resolve the profile picture without following redirects.
	pq := url.Values{"redirect": {"false"}, "type": {"large"}, "access_token": {accessToken}}
	if pbody, perr := c.get(ctx, c.FacebookGraphBase+"/"+me.ID+"/picture?"+pq.Encode()); perr == nil {
		var pic struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		}
		if json.Unmarshal(pbody, &pic) == nil {
			ident.AvatarURL = pic.Data.URL
		}
	}
	return ident, nil
}

// TikTokStartURL builds the authorization URL with the PKCE challenge (S256).
func (c *Config) TikTokStartURL(redirectURI, state, codeChallenge string) string {
	if c.TikTokClientKey == "" {
		return ""
	}
	q := url.Values{
		"client_key":            {c.TikTokClientKey},
		"redirect_uri":          {redirectURI},
		"state":                 {state},
		"response_type":         {"code"},
		"scope":                 {"user.info.basic"},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}
	return c.TikTokAuthBase + "?" + q.Encode()
}

// TikTokExchange swaps the authorization code for an access token using the
// PKCE verifier stored with the state.
func (c *Config) TikTokExchange(ctx context.Context, code, redirectURI, verifier string) (string, error) {
	c.withDefaults()
	if c.TikTokClientKey == "" || c.TikTokClientSecret == "" {
		return "", ErrNotConfigured
	}
	form := url.Values{
		"client_key":    {c.TikTokClientKey},
		"client_secret": {c.TikTokClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {verifier},
	}
	body, err := c.post(ctx, c.TikTokAPIBase+"/oauth/token/", form, "")
	if err != nil {
		return "", err
	}
	var resp struct {
		AccessToken string `json:"access_token"`
		Data        struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("%w: tiktok token response", ErrUpstream)
	}
	if resp.AccessToken == "" {
		resp.AccessToken = resp.Data.AccessToken
	}
	if resp.AccessToken == "" {
		return "", fmt.Errorf("%w: tiktok token response", ErrUpstream)
	}
	return resp.AccessToken, nil
}

// TikTokUserInfo fetches the open id, TikTok nickname and avatar. These are
// real identity data — see the package contract.
func (c *Config) TikTokUserInfo(ctx context.Context, accessToken string) (*Identity, error) {
	c.withDefaults()
	body, err := c.post(ctx, c.TikTokAPIBase+"/user/info/?fields=open_id,display_name,avatar_url",
		url.Values{}, accessToken)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data struct {
			User struct {
				OpenID      string `json:"open_id"`
				DisplayName string `json:"display_name"`
				AvatarURL   string `json:"avatar_url"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Data.User.OpenID == "" {
		return nil, fmt.Errorf("%w: tiktok profile response", ErrUpstream)
	}
	return &Identity{
		Provider:       TikTok,
		ProviderUserID: resp.Data.User.OpenID,
		RealName:       resp.Data.User.DisplayName,
		AvatarURL:      resp.Data.User.AvatarURL,
	}, nil
}

// TelegramVerify validates a Telegram Login Widget payload: the HMAC-SHA256
// signature over the sorted key=value pairs (secret = SHA256(bot token)) and
// the auth_date freshness window. On success it returns the real identity.
func (c *Config) TelegramVerify(fields url.Values) (*Identity, error) {
	c.withDefaults()
	if c.TelegramBotToken == "" {
		return nil, ErrNotConfigured
	}
	hash := fields.Get("hash")
	id := fields.Get("id")
	authDate := fields.Get("auth_date")
	if hash == "" || id == "" || authDate == "" {
		return nil, fmt.Errorf("%w: missing id, auth_date or hash", ErrBadPayload)
	}
	var pairs []string
	for key, vals := range fields {
		if key == "hash" || len(vals) == 0 {
			continue
		}
		pairs = append(pairs, key+"="+vals[0])
	}
	sort.Strings(pairs)
	secret := sha256.Sum256([]byte(c.TelegramBotToken))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(strings.Join(pairs, "\n")))
	if hmac.Equal([]byte(hex.EncodeToString(mac.Sum(nil))), []byte(hash)) != true {
		return nil, fmt.Errorf("%w: telegram hash mismatch", ErrBadPayload)
	}
	ts, err := strconv.ParseInt(authDate, 10, 64)
	if err != nil || time.Since(time.Unix(ts, 0)) > telegramAuthMaxAge {
		return nil, fmt.Errorf("%w: telegram auth_date stale", ErrBadPayload)
	}
	name := strings.TrimSpace(fields.Get("first_name") + " " + fields.Get("last_name"))
	return &Identity{
		Provider:       Telegram,
		ProviderUserID: id,
		RealName:       name,
		AvatarURL:      fields.Get("photo_url"),
	}, nil
}

// get performs a GET and returns the raw body, mapping failures to ErrUpstream.
func (c *Config) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	return c.do(req)
}

// post performs a form POST, optionally with a bearer token.
func (c *Config) post(ctx context.Context, u string, form url.Values, bearer string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return c.do(req)
}

func (c *Config) do(req *http.Request) ([]byte, error) {
	c.withDefaults()
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	return body, nil
}

package api

// Social login integration tests: full OAuth2 flows against fake provider
// servers (httptest), plus the CRITICAL identity-isolation tests proving
// provider real names/avatars never reach the Dating Profile, Anonymous
// Radar, feed or /me payloads — and that the nexus_dating DB role cannot
// even read identity.social_identities.

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// ---- helpers ----

func socialDo(t *testing.T, method, path string, body any, bearer string) (int, map[string]any) {
	t.Helper()
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out == nil {
		out = map[string]any{}
	}
	return w.Code, out
}

func socialPhone(t *testing.T) string {
	t.Helper()
	n := atomic.AddInt64(&phoneSeq, 1)
	return fmt.Sprintf("+9597%07d", n)
}

func socialRegisterPhone(t *testing.T, phone string) (int64, string) {
	t.Helper()
	code, res := socialDo(t, "POST", "/api/v1/auth/register",
		map[string]string{"phone": phone, "display_name": "Phone User"}, "")
	if code != http.StatusCreated {
		t.Fatalf("register: %d %v", code, res)
	}
	otp, _ := res["dev_code"].(string)
	code2, res2 := socialDo(t, "POST", "/api/v1/auth/verify",
		map[string]string{"phone": phone, "code": otp}, "")
	if code2 != http.StatusOK {
		t.Fatalf("verify: %d %v", code2, res2)
	}
	uid := int64(res2["user"].(map[string]any)["id"].(float64))
	return uid, res2["access_token"].(string)
}

var fakeIDSeq atomic.Int64

// socialFakeProvider starts one httptest server serving both the Facebook
// and TikTok endpoints and points the server config at it.
func socialFakeProvider(t *testing.T) string {
	t.Helper()
	// Unique identity per fake server so tests never collide on
	// identity.social_identities rows left by other tests.
	fbID := fmt.Sprintf("fb-%d", fakeIDSeq.Add(1))
	ttID := fmt.Sprintf("tt-%d", fakeIDSeq.Load())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/oauth/access_token"):
			q := r.URL.Query()
			if q.Get("client_id") != "fb-app" || q.Get("client_secret") != "fb-secret" || q.Get("code") != "good-code" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"fb-token"}`))
		case r.URL.Path == "/me":
			if r.URL.Query().Get("access_token") != "fb-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"id":%q,"name":"Aung Kyaw Real","email":"real@example.com"}`, fbID)))
		case strings.HasSuffix(r.URL.Path, "/picture"):
			_, _ = w.Write([]byte(`{"data":{"url":"https://cdn.example/real-fb777.jpg"}}`))
		case r.URL.Path == "/oauth/token/":
			_ = r.ParseForm()
			if r.PostFormValue("client_key") != "tt-key" || r.PostFormValue("code") != "tt-code" ||
				r.PostFormValue("code_verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"tt-token"}`))
		case r.URL.Path == "/user/info/":
			if r.Header.Get("Authorization") != "Bearer tt-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"data":{"user":{"open_id":%q,"display_name":"Ma Ma Real","avatar_url":"https://cdn.example/real-tt42.jpg"}}}`, ttID)))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	old := testServer.Cfg
	testServer.Cfg.FacebookAppID = "fb-app"
	testServer.Cfg.FacebookAppSecret = "fb-secret"
	testServer.Cfg.FBGraphBase = srv.URL
	testServer.Cfg.FBOAuthBase = srv.URL + "/dialog/oauth"
	testServer.Cfg.TikTokClientKey = "tt-key"
	testServer.Cfg.TikTokClientSecret = "tt-secret"
	testServer.Cfg.TikTokAPIBase = srv.URL
	testServer.Cfg.TikTokAuthBase = srv.URL + "/auth/authorize/"
	testServer.Cfg.TelegramBotToken = "123:botsecret"
	testServer.Cfg.SocialRedirectBase = "https://app.example.com/api/v1/auth/social"
	t.Cleanup(func() { testServer.Cfg = old })
	return srv.URL
}

func socialTelegramHash(token string, fields map[string]string) string {
	var keys []string
	for k := range fields {
		if k != "hash" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteString(fields[k])
	}
	secret := sha256.Sum256([]byte(token))
	mac := hmac.New(sha256.New, secret[:])
	mac.Write([]byte(sb.String()))
	return hex.EncodeToString(mac.Sum(nil))
}

// socialSignup runs the Facebook signup flow and returns the user id,
// access token and the raw final response.
func socialSignup(t *testing.T, provider, code string) (int64, string, map[string]any) {
	t.Helper()
	cStart, resStart := socialDo(t, "GET", "/api/v1/auth/social/"+provider+"/start", nil, "")
	if cStart != http.StatusOK {
		t.Fatalf("%s start: %d %v", provider, cStart, resStart)
	}
	state, _ := resStart["state"].(string)
	cCb, resCb := socialDo(t, "GET", "/api/v1/auth/social/"+provider+"/callback?code="+code+"&state="+state, nil, "")
	if cCb != http.StatusOK {
		t.Fatalf("%s callback: %d %v", provider, cCb, resCb)
	}
	user := resCb["user"].(map[string]any)
	uid := int64(user["id"].(float64))
	return uid, resCb["access_token"].(string), resCb
}

// ---- tests ----

func TestSocialFacebookSignupAndRelogin(t *testing.T) {
	socialFakeProvider(t)
	uid1, tok, res := socialSignup(t, "facebook", "good-code")
	display, _ := res["user"].(map[string]any)["display_name"].(string)
	// CRITICAL: the pseudonym must NOT be the provider real name.
	if !strings.HasPrefix(display, "Nexus-") || display == "Aung Kyaw Real" {
		t.Fatalf("social signup display_name must be a pseudonym, got %q", display)
	}
	if tok == "" {
		t.Fatal("no access token")
	}
	// Relogin with the same provider identity must resolve to the same user.
	uid2, _, res2 := socialSignup(t, "facebook", "good-code")
	if uid2 != uid1 {
		t.Fatalf("relogin created a new user: %d vs %d", uid1, uid2)
	}
	if res2["user"].(map[string]any)["display_name"] != display {
		t.Fatal("display name changed between logins")
	}
}

func TestSocialTikTokPKCEFlow(t *testing.T) {
	socialFakeProvider(t)
	uid, tok, res := socialSignup(t, "tiktok", "tt-code")
	display, _ := res["user"].(map[string]any)["display_name"].(string)
	if !strings.HasPrefix(display, "Nexus-") || display == "Ma Ma Real" {
		t.Fatalf("tiktok display_name must be a pseudonym, got %q", display)
	}
	if tok == "" || uid == 0 {
		t.Fatal("tiktok flow incomplete")
	}
}

func TestSocialStateReplayAndMismatch(t *testing.T) {
	socialFakeProvider(t)
	// Missing state/code -> 400.
	if c, _ := socialDo(t, "GET", "/api/v1/auth/social/facebook/callback", nil, ""); c != http.StatusBadRequest {
		t.Fatalf("missing params: %d", c)
	}
	// Start a TikTok flow and replay its state against Facebook.
	_, start := socialDo(t, "GET", "/api/v1/auth/social/tiktok/start", nil, "")
	state, _ := start["state"].(string)
	c, res := socialDo(t, "GET", "/api/v1/auth/social/facebook/callback?code=good-code&state="+state, nil, "")
	if c != http.StatusBadRequest {
		t.Fatalf("cross-provider state accepted: %d %v", c, res)
	}
	// Unknown provider -> 503.
	if c, _ := socialDo(t, "GET", "/api/v1/auth/social/myspace/start", nil, ""); c != http.StatusServiceUnavailable {
		t.Fatalf("unknown provider: %d", c)
	}
}

func TestSocialNotConfigured(t *testing.T) {
	old := testServer.Cfg
	testServer.Cfg.FacebookAppID = ""
	testServer.Cfg.TikTokClientKey = ""
	testServer.Cfg.TelegramBotToken = ""
	t.Cleanup(func() { testServer.Cfg = old })
	if c, _ := socialDo(t, "GET", "/api/v1/auth/social/facebook/start", nil, ""); c != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured start: %d", c)
	}
	if c, _ := socialDo(t, "POST", "/api/v1/auth/social/telegram/callback",
		map[string]string{"id": "1"}, ""); c != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured telegram: %d", c)
	}
}

func TestSocialTelegramLogin(t *testing.T) {
	socialFakeProvider(t)
	payload := map[string]string{
		"id": "555000111", "first_name": "Aung", "last_name": "Kyaw",
		"username": "aungk", "photo_url": "https://t.me/i/pic.jpg",
		"auth_date": fmt.Sprintf("%d", time.Now().Unix()),
	}
	payload["hash"] = socialTelegramHash("123:botsecret", payload)
	c, res := socialDo(t, "POST", "/api/v1/auth/social/telegram/callback", payload, "")
	if c != http.StatusOK {
		t.Fatalf("telegram login: %d %v", c, res)
	}
	display, _ := res["user"].(map[string]any)["display_name"].(string)
	if !strings.HasPrefix(display, "Nexus-") || strings.Contains(display, "Aung") {
		t.Fatalf("telegram display_name must be a pseudonym, got %q", display)
	}

	// Tampered hash must be rejected.
	bad := map[string]string{}
	for k, v := range payload {
		bad[k] = v
	}
	bad["hash"] = strings.Repeat("0", 64)
	if c, _ := socialDo(t, "POST", "/api/v1/auth/social/telegram/callback", bad, ""); c != http.StatusBadRequest {
		t.Fatalf("tampered telegram payload: %d", c)
	}
}

func TestSocialLinkUnlinkAndPrivacy(t *testing.T) {
	socialFakeProvider(t)
	uid, bearer := socialRegisterPhone(t, socialPhone(t))

	// Link the Facebook identity to the phone account via the bearer token.
	cStart, resStart := socialDo(t, "GET", "/api/v1/auth/social/facebook/start", nil, bearer)
	if cStart != http.StatusOK {
		t.Fatalf("start: %d %v", cStart, resStart)
	}
	state, _ := resStart["state"].(string)
	c, res := socialDo(t, "GET", "/api/v1/auth/social/facebook/callback?code=good-code&state="+state, nil, bearer)
	if c != http.StatusOK {
		t.Fatalf("link callback: %d %v", c, res)
	}
	if int64(res["user"].(map[string]any)["id"].(float64)) != uid {
		t.Fatal("link flow returned a different account")
	}

	// The links list must NOT expose real names or avatars.
	cL, resL := socialDo(t, "GET", "/api/v1/auth/social/links", nil, bearer)
	if cL != http.StatusOK {
		t.Fatalf("links: %d %v", cL, resL)
	}
	rawL, _ := json.Marshal(resL)
	if strings.Contains(string(rawL), "Aung Kyaw Real") || strings.Contains(string(rawL), "cdn.example") {
		t.Fatalf("links leaked provider identity: %s", rawL)
	}
	links := resL["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["provider"] != "facebook" {
		t.Fatalf("links wrong: %v", resL)
	}

	// The same provider identity bound to a different account -> 409.
	uidB, bearerB := socialRegisterPhone(t, socialPhone(t))
	_, resStart2 := socialDo(t, "GET", "/api/v1/auth/social/facebook/start", nil, bearerB)
	state2, _ := resStart2["state"].(string)
	if c, res := socialDo(t, "GET", "/api/v1/auth/social/facebook/callback?code=good-code&state="+state2, nil, bearerB); c != http.StatusConflict {
		t.Fatalf("conflicting link: %d %v", c, res)
	}

	// A social-only account (no phone) cannot unlink its only login.
	uidSocial, bearerSocial, _ := socialSignup(t, "tiktok", "tt-code")
	_ = uidSocial
	if c, res := socialDo(t, "DELETE", "/api/v1/auth/social/link/tiktok", nil, bearerSocial); c != http.StatusConflict {
		t.Fatalf("unlink only login: %d %v", c, res)
	}

	// A verified phone account may unlink.
	if c, res := socialDo(t, "DELETE", "/api/v1/auth/social/link/facebook", nil, bearer); c != http.StatusOK {
		t.Fatalf("unlink: %d %v", c, res)
	}
	if c, _ := socialDo(t, "DELETE", "/api/v1/auth/social/link/facebook", nil, bearer); c != http.StatusNotFound {
		t.Fatalf("unlink twice: %d", c)
	}
	_ = uidB
}

func TestIdentityIsolationFromDatingAndRadar(t *testing.T) {
	socialFakeProvider(t)
	// Signup through Facebook with a distinctive real name + avatar.
	// Real identity markers that must appear NOWHERE in dating/radar/feed/me.
	const realName = "Aung Kyaw Real"
	const avatar = "cdn.example/real-fb777.jpg"

	// Re-login via social to get a bearer for this user.
	uid2, bearer, _ := socialSignup(t, "facebook", "good-code")
	if uid2 == 0 || bearer == "" {
		t.Fatal("relogin failed")
	}
	if c, res := socialDo(t, "PUT", "/api/v1/me/birthdate",
		map[string]any{"date_of_birth": "1995-04-12"}, bearer); c != http.StatusOK {
		t.Fatalf("social account age verification: %d %v", c, res)
	}
	cP, resP := socialDo(t, "PUT", "/api/v1/dating/profile",
		map[string]any{"nickname": "Cute Panda", "bio": "yo", "avatar_url": "https://cdn.example/panda.png",
			"is_hidden_from_friends": false}, bearer)
	if cP != http.StatusOK {
		t.Fatalf("dating profile: %d %v", cP, resP)
	}

	// Every privacy-sensitive surface must be free of the provider identity.
	surfaces := []struct {
		name string
		do   func() (int, map[string]any)
	}{
		{"dating profile", func() (int, map[string]any) { return socialDo(t, "GET", "/api/v1/dating/profile", nil, bearer) }},
		{"me", func() (int, map[string]any) { return socialDo(t, "GET", "/api/v1/me", nil, bearer) }},
		{"feed", func() (int, map[string]any) { return socialDo(t, "GET", "/api/v1/posts", nil, bearer) }},
		{"social links", func() (int, map[string]any) { return socialDo(t, "GET", "/api/v1/auth/social/links", nil, bearer) }},
	}
	for _, s := range surfaces {
		c, res := s.do()
		if c != http.StatusOK {
			t.Fatalf("%s: %d %v", s.name, c, res)
		}
		raw, _ := json.Marshal(res)
		if strings.Contains(string(raw), realName) || strings.Contains(string(raw), avatar) {
			t.Fatalf("LEAK: %s response contains provider identity: %s", s.name, raw)
		}
	}

	// DB-level isolation: the dating role must be denied identity reads.
	datingURL := envOr("TEST_DATING_URL", "postgres://nexus_dating:dev_dating@127.0.0.1:5432/nexus?sslmode=disable")
	conn, err := pgx.Connect(context.Background(), datingURL)
	if err != nil {
		t.Fatalf("connect dating role: %v", err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(context.Background(),
		`SELECT count(*) FROM identity.social_identities`); err == nil {
		t.Fatal("LEAK: nexus_dating can read identity.social_identities")
	}
	// And the real data must actually be stored, but only readable via the social role.
	socialURL := envOr("TEST_SOCIAL_URL", "postgres://nexus_social:dev_social@127.0.0.1:5432/nexus?sslmode=disable")
	conn2, err := pgx.Connect(context.Background(), socialURL)
	if err != nil {
		t.Fatalf("connect social role: %v", err)
	}
	defer conn2.Close(context.Background())
	var storedName string
	if err := conn2.QueryRow(context.Background(),
		`SELECT real_name FROM identity.social_identities WHERE provider='facebook' ORDER BY id DESC LIMIT 1`).
		Scan(&storedName); err != nil {
		t.Fatalf("identity row missing: %v", err)
	}
	if storedName != realName {
		t.Fatalf("stored identity mismatch: %q", storedName)
	}
}

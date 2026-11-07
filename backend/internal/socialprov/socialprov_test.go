package socialprov

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// telegramHash computes the valid widget hash for a payload, mirroring the
// algorithm Telegram documents for Login Widget callbacks.
func telegramHash(token string, fields map[string]string) string {
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

func newFacebookFake(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/access_token", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "fb-app" || q.Get("client_secret") != "fb-secret" ||
			q.Get("code") != "good-code" || q.Get("redirect_uri") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"fb-token"}`))
	})
	mux.HandleFunc("/me", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("access_token") != "fb-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":"fb-777","name":"Aung Kyaw","email":"ak@example.com"}`))
	})
	mux.HandleFunc("/fb-777/picture", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"url":"https://cdn.example/fb-777.jpg"}}`))
	})
	return httptest.NewServer(mux)
}

func TestFacebookFlow(t *testing.T) {
	srv := newFacebookFake(t)
	defer srv.Close()
	cfg := &Config{
		FacebookAppID: "fb-app", FacebookAppSecret: "fb-secret",
		FacebookGraphBase: srv.URL, FacebookOAuthBase: srv.URL + "/dialog/oauth",
	}
	start := cfg.FacebookStartURL("http://app/cb", "state-1")
	if !strings.Contains(start, "client_id=fb-app") || !strings.Contains(start, "state=state-1") {
		t.Fatalf("authorize url missing params: %s", start)
	}
	tok, err := cfg.FacebookExchange(context.Background(), "good-code", "http://app/cb")
	if err != nil || tok != "fb-token" {
		t.Fatalf("exchange: %v %q", err, tok)
	}
	ident, err := cfg.FacebookUserInfo(context.Background(), tok)
	if err != nil {
		t.Fatalf("userinfo: %v", err)
	}
	if ident.ProviderUserID != "fb-777" || ident.RealName != "Aung Kyaw" ||
		ident.Email != "ak@example.com" || ident.AvatarURL != "https://cdn.example/fb-777.jpg" {
		t.Fatalf("identity wrong: %+v", ident)
	}
}

func TestFacebookExchangeFailures(t *testing.T) {
	ok := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":`))
	}))
	defer srv.Close()
	cfg := &Config{FacebookAppID: "a", FacebookAppSecret: "s", FacebookGraphBase: srv.URL}
	if _, err := cfg.FacebookExchange(context.Background(), "x", "r"); err == nil {
		t.Fatal("expected upstream error on 500")
	}
	ok = true
	if _, err := cfg.FacebookExchange(context.Background(), "x", "r"); err == nil {
		t.Fatal("expected error on malformed token body")
	}
	if _, err := cfg.FacebookUserInfo(context.Background(), "t"); err == nil {
		t.Fatal("expected userinfo upstream error")
	}
	empty := &Config{}
	if _, err := empty.FacebookExchange(context.Background(), "x", "r"); err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
	if empty.FacebookStartURL("r", "s") != "" {
		t.Fatal("unconfigured start url must be empty")
	}
}

func TestTikTokFlow(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token/", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostFormValue("client_key") != "tt-key" || r.PostFormValue("code") != "tt-code" ||
			r.PostFormValue("code_verifier") != "verifier-43-chars-long-aaaaaaaaaaaaaaaaaaaa" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"tt-token"}`))
	})
	mux.HandleFunc("/user/info/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tt-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"user":{"open_id":"tt-42","display_name":"Ma Ma","avatar_url":"https://cdn.example/tt.jpg"}}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cfg := &Config{
		TikTokClientKey: "tt-key", TikTokClientSecret: "tt-secret",
		TikTokAPIBase: srv.URL, TikTokAuthBase: srv.URL + "/auth/authorize/",
	}
	start := cfg.TikTokStartURL("http://app/cb", "st", "challenge")
	if !strings.Contains(start, "code_challenge=challenge") ||
		!strings.Contains(start, "code_challenge_method=S256") {
		t.Fatalf("authorize url: %s", start)
	}
	tok, err := cfg.TikTokExchange(context.Background(), "tt-code", "http://app/cb",
		"verifier-43-chars-long-aaaaaaaaaaaaaaaaaaaa")
	if err != nil || tok != "tt-token" {
		t.Fatalf("exchange: %v %q", err, tok)
	}
	ident, err := cfg.TikTokUserInfo(context.Background(), tok)
	if err != nil || ident.ProviderUserID != "tt-42" || ident.RealName != "Ma Ma" {
		t.Fatalf("userinfo: %v %+v", err, ident)
	}
}

func TestTikTokFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	cfg := &Config{TikTokClientKey: "k", TikTokClientSecret: "s", TikTokAPIBase: srv.URL}
	if _, err := cfg.TikTokExchange(context.Background(), "c", "r", "v"); err == nil {
		t.Fatal("expected upstream error")
	}
	_, err := cfg.TikTokUserInfo(context.Background(), "t")
	if err == nil {
		t.Fatal("expected userinfo error")
	}
	empty := &Config{}
	if _, err := empty.TikTokExchange(context.Background(), "c", "r", "v"); err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
	if empty.TikTokStartURL("r", "s", "ch") != "" {
		t.Fatal("unconfigured start url must be empty")
	}
}

func TestTelegramVerify(t *testing.T) {
	cfg := &Config{TelegramBotToken: "123:botsecret"}
	authDate := "4102444800" // 2100-01-01, far future relative to any test run
	fields := url.Values{
		"id":         {"555000111"},
		"first_name": {"Aung"},
		"last_name":  {"Kyaw"},
		"username":   {"aungk"},
		"photo_url":  {"https://t.me/i/userpic.jpg"},
		"auth_date":  {authDate},
	}
	fields.Set("hash", telegramHash("123:botsecret", map[string]string{
		"id": "555000111", "first_name": "Aung", "last_name": "Kyaw",
		"username": "aungk", "photo_url": "https://t.me/i/userpic.jpg", "auth_date": authDate,
	}))
	ident, err := cfg.TelegramVerify(fields)
	if err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if ident.ProviderUserID != "555000111" || ident.RealName != "Aung Kyaw" ||
		ident.AvatarURL != "https://t.me/i/userpic.jpg" {
		t.Fatalf("identity wrong: %+v", ident)
	}

	// Tampered hash must fail.
	tampered := url.Values{}
	for k, v := range fields {
		tampered[k] = v
	}
	tampered.Set("hash", "0000000000000000000000000000000000000000000000000000000000000000")
	if _, err := cfg.TelegramVerify(tampered); err == nil {
		t.Fatal("tampered hash accepted")
	}

	// Stale auth_date must fail.
	stale := url.Values{}
	for k, v := range fields {
		stale[k] = v
	}
	stale.Set("auth_date", "1000000000") // 2001
	stale.Set("hash", telegramHash("123:botsecret", map[string]string{
		"id": "555000111", "first_name": "Aung", "last_name": "Kyaw",
		"username": "aungk", "photo_url": "https://t.me/i/userpic.jpg", "auth_date": "1000000000",
	}))
	if _, err := cfg.TelegramVerify(stale); err == nil {
		t.Fatal("stale payload accepted")
	}

	// Missing fields must fail.
	if _, err := cfg.TelegramVerify(url.Values{"id": {"1"}}); err == nil {
		t.Fatal("incomplete payload accepted")
	}
	// Unconfigured provider must fail with the sentinel.
	if _, err := (&Config{}).TelegramVerify(fields); err != ErrNotConfigured {
		t.Fatalf("want ErrNotConfigured, got %v", err)
	}
}

func TestEnabled(t *testing.T) {
	full := &Config{FacebookAppID: "a", FacebookAppSecret: "s", TikTokClientKey: "k",
		TikTokClientSecret: "s", TelegramBotToken: "t"}
	for _, p := range []string{Facebook, TikTok, Telegram} {
		if !full.Enabled(p) {
			t.Fatalf("%s should be enabled", p)
		}
	}
	empty := &Config{}
	for _, p := range []string{Facebook, TikTok, Telegram, "unknown"} {
		if empty.Enabled(p) {
			t.Fatalf("%s should not be enabled", p)
		}
	}
	if full.Enabled("unknown") {
		t.Fatal("unknown provider must be disabled")
	}
}

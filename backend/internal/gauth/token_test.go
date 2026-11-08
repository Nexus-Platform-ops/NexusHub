package gauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func testSAJSON(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sa, _ := json.Marshal(map[string]string{
		"client_email": "svc@example.iam.gserviceaccount.com",
		"private_key":  string(pemKey),
	})
	return string(sa)
}

func TestTokenSourceGrantAndCache(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant_type = %q", got)
		}
		assertion := r.Form.Get("assertion")
		parts := strings.Split(assertion, ".")
		if len(parts) != 3 {
			t.Fatalf("assertion is not a JWT: %q", assertion)
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig); err != nil {
			t.Errorf("assertion signature invalid: %v", err)
		}
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		if !strings.Contains(string(claims), `"iss":"svc@example.iam.gserviceaccount.com"`) {
			t.Errorf("claims missing iss: %s", claims)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.test-token", "expires_in": 3600, "token_type": "Bearer",
		})
	}))
	defer srv.Close()

	saJSON := strings.Replace(testSAJSON(t, key),
		`"token_uri":""`, `"token_uri":""`, 1)
	// Point the SA at the test server by rewriting the JSON.
	var m map[string]any
	if err := json.Unmarshal([]byte(saJSON), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m["token_uri"] = srv.URL
	raw, _ := json.Marshal(m)

	ts, err := NewTokenSourceClient(string(raw), "scope", srv.Client())
	if err != nil {
		t.Fatalf("NewTokenSource: %v", err)
	}
	tok, err := ts.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "ya29.test-token" {
		t.Fatalf("token = %q", tok)
	}
	if _, err := ts.Token(); err != nil {
		t.Fatalf("cached Token: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("token endpoint called %d times, want 1 (cache miss)", got)
	}
}

func TestNewTokenSourceRejectsGarbage(t *testing.T) {
	if _, err := NewTokenSource("{not json", "scope"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := NewTokenSource(`{"client_email":"a@b.c"}`, "scope"); err == nil {
		t.Fatal("missing private_key accepted")
	}
	if _, err := NewTokenSource(`{"client_email":"a@b.c","private_key":"-----BEGIN PRIVATE KEY-----\nZ2FyYmFnZQ==\n-----END PRIVATE KEY-----"}`, "scope"); err == nil {
		t.Fatal("non-PEM private key accepted")
	}
}

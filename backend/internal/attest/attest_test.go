package attest

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLogVerifier(t *testing.T) {
	v := New(Config{Provider: Log})
	if v.Provider() != Log {
		t.Fatalf("provider = %q", v.Provider())
	}
	res, err := v.Verify(context.Background(), "dev:pixel8", "nonce-1")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.DeviceID != "pixel8" || res.Nonce != "nonce-1" || res.Verdict != "dev-ok" {
		t.Fatalf("result = %+v", res)
	}
	for _, token := range []string{"bogus", "dev:", ""} {
		if _, err := v.Verify(context.Background(), token, "nonce-1"); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("token %q: err = %v, want ErrInvalidToken", token, err)
		}
	}
	if _, err := v.Verify(context.Background(), "dev:pixel8", ""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("empty nonce: err = %v", err)
	}
}

func TestNewDispatch(t *testing.T) {
	if _, ok := New(Config{Provider: "unknown"}).(logVerifier); !ok {
		t.Fatal("unknown provider must fall back to the log stub")
	}
	if _, err := New(Config{Provider: AppAttest}).Verify(context.Background(), "x", "n"); !errors.Is(err, ErrUnavailable) {
		t.Fatal("app attest must report unavailable until implemented")
	}
	if _, err := New(Config{Provider: PlayIntegrity, ServiceAccount: "{not json"}).
		Verify(context.Background(), "x", "n"); err == nil {
		t.Fatal("broken play integrity config must fail loudly")
	}
}

// TestPlayIntegrityVerify drives the full Google path against httptest:
// SA JWT token exchange, then the decodeIntegrityToken verdict check.
func TestPlayIntegrityVerify(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, _ := x509.MarshalPKCS8PrivateKey(key)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if !strings.HasPrefix(r.Form.Get("assertion"), "eyJ") {
			t.Errorf("assertion missing: %q", r.Form.Get("assertion"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.attest", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	var auth string
	mux.HandleFunc("/v1/mm.nexushub.nexus_app:decodeIntegrityToken",
		func(w http.ResponseWriter, r *http.Request) {
			auth = r.Header.Get("Authorization")
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{
			  "requestDetails": {"requestPackageName": "mm.nexushub.nexus_app",
			    "nonce": %q, "timestampMillis": 1700000000000},
			  "appIntegrity": {"appRecognitionVerdict": "PLAY_RECOGNIZED",
			    "packageName": "mm.nexushub.nexus_app",
			    "certificateSha256Digest": ["cert-digest-1"]},
			  "deviceIntegrity": {"deviceRecognitionVerdict": ["MEETS_DEVICE_INTEGRITY"]}
			}`, body["integrityToken"])
		})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	sa, _ := json.Marshal(map[string]string{
		"client_email": "svc@example.iam.gserviceaccount.com",
		"private_key":  string(pemKey),
		"token_uri":    srv.URL + "/token",
	})
	v := New(Config{
		Provider:       PlayIntegrity,
		PackageName:    "mm.nexushub.nexus_app",
		ServiceAccount: string(sa),
		Client:         srv.Client(),
		APIBase:        srv.URL,
	})

	// Verdict echoes our token as the nonce; a matching nonce passes.
	res, err := v.Verify(context.Background(), "nonce-abc", "nonce-abc")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.Provider != PlayIntegrity || res.DeviceID != "cert-digest-1" || res.AppID != "mm.nexushub.nexus_app" {
		t.Fatalf("result = %+v", res)
	}
	if auth != "Bearer ya29.attest" {
		t.Fatalf("authorization = %q", auth)
	}

	// Nonce mismatch fails closed.
	if _, err := v.Verify(context.Background(), "nonce-abc", "different"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("nonce mismatch: err = %v", err)
	}
}

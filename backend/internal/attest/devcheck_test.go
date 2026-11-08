package attest

// DeviceCheck verifier tests: JWT client-auth shape, token validation
// mapping (200 ok / 400 invalid / other upstream) and config failures.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testP8(t *testing.T) (string, *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})), key
}

func deviceCheckServer(t *testing.T, status int, saw *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/validate_device_token" {
			t.Errorf("path = %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			t.Errorf("missing bearer auth")
		}
		parts := strings.Split(strings.TrimPrefix(auth, "Bearer "), ".")
		if len(parts) != 3 {
			t.Fatalf("jwt parts = %d", len(parts))
		}
		hdr, _ := base64.RawURLEncoding.DecodeString(parts[0])
		var header map[string]string
		if err := json.Unmarshal(hdr, &header); err != nil {
			t.Fatalf("jwt header: %v", err)
		}
		if header["alg"] != "ES256" || header["kid"] != "KEY1" {
			t.Errorf("jwt header = %v", header)
		}
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		var claimsMap map[string]any
		if err := json.Unmarshal(claims, &claimsMap); err != nil {
			t.Fatalf("jwt claims: %v", err)
		}
		if claimsMap["iss"] != "TEAM1" {
			t.Errorf("iss = %v", claimsMap["iss"])
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("body: %v", err)
		}
		*saw = body
		w.WriteHeader(status)
	}))
}

func TestDeviceCheckVerifyOK(t *testing.T) {
	p8, _ := testP8(t)
	var saw map[string]any
	srv := deviceCheckServer(t, http.StatusOK, &saw)
	defer srv.Close()

	v := New(Config{
		Provider:      DeviceCheck,
		AppleTeamID:   "TEAM1",
		AppleKeyID:    "KEY1",
		AppleBundleID: "mm.nexushub.nexus_app",
		AppleP8:       p8,
		APIBase:       srv.URL,
		Timeout:       5 * time.Second,
	})
	token := base64.StdEncoding.EncodeToString(make([]byte, 32))
	res, err := v.Verify(context.Background(), token, "nonce-123")
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if res.Provider != DeviceCheck || res.AppID != "mm.nexushub.nexus_app" ||
		res.Nonce != "nonce-123" || res.Verdict != "devicecheck-ok" {
		t.Fatalf("result: %+v", res)
	}
	if saw["device_token"] != token || saw["transaction_id"] != "nonce-123" {
		t.Fatalf("request body: %v", saw)
	}
	if saw["timestamp"] == "" {
		t.Fatalf("timestamp missing: %v", saw)
	}
}

func TestDeviceCheckInvalidToken(t *testing.T) {
	p8, _ := testP8(t)
	var saw map[string]any
	srv := deviceCheckServer(t, http.StatusBadRequest, &saw)
	defer srv.Close()

	v := New(Config{
		Provider:      DeviceCheck,
		AppleTeamID:   "TEAM1",
		AppleKeyID:    "KEY1",
		AppleBundleID: "mm.nexushub.nexus_app",
		AppleP8:       p8,
		APIBase:       srv.URL,
	}).(*deviceCheckVerifier)

	// Apple answered 400: token rejected.
	if _, err := v.Verify(context.Background(),
		base64.StdEncoding.EncodeToString(make([]byte, 32)), "n"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
	// Structurally invalid token (not 32 bytes) fails before any call.
	if _, err := v.Verify(context.Background(), "aGk=", "n"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken for malformed token, got %v", err)
	}
	if saw["transaction_id"] != "n" {
		t.Fatalf("expected exactly one upstream call, body %v", saw)
	}
}

func TestDeviceCheckUpstreamError(t *testing.T) {
	p8, _ := testP8(t)
	var saw map[string]any
	srv := deviceCheckServer(t, http.StatusUnauthorized, &saw)
	defer srv.Close()

	v := New(Config{
		Provider:      DeviceCheck,
		AppleTeamID:   "TEAM1",
		AppleKeyID:    "KEY1",
		AppleBundleID: "mm.nexushub.nexus_app",
		AppleP8:       p8,
		APIBase:       srv.URL,
	})
	_, err := v.Verify(context.Background(),
		base64.StdEncoding.EncodeToString(make([]byte, 32)), "n")
	if err == nil || errors.Is(err, ErrInvalidToken) {
		t.Fatalf("401 from Apple is an upstream/auth error, got %v", err)
	}
}

func TestDeviceCheckMisconfig(t *testing.T) {
	// Garbage PEM → broken verifier whose Verify always errors.
	v := New(Config{Provider: DeviceCheck, AppleP8: "not a pem"})
	if _, err := v.Verify(context.Background(), "x", "n"); err == nil {
		t.Fatalf("broken verifier must fail")
	}
	// Valid PEM but missing ids → broken verifier.
	p8, _ := testP8(t)
	v = New(Config{Provider: DeviceCheck, AppleP8: p8})
	if _, err := v.Verify(context.Background(), "x", "n"); err == nil {
		t.Fatalf("broken verifier must fail without team/key/bundle ids")
	}
}

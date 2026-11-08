// Package gauth exchanges a Google service account JSON key for OAuth2
// access tokens (JWT bearer grant). It is shared by the GCP KMS master key
// adapter and the Play Integrity attestation verifier, keeping the Google
// SDK dependency out of the binary.
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const tokenEndpoint = "https://oauth2.googleapis.com/token"

type serviceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// TokenSource yields short-lived OAuth2 tokens, caching them until shortly
// before expiry. It is safe for concurrent use. The HTTP client is
// injectable for tests.
type TokenSource struct {
	mu        sync.Mutex
	sa        serviceAccount
	key       *rsa.PrivateKey
	scope     string
	client    *httpClient
	token     string
	expiresAt time.Time
	now       func() time.Time
}

// NewTokenSource parses a service account JSON key for the given OAuth scope.
func NewTokenSource(saJSON, scope string) (*TokenSource, error) {
	var sa serviceAccount
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		return nil, fmt.Errorf("gauth: parse service account JSON: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("gauth: service account JSON missing client_email or private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = tokenEndpoint
	}
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return nil, errors.New("gauth: private_key is not valid PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Older SA keys are PKCS#1.
		if key, err = x509.ParsePKCS1PrivateKey(block.Bytes); err != nil {
			return nil, fmt.Errorf("gauth: parse private key: %w", err)
		}
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("gauth: private key is not RSA")
	}
	return &TokenSource{sa: sa, key: rsaKey, scope: scope, client: &httpClient{}, now: time.Now}, nil
}

// NewTokenSourceClient is NewTokenSource with an injected transport (tests).
func NewTokenSourceClient(saJSON, scope string, client *http.Client) (*TokenSource, error) {
	ts, err := NewTokenSource(saJSON, scope)
	if err != nil {
		return nil, err
	}
	ts.client = &httpClient{client: client}
	return ts, nil
}

// Token returns a valid access token, refreshing it when expired.
func (ts *TokenSource) Token() (string, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.token != "" && ts.now().Before(ts.expiresAt.Add(-2*time.Minute)) {
		return ts.token, nil
	}
	tok, expiry, err := ts.fetch()
	if err != nil {
		return "", err
	}
	ts.token, ts.expiresAt = tok, expiry
	return tok, nil
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (ts *TokenSource) fetch() (string, time.Time, error) {
	now := ts.now()
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims := b64([]byte(fmt.Sprintf(
		`{"iss":%q,"scope":%q,"aud":%q,"iat":%d,"exp":%d}`,
		ts.sa.ClientEmail, ts.scope, ts.sa.TokenURI, now.Unix(), now.Add(time.Hour).Unix())))
	digest := sha256.Sum256([]byte(header + "." + claims))
	sig, err := rsa.SignPKCS1v15(rand.Reader, ts.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("gauth: sign assertion: %w", err)
	}
	assertion := header + "." + claims + "." + b64(sig)
	form := "grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + assertion
	resp, err := ts.client.PostForm(ts.sa.TokenURI, form)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("gauth: token exchange: %w", err)
	}
	if resp.StatusCode != 200 {
		return "", time.Time{}, fmt.Errorf("gauth: token exchange status %d: %s", resp.StatusCode, strings.TrimSpace(string(resp.Body)))
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(resp.Body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", time.Time{}, fmt.Errorf("gauth: decode token response: %v", err)
	}
	return parsed.AccessToken, now.Add(time.Duration(parsed.ExpiresIn) * time.Second), nil
}

type httpResponse struct {
	StatusCode int
	Body       []byte
}

type httpClient struct{ client *http.Client }

func (c *httpClient) PostForm(url, form string) (*httpResponse, error) {
	client := c.client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Post(url, "application/x-www-form-urlencoded", strings.NewReader(form))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if err != nil {
		return nil, err
	}
	return &httpResponse{StatusCode: resp.StatusCode, Body: body}, nil
}

// ParseServiceAccountEmail exposes the SA client email for logs/config
// validation without revealing key material.
func ParseServiceAccountEmail(saJSON string) (string, error) {
	var sa serviceAccount
	if err := json.Unmarshal([]byte(saJSON), &sa); err != nil {
		return "", fmt.Errorf("gauth: parse service account JSON: %w", err)
	}
	if !strings.Contains(sa.ClientEmail, "@") {
		return "", errors.New("gauth: invalid client_email")
	}
	return sa.ClientEmail, nil
}

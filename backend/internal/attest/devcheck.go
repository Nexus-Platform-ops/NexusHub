package attest

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
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
	"time"
)

// deviceCheckVerifier validates Apple DeviceCheck tokens against
// api.devicecheck.apple.com. The JWT client auth is hand-rolled (ES256 JWS
// over a PKCS#8 private key) to match the no-heavy-SDK policy elsewhere.
type deviceCheckVerifier struct {
	teamID   string
	keyID    string
	bundleID string
	key      *ecdsa.PrivateKey
	client   *http.Client
	timeout  time.Duration
	apiBase  string // tests only; default https://api.devicecheck.apple.com
}

func newDeviceCheck(c Config) Verifier {
	block, _ := pem.Decode([]byte(c.AppleP8))
	if block == nil || block.Type != "PRIVATE KEY" {
		return brokenVerifier{provider: DeviceCheck, err: errors.New("attestation devicecheck: APPLE_P8 must be a PEM PKCS#8 private key")}
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return brokenVerifier{provider: DeviceCheck, err: fmt.Errorf("attestation devicecheck: parse APPLE_P8: %w", err)}
	}
	ecKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return brokenVerifier{provider: DeviceCheck, err: errors.New("attestation devicecheck: APPLE_P8 must be an EC (ES256) key")}
	}
	if c.AppleTeamID == "" || c.AppleKeyID == "" || c.AppleBundleID == "" {
		return brokenVerifier{provider: DeviceCheck, err: errors.New("attestation devicecheck: APPLE_TEAM_ID, APPLE_KEY_ID and APPLE_BUNDLE_ID are required")}
	}
	return &deviceCheckVerifier{
		teamID:   c.AppleTeamID,
		keyID:    c.AppleKeyID,
		bundleID: c.AppleBundleID,
		key:      ecKey,
		client:   c.Client,
		timeout:  c.Timeout,
		apiBase:  c.APIBase,
	}
}

func (v *deviceCheckVerifier) Provider() string { return DeviceCheck }

func (v *deviceCheckVerifier) Verify(ctx context.Context, token, expectedNonce string) (*Result, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrInvalidToken
	}
	jwt, err := v.signJWT()
	if err != nil {
		return nil, err
	}
	body, _ := json.Marshal(map[string]string{
		"device_token":   token,
		"transaction_id": expectedNonce,
		"timestamp":      time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	})
	base := v.apiBase
	if base == "" {
		base = "https://api.devicecheck.apple.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/v1/validate_device_token", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Content-Type", "application/json")
	client := v.client
	if client == nil {
		timeout := v.timeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("devicecheck validate: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	// Apple answers 200 for a valid token and 400 for an invalid/expired
	// one; anything else (401/5xx) is an upstream/auth problem, not a token
	// verdict, so it is not mapped to ErrInvalidToken.
	if resp.StatusCode == http.StatusOK {
		return &Result{
			Provider:  DeviceCheck,
			DeviceID:  fmt.Sprintf("%x", sha256.Sum256(raw)),
			AppID:     v.bundleID,
			Nonce:     expectedNonce,
			Verdict:   "devicecheck-ok",
			Timestamp: time.Now(),
		}, nil
	}
	if resp.StatusCode == http.StatusBadRequest {
		return nil, ErrInvalidToken
	}
	return nil, fmt.Errorf("devicecheck validate: status %d", resp.StatusCode)
}

// signJWT builds the ES256 client token: header {alg, kid}, claims
// {iss: teamID, iat}. Apple requires no audience for this endpoint.
func (v *deviceCheckVerifier) signJWT() (string, error) {
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": v.keyID, "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{"iss": v.teamID, "iat": time.Now().Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, v.key, digest[:])
	if err != nil {
		return "", fmt.Errorf("devicecheck jwt sign: %w", err)
	}
	// JWS ES256 wants raw R||S (32 bytes each), not ASN.1 DER.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// Package attest verifies device integrity tokens: Google Play Integrity
// verdicts on Android and (planned) Apple App Attest on iOS.
//
// Verification proves a request comes from the genuine app on a genuine
// device. The API binds each verification to a single-use server nonce so a
// stolen token cannot be replayed later from a different session.
package attest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nexus/backend/internal/gauth"
)

// Providers accepted by Config.Provider.
const (
	Log           = "log"           // development stub
	PlayIntegrity = "playintegrity" // Google Play Integrity API
	DeviceCheck   = "devicecheck"   // Apple DeviceCheck token validation
	AppAttest     = "appattest"     // Apple App Attest DCN (planned)
)

// Result is a successfully verified attestation.
type Result struct {
	Provider  string
	DeviceID  string
	AppID     string
	Nonce     string
	Verdict   string
	Timestamp time.Time
}

// Verifier checks an attestation token.
type Verifier interface {
	Provider() string
	// Verify validates token and binds it to the server-issued nonce.
	Verify(ctx context.Context, token, expectedNonce string) (*Result, error)
}

// ErrInvalidToken means the token is structurally invalid or its verdict
// failed. Callers map it to 400; upstream failures map to 502.
var ErrInvalidToken = errors.New("attestation token invalid")

// ErrUnavailable means the provider is not configured/implemented.
var ErrUnavailable = errors.New("attestation provider unavailable")

// Config configures the verifier. Provider "log" (development) only accepts
// tokens of the form "dev:<device-id>".
type Config struct {
	Provider       string
	PackageName    string // Android application id / iOS bundle id
	ServiceAccount string // Google service account JSON (Play Integrity)
	Timeout        time.Duration
	Client         *http.Client // tests inject an httptest client
	APIBase        string       // tests only; default Google/Apple endpoint

	// Apple DeviceCheck: team + key id, iOS bundle id and the .p8 private
	// key (PEM PKCS#8, ES256) used to sign the client JWT.
	AppleTeamID   string
	AppleKeyID    string
	AppleBundleID string
	AppleP8       string
}

// New builds the configured verifier; unknown providers fall back to the
// log stub so a zero-value config never breaks local development.
func New(c Config) Verifier {
	switch strings.ToLower(strings.TrimSpace(c.Provider)) {
	case PlayIntegrity:
		return newPlayIntegrity(c)
	case DeviceCheck:
		return newDeviceCheck(c)
	case AppAttest:
		return appAttestVerifier{}
	default:
		return logVerifier{}
	}
}

// logVerifier is the development stub: it proves the endpoint flow (nonce
// issue -> verify -> record) without vendor credentials. Tokens must look
// like "dev:<device-id>".
type logVerifier struct{}

func (logVerifier) Provider() string { return Log }

func (logVerifier) Verify(_ context.Context, token, nonce string) (*Result, error) {
	device, ok := strings.CutPrefix(token, "dev:")
	if !ok || device == "" || nonce == "" {
		return nil, ErrInvalidToken
	}
	return &Result{
		Provider:  Log,
		DeviceID:  device,
		AppID:     "dev.nexus",
		Nonce:     nonce,
		Verdict:   "dev-ok",
		Timestamp: time.Now(),
	}, nil
}

// playIntegrityVerifier calls the Play Integrity API decodeIntegrityToken
// endpoint and checks the verdict: recognized app, matching package name,
// matching server nonce, and a passing device-integrity verdict.
type playIntegrityVerifier struct {
	packageName string
	ts          *gauth.TokenSource
	client      *http.Client
	timeout     time.Duration
	apiBase     string
}

func newPlayIntegrity(c Config) Verifier {
	ts, err := gauth.NewTokenSourceClient(c.ServiceAccount,
		"https://www.googleapis.com/auth/playintegrity", c.Client)
	if err != nil {
		// Misconfigured credentials must be loud at startup-adjacent time:
		// every verification fails instead of silently passing.
		return brokenVerifier{provider: PlayIntegrity, err: fmt.Errorf("play integrity auth: %w", err)}
	}
	return &playIntegrityVerifier{
		packageName: c.PackageName,
		ts:          ts,
		client:      c.Client,
		timeout:     c.Timeout,
		apiBase:     c.APIBase,
	}
}

type brokenVerifier struct {
	provider string
	err      error
}

func (b brokenVerifier) Provider() string { return b.provider }
func (b brokenVerifier) Verify(context.Context, string, string) (*Result, error) {
	return nil, b.err
}

type integrityVerdict struct {
	RequestDetails struct {
		RequestPackageName string `json:"requestPackageName"`
		Nonce              string `json:"nonce"`
		TimestampMillis    int64  `json:"timestampMillis"`
	} `json:"requestDetails"`
	AppIntegrity struct {
		AppRecognitionVerdict string   `json:"appRecognitionVerdict"`
		PackageName           string   `json:"packageName"`
		CertificateSHA256     []string `json:"certificateSha256Digest"`
	} `json:"appIntegrity"`
	DeviceIntegrity struct {
		DeviceRecognitionVerdict []string `json:"deviceRecognitionVerdict"`
	} `json:"deviceIntegrity"`
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (v *playIntegrityVerifier) Provider() string { return PlayIntegrity }

func (v *playIntegrityVerifier) Verify(ctx context.Context, token, expectedNonce string) (*Result, error) {
	base := v.apiBase
	if base == "" {
		base = "https://playintegrity.googleapis.com"
	}
	url := fmt.Sprintf("%s/v1/%s:decodeIntegrityToken", base, v.packageName)
	body, _ := json.Marshal(map[string]string{"integrityToken": token})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	access, err := v.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("play integrity auth: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+access)
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
		return nil, fmt.Errorf("play integrity decode: %w", err)
	}
	defer resp.Body.Close()
	var verdict integrityVerdict
	dec := json.NewDecoder(io.LimitReader(resp.Body, 1<<20))
	if err := dec.Decode(&verdict); err != nil {
		return nil, fmt.Errorf("play integrity decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("play integrity decode: status %d: %s", resp.StatusCode, verdict.Error.Message)
	}
	if verdict.AppIntegrity.AppRecognitionVerdict != "PLAY_RECOGNIZED" ||
		verdict.AppIntegrity.PackageName != v.packageName {
		return nil, ErrInvalidToken
	}
	// The client submits the server nonce verbatim in the Play Integrity
	// request; the verdict echoes it. Equality is the binding check.
	if verdict.RequestDetails.Nonce != expectedNonce {
		return nil, ErrInvalidToken
	}
	devices := verdict.DeviceIntegrity.DeviceRecognitionVerdict
	if len(devices) == 0 {
		return nil, ErrInvalidToken
	}
	for _, d := range devices {
		if d != "MEETS_DEVICE_INTEGRITY" && d != "MEETS_BASIC_INTEGRITY" {
			return nil, ErrInvalidToken
		}
	}
	return &Result{
		Provider:  PlayIntegrity,
		DeviceID:  strings.Join(verdict.AppIntegrity.CertificateSHA256, ","),
		AppID:     verdict.AppIntegrity.PackageName,
		Nonce:     expectedNonce,
		Verdict:   strings.Join(devices, ","),
		Timestamp: time.UnixMilli(verdict.RequestDetails.TimestampMillis),
	}, nil
}

// appAttestVerifier is a documented stub: real App Attest verification
// requires parsing the Apple App Attest DCN (CBOR) and chaining it to the
// pinned Apple Root CA - G3, which is not implemented yet. The endpoint
// answers 501 while this provider is selected.
type appAttestVerifier struct{}

func (appAttestVerifier) Provider() string { return AppAttest }

func (appAttestVerifier) Verify(context.Context, string, string) (*Result, error) {
	return nil, fmt.Errorf("%w: app attest DCN verification not yet implemented (requires Apple root CA pinning)", ErrUnavailable)
}

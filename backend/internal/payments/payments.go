// Package payments creates charges through a provider adapter and verifies
// the signed callbacks the provider sends when a payment completes.
//
// The adapter is deliberately thin: the API owns coin packages, pricing,
// intent state and ledger crediting, so swapping the Myanmar payment vendor
// (KBZPay, Wave Money, a PSP aggregator, ...) only changes the endpoint and
// the charge payload, never money handling. Providers with a different wire
// format get a Sender-like adapter here.
package payments

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Provider identifiers accepted by Config.Provider.
const (
	Log  = "log"
	HTTP = "http"

	Currency                = "MMK"
	DefaultWebhookTolerance = 5 * time.Minute
)

// Sentinel errors mapped to HTTP responses by the API layer.
var (
	// ErrUpstream — the charge could not be created at the provider.
	ErrUpstream = errors.New("payment provider request failed")
	// ErrBadSignature — missing/non-hex/mismatched webhook signature.
	ErrBadSignature = errors.New("invalid webhook signature")
	// ErrStaleTimestamp — the signed timestamp is outside the replay window.
	ErrStaleTimestamp = errors.New("stale webhook timestamp")
)

// CoinPackage is a server-priced top-up. The client sends only the id; the
// coin amount and MMK price can never be chosen by the client.
type CoinPackage struct {
	ID       string
	Coins    int64
	PriceMMK int64
}

// Packages is the authoritative price list. Changing prices is a code change
// plus a release, never a client-side calculation.
var Packages = []CoinPackage{
	{ID: "coins_100", Coins: 100, PriceMMK: 2000},
	{ID: "coins_500", Coins: 500, PriceMMK: 9000},
	{ID: "coins_1200", Coins: 1200, PriceMMK: 20000},
	{ID: "coins_2500", Coins: 2500, PriceMMK: 40000},
}

// FindPackage looks up a package by id.
func FindPackage(id string) (CoinPackage, bool) {
	for _, p := range Packages {
		if p.ID == id {
			return p, true
		}
	}
	return CoinPackage{}, false
}

// ChargeRequest is the provider-agnostic charge to create for an intent.
type ChargeRequest struct {
	IntentID  string
	AmountMMK int64
	Currency  string
	ReturnURL string
}

// Charge is the provider's answer: its own reference and the hosted checkout
// URL the client opens.
type Charge struct {
	ProviderRef string
	PaymentURL  string
}

// Provider creates charges at the payment vendor.
type Provider interface {
	CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error)
}

// Config configures the provider adapter.
type Config struct {
	Provider   string
	APIURL     string // full charge endpoint, https:// outside development
	APIKey     string
	Timeout    time.Duration
	HTTPClient *http.Client // tests inject an httptest client
}

// New returns the configured provider. It never returns nil.
func New(c Config) Provider {
	switch c.Provider {
	case HTTP:
		return newHTTP(c)
	case Stripe:
		return NewStripe(StripeConfig{
			SecretKey:  c.APIKey,
			APIBase:    c.APIURL,
			HTTPClient: c.HTTPClient,
			Timeout:    c.Timeout,
		})
	}
	return logProvider{}
}

// logProvider creates intents without a checkout URL. It exists for
// development and tests; production validation requires the http provider.
type logProvider struct{}

func (logProvider) CreateCharge(context.Context, ChargeRequest) (Charge, error) {
	return Charge{}, nil
}

type httpProvider struct {
	url     string
	apiKey  string
	timeout time.Duration
	client  *http.Client
}

func newHTTP(c Config) *httpProvider {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	client := c.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &httpProvider{url: c.APIURL, apiKey: c.APIKey, timeout: timeout, client: client}
}

func (p *httpProvider) CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error) {
	payload, err := json.Marshal(map[string]any{
		"intent_id":  req.IntentID,
		"reference":  req.IntentID,
		"amount_mmk": req.AmountMMK,
		"currency":   req.Currency,
		"return_url": req.ReturnURL,
	})
	if err != nil {
		return Charge{}, fmt.Errorf("%w: encode payload: %v", ErrUpstream, err)
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(payload))
	if err != nil {
		return Charge{}, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return Charge{}, fmt.Errorf("%w: request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Charge{}, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Charge{}, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	var out struct {
		ID         string `json:"id"`
		PaymentURL string `json:"payment_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.PaymentURL == "" {
		return Charge{}, fmt.Errorf("%w: charge response", ErrUpstream)
	}
	return Charge{ProviderRef: out.ID, PaymentURL: out.PaymentURL}, nil
}

// SignWebhook returns hex(HMAC-SHA256(secret, timestamp + "." + body)).
// The timestamp is the exact header value, which makes the signature
// independent of how the receiver parses numbers.
func SignWebhook(secret, timestamp string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifyWebhook checks the signature and freshness of a provider callback.
// The timestamp is unix seconds; signatures older or newer than the
// tolerance are rejected. Replays inside the window are safe because intent
// crediting is idempotent.
func VerifyWebhook(secret, timestamp, signature string, body []byte, now time.Time) error {
	timestamp = strings.TrimSpace(timestamp)
	signature = strings.ToLower(strings.TrimSpace(signature))
	if timestamp == "" || signature == "" {
		return ErrBadSignature
	}
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	skew := now.Sub(time.Unix(ts, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > DefaultWebhookTolerance {
		return ErrStaleTimestamp
	}
	expected := SignWebhook(secret, timestamp, body)
	if hmac.Equal([]byte(expected), []byte(signature)) != true {
		return ErrBadSignature
	}
	return nil
}

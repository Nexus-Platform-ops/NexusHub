package payments

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
	"strconv"
	"strings"
	"time"
)

const (
	Stripe          = "stripe"
	stripeAPIBase   = "https://api.stripe.com/v1"
	stripeVersion   = "2024-12-18.acacia"
)

// StripeConfig holds Stripe-specific settings.
type StripeConfig struct {
	SecretKey     string // sk_live_... or sk_test_...
	WebhookSecret string // whsec_...
	APIBase       string // override for testing
	HTTPClient    *http.Client
	Timeout       time.Duration
}

type stripeProvider struct {
	cfg    StripeConfig
	client *http.Client
	base   string
}

// NewStripe returns a Provider backed by Stripe's PaymentIntent API.
func NewStripe(cfg StripeConfig) Provider {
	base := cfg.APIBase
	if base == "" {
		base = stripeAPIBase
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &stripeProvider{cfg: cfg, client: client, base: base}
}

// CreateCharge builds a Stripe PaymentIntent and returns the client secret
// as the PaymentURL (the mobile SDK consumes it directly).
func (p *stripeProvider) CreateCharge(ctx context.Context, req ChargeRequest) (Charge, error) {
	form := url.Values{}
	form.Set("amount", strconv.FormatInt(req.AmountMMK*100, 10)) // Stripe uses smallest unit
	form.Set("currency", strings.ToLower(req.Currency))
	form.Set("metadata[intent_id]", req.IntentID)
	form.Set("metadata[reference]", req.IntentID)
	if req.ReturnURL != "" {
		form.Set("automatic_payment_methods[enabled]", "true")
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.base+"/payment_intents", strings.NewReader(form.Encode()))
	if err != nil {
		return Charge{}, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "Bearer "+p.cfg.SecretKey)
	httpReq.Header.Set("Stripe-Version", stripeVersion)
	httpReq.Header.Set("Idempotency-Key", req.IntentID)

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
		var stripeErr struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
				Code    string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(body, &stripeErr)
		if stripeErr.Error.Message != "" {
			return Charge{}, fmt.Errorf("%w: stripe: %s (%s)", ErrUpstream, stripeErr.Error.Message, stripeErr.Error.Code)
		}
		return Charge{}, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var pi struct {
		ID           string `json:"id"`
		ClientSecret string `json:"client_secret"`
		Status       string `json:"status"`
	}
	if err := json.Unmarshal(body, &pi); err != nil || pi.ID == "" {
		return Charge{}, fmt.Errorf("%w: decode response", ErrUpstream)
	}
	return Charge{
		ProviderRef: pi.ID,
		PaymentURL:  pi.ClientSecret,
	}, nil
}

// StripeWebhookEvent is a parsed Stripe webhook event.
type StripeWebhookEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// StripePaymentIntent is the relevant subset of a Stripe PaymentIntent object.
type StripePaymentIntent struct {
	ID         string            `json:"id"`
	Amount     int64             `json:"amount"`
	Currency   string            `json:"currency"`
	Status     string            `json:"status"`
	Metadata   map[string]string `json:"metadata"`
	ClientIP   string            `json:"client_ip,omitempty"`
	ChargeID   string            `json:"latest_charge,omitempty"`
}

// VerifyStripeWebhook checks the Stripe-Signature header and returns the
// parsed event. The header format is: t=<timestamp>,v1=<signature>.
func VerifyStripeWebhook(secret string, header string, body []byte) (*StripeWebhookEvent, error) {
	var (
		ts    int64
		sig   string
		found bool
	)
	for _, part := range strings.Split(header, ",") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
			found = true
		case "v1":
			sig = v
		}
	}
	if !found || sig == "" {
		return nil, ErrBadSignature
	}

	skew := time.Since(time.Unix(ts, 0))
	if skew < 0 {
		skew = -skew
	}
	if skew > DefaultWebhookTolerance {
		return nil, ErrStaleTimestamp
	}

	signedPayload := fmt.Sprintf("%d.%s", ts, string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return nil, ErrBadSignature
	}

	var evt StripeWebhookEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return nil, fmt.Errorf("decode event: %w", err)
	}
	return &evt, nil
}

// ParseStripePaymentIntent extracts a PaymentIntent from a webhook event's
// data.object field.
func ParseStripePaymentIntent(evt *StripeWebhookEvent) (*StripePaymentIntent, error) {
	var pi StripePaymentIntent
	if err := json.Unmarshal(evt.Data.Object, &pi); err != nil {
		return nil, fmt.Errorf("decode payment_intent: %w", err)
	}
	if pi.ID == "" {
		return nil, errors.New("missing payment_intent id")
	}
	return &pi, nil
}

// StripeStatusToIntentStatus maps Stripe PaymentIntent statuses to our
// internal intent statuses.
func StripeStatusToIntentStatus(stripeStatus string) string {
	switch stripeStatus {
	case "succeeded":
		return "paid"
	case "canceled", "requires_payment_method", "requires_confirmation":
		return "failed"
	case "processing", "requires_action":
		return "pending"
	default:
		return "pending"
	}
}

// StripeRefundRequest is the payload for creating a refund.
type StripeRefundRequest struct {
	PaymentIntentID string
	AmountMMK       int64  // 0 = full refund
	Reason          string // "duplicate", "fraudulent", "requested_by_customer"
}

// StripeRefund is the response from a refund creation.
type StripeRefund struct {
	ID     string
	Status string
	Amount int64
}

// CreateStripeRefund issues a refund against a Stripe PaymentIntent.
func CreateStripeRefund(ctx context.Context, cfg StripeConfig, req StripeRefundRequest) (*StripeRefund, error) {
	base := cfg.APIBase
	if base == "" {
		base = stripeAPIBase
	}

	form := url.Values{}
	form.Set("payment_intent", req.PaymentIntentID)
	if req.AmountMMK > 0 {
		form.Set("amount", strconv.FormatInt(req.AmountMMK*100, 10))
	}
	if req.Reason != "" {
		form.Set("reason", req.Reason)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		base+"/refunds", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Authorization", "Bearer "+cfg.SecretKey)
	httpReq.Header.Set("Stripe-Version", stripeVersion)

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%w: request: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read body: %v", ErrUpstream, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}

	var refund struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Amount int64  `json:"amount"`
	}
	if err := json.Unmarshal(body, &refund); err != nil {
		return nil, fmt.Errorf("%w: decode response", ErrUpstream)
	}
	return &StripeRefund{
		ID:     refund.ID,
		Status: refund.Status,
		Amount: refund.Amount / 100, // convert back from smallest unit
	}, nil
}

// StripeDisputeEvent represents a charge.dispute.created webhook event.
type StripeDisputeEvent struct {
	ID            string `json:"id"`
	ChargeID      string `json:"charge"`
	PaymentIntent string `json:"payment_intent"`
	Amount        int64  `json:"amount"`
	Currency      string `json:"currency"`
	Status        string `json:"status"`
	Reason        string `json:"reason"`
	Created       int64  `json:"created"`
}

// ParseStripeDispute extracts a dispute from a webhook event.
func ParseStripeDispute(evt *StripeWebhookEvent) (*StripeDisputeEvent, error) {
	var d StripeDisputeEvent
	if err := json.Unmarshal(evt.Data.Object, &d); err != nil {
		return nil, fmt.Errorf("decode dispute: %w", err)
	}
	return &d, nil
}

// SignStripeTestWebhook is a test helper that creates a valid Stripe webhook
// signature for the given payload. NOT for production use.
func SignStripeTestWebhook(secret string, body []byte, ts time.Time) string {
	signedPayload := fmt.Sprintf("%d.%s", ts.Unix(), string(body))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

// BuildStripeWebhookPayload constructs a test webhook payload for a
// payment_intent event.
func BuildStripeWebhookPayload(eventType, piID string, amount int64, currency, status string, metadata map[string]string) ([]byte, error) {
	evt := map[string]any{
		"id":   "evt_test_" + piID,
		"type": eventType,
		"data": map[string]any{
			"object": map[string]any{
				"id":       piID,
				"amount":   amount,
				"currency": currency,
				"status":   status,
				"metadata": metadata,
			},
		},
	}
	return json.Marshal(evt)
}

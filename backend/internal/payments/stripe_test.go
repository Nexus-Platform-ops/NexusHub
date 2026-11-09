package payments

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStripeProviderCreatesCharge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/payment_intents" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk_test_fake" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Idempotency-Key") != "intent-abc" {
			t.Errorf("idempotency = %q", r.Header.Get("Idempotency-Key"))
		}
		if r.Header.Get("Stripe-Version") == "" {
			t.Error("missing Stripe-Version")
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("amount") != "900000" {
			t.Errorf("amount = %q want 900000", r.FormValue("amount"))
		}
		if r.FormValue("currency") != "mmk" {
			t.Errorf("currency = %q", r.FormValue("currency"))
		}
		if r.FormValue("metadata[intent_id]") != "intent-abc" {
			t.Errorf("metadata[intent_id] = %q", r.FormValue("metadata[intent_id]"))
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":            "pi_test_123",
			"client_secret": "pi_test_123_secret_456",
			"status":        "requires_payment_method",
		})
	}))
	defer srv.Close()

	p := NewStripe(StripeConfig{
		SecretKey:  "sk_test_fake",
		APIBase:    srv.URL,
		HTTPClient: srv.Client(),
	})
	charge, err := p.CreateCharge(context.Background(), ChargeRequest{
		IntentID:  "intent-abc",
		AmountMMK: 9000,
		Currency:  Currency,
		ReturnURL: "https://app.example.com/wallet",
	})
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}
	if charge.ProviderRef != "pi_test_123" {
		t.Errorf("ref = %q", charge.ProviderRef)
	}
	if charge.PaymentURL != "pi_test_123_secret_456" {
		t.Errorf("url = %q", charge.PaymentURL)
	}
}

func TestStripeProviderMapsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "Card declined",
				"type":    "card_error",
				"code":    "card_declined",
			},
		})
	}))
	defer srv.Close()

	p := NewStripe(StripeConfig{SecretKey: "sk_test", APIBase: srv.URL, HTTPClient: srv.Client()})
	_, err := p.CreateCharge(context.Background(), ChargeRequest{IntentID: "x", AmountMMK: 100})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("empty error")
	}
}

func TestStripeProviderRejectsBadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer srv.Close()

	p := NewStripe(StripeConfig{SecretKey: "sk_test", APIBase: srv.URL, HTTPClient: srv.Client()})
	_, err := p.CreateCharge(context.Background(), ChargeRequest{IntentID: "x"})
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestStripeWebhookVerification(t *testing.T) {
	secret := "whsec_test_secret"
	body := []byte(`{"id":"evt_1","type":"payment_intent.succeeded","data":{"object":{"id":"pi_1"}}}`)
	ts := time.Now()
	header := SignStripeTestWebhook(secret, body, ts)

	evt, err := VerifyStripeWebhook(secret, header, body)
	if err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if evt.Type != "payment_intent.succeeded" {
		t.Errorf("type = %q", evt.Type)
	}

	// Tampered body.
	_, err = VerifyStripeWebhook(secret, header, []byte(`{"tampered":true}`))
	if err == nil {
		t.Error("tampered body accepted")
	}

	// Wrong secret.
	_, err = VerifyStripeWebhook("whsec_wrong", header, body)
	if err == nil {
		t.Error("wrong secret accepted")
	}

	// Stale timestamp.
	oldHeader := SignStripeTestWebhook(secret, body, ts.Add(-10*time.Minute))
	_, err = VerifyStripeWebhook(secret, oldHeader, body)
	if err == nil {
		t.Error("stale timestamp accepted")
	}

	// Empty header.
	_, err = VerifyStripeWebhook(secret, "", body)
	if err == nil {
		t.Error("empty header accepted")
	}
}

func TestStripeStatusMapping(t *testing.T) {
	cases := []struct {
		stripe string
		want   string
	}{
		{"succeeded", "paid"},
		{"canceled", "failed"},
		{"processing", "pending"},
		{"requires_action", "pending"},
		{"requires_payment_method", "failed"},
		{"requires_confirmation", "failed"},
		{"unknown_status", "pending"},
	}
	for _, c := range cases {
		if got := StripeStatusToIntentStatus(c.stripe); got != c.want {
			t.Errorf("StripeStatusToIntentStatus(%q) = %q, want %q", c.stripe, got, c.want)
		}
	}
}

func TestParseStripePaymentIntent(t *testing.T) {
	body, _ := BuildStripeWebhookPayload("payment_intent.succeeded", "pi_123", 900000, "mmk", "succeeded",
		map[string]string{"intent_id": "abc"})
	var evt StripeWebhookEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	pi, err := ParseStripePaymentIntent(&evt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if pi.ID != "pi_123" {
		t.Errorf("id = %q", pi.ID)
	}
	if pi.Amount != 900000 {
		t.Errorf("amount = %d", pi.Amount)
	}
	if pi.Metadata["intent_id"] != "abc" {
		t.Errorf("metadata = %v", pi.Metadata)
	}
}

func TestParseStripeDispute(t *testing.T) {
	evt := &StripeWebhookEvent{
		ID:   "evt_dispute",
		Type: "charge.dispute.created",
	}
	evt.Data.Object = json.RawMessage(`{"id":"dp_1","charge":"ch_1","payment_intent":"pi_1","amount":5000,"currency":"mmk","status":"warning_needs_response","reason":"general"}`)
	d, err := ParseStripeDispute(evt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if d.ID != "dp_1" || d.ChargeID != "ch_1" || d.Amount != 5000 {
		t.Errorf("dispute = %+v", d)
	}
}

func TestStripeRefundCreation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/refunds" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("payment_intent") != "pi_ref" {
			t.Errorf("payment_intent = %q", r.FormValue("payment_intent"))
		}
		if r.FormValue("reason") != "duplicate" {
			t.Errorf("reason = %q", r.FormValue("reason"))
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "re_123",
			"status": "succeeded",
			"amount": 500000,
		})
	}))
	defer srv.Close()

	refund, err := CreateStripeRefund(context.Background(), StripeConfig{
		SecretKey:  "sk_test",
		APIBase:    srv.URL,
		HTTPClient: srv.Client(),
	}, StripeRefundRequest{
		PaymentIntentID: "pi_ref",
		AmountMMK:       5000,
		Reason:          "duplicate",
	})
	if err != nil {
		t.Fatalf("refund: %v", err)
	}
	if refund.ID != "re_123" || refund.Status != "succeeded" || refund.Amount != 5000 {
		t.Errorf("refund = %+v", refund)
	}
}

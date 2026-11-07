package payments

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFindPackage(t *testing.T) {
	pkg, ok := FindPackage("coins_500")
	if !ok || pkg.Coins != 500 || pkg.PriceMMK != 9000 {
		t.Fatalf("package = %+v ok=%v", pkg, ok)
	}
	if _, ok := FindPackage("coins_999999"); ok {
		t.Fatal("unknown package accepted")
	}
}

func TestHTTPProviderCreatesCharge(t *testing.T) {
	var got map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &got); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"ch_123","payment_url":"https://pay.example/checkout/ch_123"}`))
	}))
	defer srv.Close()

	p := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "pay-key", HTTPClient: srv.Client()})
	charge, err := p.CreateCharge(context.Background(), ChargeRequest{
		IntentID: "0123456789abcdef0123456789abcdef", AmountMMK: 9000, Currency: Currency,
		ReturnURL: "https://app.example.com/wallet",
	})
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}
	if auth != "Bearer pay-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if charge.ProviderRef != "ch_123" || charge.PaymentURL != "https://pay.example/checkout/ch_123" {
		t.Fatalf("charge = %+v", charge)
	}
	if got["amount_mmk"] != float64(9000) || got["currency"] != Currency || got["intent_id"] != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("payload = %v", got)
	}
}

func TestHTTPProviderMapsFailure(t *testing.T) {
	cases := []http.HandlerFunc{
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadGateway) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"id":"ch_1"}`)) },
		func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`not-json`)) },
	}
	for i, h := range cases {
		srv := httptest.NewServer(h)
		p := New(Config{Provider: HTTP, APIURL: srv.URL, APIKey: "k", HTTPClient: srv.Client()})
		if _, err := p.CreateCharge(context.Background(), ChargeRequest{IntentID: "x"}); !errors.Is(err, ErrUpstream) {
			t.Fatalf("case %d: error = %v, want ErrUpstream", i, err)
		}
		srv.Close()
	}
}

func TestLogProviderDoesNotCallOut(t *testing.T) {
	charge, err := New(Config{Provider: Log}).CreateCharge(context.Background(), ChargeRequest{IntentID: "x", AmountMMK: 1})
	if err != nil || charge != (Charge{}) {
		t.Fatalf("log charge = %+v err=%v", charge, err)
	}
}

func TestWebhookSigning(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ts := "1800000000"
	body := []byte(`{"intent_id":"abc","status":"paid"}`)
	sig := SignWebhook("secret-key", ts, body)

	if err := VerifyWebhook("secret-key", ts, sig, body, now); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifyWebhook("secret-key", ts, sig, []byte(`{"status":"paid"}`), now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered body = %v", err)
	}
	if err := VerifyWebhook("other-key", ts, sig, body, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("wrong key = %v", err)
	}
	if err := VerifyWebhook("secret-key", ts, sig, body, now.Add(DefaultWebhookTolerance+time.Second)); !errors.Is(err, ErrStaleTimestamp) {
		t.Fatalf("stale timestamp = %v", err)
	}
	if err := VerifyWebhook("secret-key", "", sig, body, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("missing timestamp = %v", err)
	}
	if err := VerifyWebhook("secret-key", ts, "", body, now); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("missing signature = %v", err)
	}
}

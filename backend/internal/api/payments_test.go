package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"nexus/backend/internal/payments"
)

func postWebhook(t *testing.T, evt map[string]any, secret string, ts time.Time, badSig bool) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(evt)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	tsStr := strconv.FormatInt(ts.Unix(), 10)
	sig := payments.SignWebhook(secret, tsStr, body)
	if badSig {
		sig = "0000"
	}
	req := httptest.NewRequest("POST", "/api/v1/payments/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Timestamp", tsStr)
	req.Header.Set("X-Payment-Signature", sig)
	w := httptest.NewRecorder()
	testServer.Routes().ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func createIntent(t *testing.T, token, packageID string) (int, map[string]any) {
	t.Helper()
	st, resp, _ := doReq(t, "POST", "/api/v1/coins/intents", map[string]any{"package_id": packageID}, token)
	return st, resp
}

func balanceOf(t *testing.T, token string) int64 {
	t.Helper()
	st, resp, _ := doReq(t, "GET", "/api/v1/coins/balance", nil, token)
	needStatus(t, st, http.StatusOK, resp)
	return int64(resp["balance"].(float64))
}

func TestPaymentIntentLifecycle(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Payment User")

	st, resp := createIntent(t, token, "coins_500")
	needStatus(t, st, http.StatusCreated, resp)
	intentID, _ := resp["intent_id"].(string)
	if len(intentID) != 32 {
		t.Fatalf("intent id = %q", intentID)
	}
	if resp["coins"].(float64) != 500 || resp["amount_mmk"].(float64) != 9000 || resp["status"] != "pending" {
		t.Fatalf("intent = %v", resp)
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+intentID, nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["status"] != "pending" {
		t.Fatalf("intent status = %v", resp["status"])
	}

	evt := map[string]any{"intent_id": intentID, "status": "paid", "amount_mmk": 9000, "provider_ref": "ch_1"}
	st, resp = postWebhook(t, evt, testServer.Cfg.PaymentSigningKey, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)

	if got := balanceOf(t, token); got != 500 {
		t.Fatalf("balance = %d want 500", got)
	}

	// Replay must not double-credit.
	st, resp = postWebhook(t, evt, testServer.Cfg.PaymentSigningKey, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	if resp["duplicate"] != true {
		t.Fatalf("replay = %v", resp)
	}
	if got := balanceOf(t, token); got != 500 {
		t.Fatalf("balance after replay = %d want 500", got)
	}

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+intentID, nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["status"] != "paid" || resp["paid_at"] == nil {
		t.Fatalf("paid intent = %v", resp)
	}

	st, resp = createIntent(t, token, "coins_999")
	needStatus(t, st, http.StatusBadRequest, resp)
}

func TestPaymentIntentIsPrivate(t *testing.T) {
	resetDB(t)
	_, tokenA := makeUser(t, "Payer")
	_, tokenB := makeUser(t, "Other Payer")
	st, resp := createIntent(t, tokenA, "coins_100")
	needStatus(t, st, http.StatusCreated, resp)
	intentID := resp["intent_id"].(string)

	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+intentID, nil, tokenB)
	needStatus(t, st, http.StatusNotFound, resp)
}

func TestPaymentWebhookRejectsBadRequests(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Webhook User")
	st, resp := createIntent(t, token, "coins_100")
	needStatus(t, st, http.StatusCreated, resp)
	intentID := resp["intent_id"].(string)
	secret := testServer.Cfg.PaymentSigningKey
	evt := map[string]any{"intent_id": intentID, "status": "paid", "amount_mmk": 2000}

	// Bad signature and stale signatures are rejected before any state change.
	st, resp = postWebhook(t, evt, secret, time.Now(), true)
	needStatus(t, st, http.StatusUnauthorized, resp)
	st, resp = postWebhook(t, evt, secret, time.Now().Add(-10*time.Minute), false)
	needStatus(t, st, http.StatusUnauthorized, resp)

	// Wrong amount is rejected and leaves the intent payable.
	wrong := map[string]any{"intent_id": intentID, "status": "paid", "amount_mmk": 1}
	st, resp = postWebhook(t, wrong, secret, time.Now(), false)
	needStatus(t, st, http.StatusBadRequest, resp)
	if got := balanceOf(t, token); got != 0 {
		t.Fatalf("balance after bad amount = %d", got)
	}

	st, resp = postWebhook(t, evt, secret, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	if got := balanceOf(t, token); got != 100 {
		t.Fatalf("balance = %d want 100", got)
	}

	// Unknown intent.
	unknown := map[string]any{"intent_id": "0123456789abcdef0123456789abcdef", "status": "paid", "amount_mmk": 2000}
	st, resp = postWebhook(t, unknown, secret, time.Now(), false)
	needStatus(t, st, http.StatusNotFound, resp)
}

func TestPaymentWebhookFailedAndExpired(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Expiry User")

	st, resp := createIntent(t, token, "coins_100")
	needStatus(t, st, http.StatusCreated, resp)
	failedID := resp["intent_id"].(string)
	secret := testServer.Cfg.PaymentSigningKey

	st, resp = postWebhook(t, map[string]any{"intent_id": failedID, "status": "failed", "amount_mmk": 2000}, secret, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+failedID, nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["status"] != "failed" {
		t.Fatalf("status = %v", resp["status"])
	}
	// A late confirmation for an intent we marked failed still credits: the
	// provider proved the user paid, and the user must not lose money.
	st, resp = postWebhook(t, map[string]any{"intent_id": failedID, "status": "paid", "amount_mmk": 2000}, secret, time.Now(), false)
	needStatus(t, st, http.StatusOK, resp)
	if got := balanceOf(t, token); got != 100 {
		t.Fatalf("balance after late payment = %d want 100", got)
	}
	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+failedID, nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["status"] != "paid" {
		t.Fatalf("late paid status = %v", resp["status"])
	}

	st, resp = createIntent(t, token, "coins_100")
	needStatus(t, st, http.StatusCreated, resp)
	expiredID := resp["intent_id"].(string)
	if _, err := testStore.Social.Exec(context.Background(),
		`UPDATE ledger.payment_intents SET expires_at = now() - interval '1 minute' WHERE id=$1`, expiredID); err != nil {
		t.Fatalf("expire intent: %v", err)
	}
	st, resp = postWebhook(t, map[string]any{"intent_id": expiredID, "status": "paid", "amount_mmk": 2000}, secret, time.Now(), false)
	needStatus(t, st, http.StatusConflict, resp)
	st, resp, _ = doReq(t, "GET", "/api/v1/coins/intents/"+expiredID, nil, token)
	needStatus(t, st, http.StatusOK, resp)
	if resp["status"] != "expired" {
		t.Fatalf("status = %v", resp["status"])
	}
}

func TestPaymentIntentProviderFailure(t *testing.T) {
	resetDB(t)
	_, token := makeUser(t, "Provider Failure")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	orig := testServer.Payments
	testServer.Payments = payments.New(payments.Config{
		Provider: payments.HTTP, APIURL: srv.URL, APIKey: "k", HTTPClient: srv.Client(),
	})
	defer func() { testServer.Payments = orig }()

	st, resp := createIntent(t, token, "coins_100")
	needStatus(t, st, http.StatusBadGateway, resp)
	if got := balanceOf(t, token); got != 0 {
		t.Fatalf("balance = %d", got)
	}
	var status string
	if err := testStore.Social.QueryRow(context.Background(),
		`SELECT status FROM ledger.payment_intents ORDER BY created_at DESC LIMIT 1`).Scan(&status); err != nil {
		t.Fatalf("read intent: %v", err)
	}
	if status != "failed" {
		t.Fatalf("status = %q want failed", status)
	}
}

type fakeSMSSender struct {
	code  string
	phone string
	err   error
	calls int
}

func (f *fakeSMSSender) SendOTP(_ context.Context, phone, code string) error {
	f.calls++
	f.phone = phone
	f.code = code
	return f.err
}

func TestOTPDeliveryUsesSender(t *testing.T) {
	resetDB(t)
	orig := testServer.SMS
	fake := &fakeSMSSender{}
	testServer.SMS = fake
	defer func() { testServer.SMS = orig }()

	phone := fmt.Sprintf("+959%08d", 42424242)
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": phone, "display_name": "SMS User"}, "")
	needStatus(t, st, http.StatusCreated, resp)
	if fake.calls != 1 || fake.phone != phone || len(fake.code) != 6 {
		t.Fatalf("sender = %+v", fake)
	}
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": fake.code}, "")
	needStatus(t, st, http.StatusOK, resp)
}

func TestOTPDeliveryFailureClearsCode(t *testing.T) {
	resetDB(t)
	orig := testServer.SMS
	fake := &fakeSMSSender{err: fmt.Errorf("provider down")}
	testServer.SMS = fake
	defer func() { testServer.SMS = orig }()

	phone := fmt.Sprintf("+959%08d", 43434343)
	st, resp, _ := doReq(t, "POST", "/api/v1/auth/register",
		map[string]any{"phone": phone, "display_name": "SMS Failure"}, "")
	needStatus(t, st, http.StatusBadGateway, resp)
	if fake.calls != 1 {
		t.Fatalf("sender calls = %d", fake.calls)
	}
	// The undeliverable code must not remain valid.
	st, resp, _ = doReq(t, "POST", "/api/v1/auth/verify",
		map[string]any{"phone": phone, "code": fake.code}, "")
	needStatus(t, st, http.StatusBadRequest, resp)
}

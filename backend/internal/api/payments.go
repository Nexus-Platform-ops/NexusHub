package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nexus/backend/internal/payments"
)

// paymentIntentTTL bounds how long a checkout URL may be paid.
const paymentIntentTTL = 30 * time.Minute

type intentRequest struct {
	PackageID string `json:"package_id"`
}

type intentJSON struct {
	IntentID   string     `json:"intent_id"`
	PackageID  string     `json:"package_id"`
	Coins      int64      `json:"coins"`
	AmountMMK  int64      `json:"amount_mmk"`
	Currency   string     `json:"currency"`
	Status     string     `json:"status"`
	PaymentURL string     `json:"payment_url"`
	ExpiresAt  time.Time  `json:"expires_at"`
	PaidAt     *time.Time `json:"paid_at,omitempty"`
}

func newIntentID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// handleCreatePaymentIntent starts a top-up: the client chooses a package id,
// the server fixes the coin amount and MMK price, and the provider returns a
// hosted checkout URL.
func (s *Server) handleCreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req intentRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	pkg, ok := payments.FindPackage(strings.TrimSpace(req.PackageID))
	if !ok {
		errorJSON(w, http.StatusBadRequest, "unknown package_id")
		return
	}
	id, err := newIntentID()
	if err != nil {
		serverError(w, "generate intent id", err)
		return
	}
	expires := time.Now().Add(paymentIntentTTL).UTC()
	if _, err := s.Store.Social.Exec(r.Context(),
		`INSERT INTO ledger.payment_intents
		   (id, user_id, package_id, coins, amount_mmk, currency, provider, status, expires_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,'pending',$8)`,
		id, uid, pkg.ID, pkg.Coins, pkg.PriceMMK, payments.Currency, s.Cfg.PaymentProvider, expires); err != nil {
		serverError(w, "create payment intent", err)
		return
	}
	charge, err := s.Payments.CreateCharge(r.Context(), payments.ChargeRequest{
		IntentID:  id,
		AmountMMK: pkg.PriceMMK,
		Currency:  payments.Currency,
		ReturnURL: s.Cfg.PaymentReturnURL,
	})
	if err != nil {
		if _, uerr := s.Store.Social.Exec(r.Context(),
			`UPDATE ledger.payment_intents SET status='failed' WHERE id=$1`, id); uerr != nil {
			log.Printf("close failed payment intent %s: %v", id, uerr)
		}
		providerError(w, "create payment charge", "payment provider unavailable", err)
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`UPDATE ledger.payment_intents SET provider_ref=$2, payment_url=$3 WHERE id=$1`,
		id, charge.ProviderRef, charge.PaymentURL); err != nil {
		serverError(w, "record payment charge", err)
		return
	}
	writeJSON(w, http.StatusCreated, intentJSON{
		IntentID:   id,
		PackageID:  pkg.ID,
		Coins:      pkg.Coins,
		AmountMMK:  pkg.PriceMMK,
		Currency:   payments.Currency,
		Status:     "pending",
		PaymentURL: charge.PaymentURL,
		ExpiresAt:  expires,
	})
}

// handleGetPaymentIntent returns the caller's intent so the client can poll
// after returning from checkout.
func (s *Server) handleGetPaymentIntent(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	id := strings.TrimSpace(r.PathValue("id"))
	if len(id) < 16 || len(id) > 64 {
		errorJSON(w, http.StatusNotFound, "payment intent not found")
		return
	}
	var out intentJSON
	err := s.Store.Social.QueryRow(r.Context(),
		`SELECT package_id, coins, amount_mmk, currency, status, payment_url, expires_at, paid_at
		 FROM ledger.payment_intents WHERE id=$1 AND user_id=$2`,
		id, uid).Scan(&out.PackageID, &out.Coins, &out.AmountMMK, &out.Currency,
		&out.Status, &out.PaymentURL, &out.ExpiresAt, &out.PaidAt)
	if errors.Is(err, pgx.ErrNoRows) {
		errorJSON(w, http.StatusNotFound, "payment intent not found")
		return
	}
	if err != nil {
		serverError(w, "load payment intent", err)
		return
	}
	out.IntentID = id
	if out.Status == "pending" && time.Now().After(out.ExpiresAt) {
		out.Status = "expired"
	}
	writeJSON(w, http.StatusOK, out)
}

type webhookEvent struct {
	IntentID    string `json:"intent_id"`
	ProviderRef string `json:"provider_ref"`
	Status      string `json:"status"`
	AmountMMK   int64  `json:"amount_mmk"`
}

// handlePaymentWebhook credits coins when the provider confirms a payment.
// The HMAC covers the raw body plus the signed timestamp; the intent carries
// the user and price, so a forged body can never choose either.
func (s *Server) handlePaymentWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := payments.VerifyWebhook(s.Cfg.PaymentSigningKey,
		r.Header.Get("X-Payment-Timestamp"), r.Header.Get("X-Payment-Signature"),
		body, time.Now()); err != nil {
		errorJSON(w, http.StatusUnauthorized, "invalid webhook signature")
		return
	}
	var evt webhookEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}
	evt.IntentID = strings.TrimSpace(evt.IntentID)
	if len(evt.IntentID) < 16 || len(evt.IntentID) > 64 {
		errorJSON(w, http.StatusBadRequest, "invalid intent_id")
		return
	}
	switch evt.Status {
	case "paid", "failed":
	default:
		errorJSON(w, http.StatusBadRequest, "unsupported status")
		return
	}
	evt.ProviderRef = strings.TrimSpace(evt.ProviderRef)

	ctx := r.Context()
	tx, err := s.Store.Social.Begin(ctx)
	if err != nil {
		serverError(w, "begin webhook tx", err)
		return
	}
	defer tx.Rollback(ctx)

	var (
		uid, coins, amount  int64
		status, providerRef string
		expires             time.Time
	)
	err = tx.QueryRow(ctx,
		`SELECT user_id, coins, amount_mmk, status, provider_ref, expires_at
		 FROM ledger.payment_intents WHERE id=$1 FOR UPDATE`,
		evt.IntentID).Scan(&uid, &coins, &amount, &status, &providerRef, &expires)
	if errors.Is(err, pgx.ErrNoRows) {
		errorJSON(w, http.StatusNotFound, "unknown payment intent")
		return
	}
	if err != nil {
		serverError(w, "load payment intent", err)
		return
	}

	// Replays and out-of-order confirmations never double-credit.
	if status == "paid" {
		writeJSON(w, http.StatusOK, map[string]any{"status": "paid", "duplicate": true})
		return
	}

	if evt.Status == "failed" {
		if status == "pending" {
			if _, err := tx.Exec(ctx,
				`UPDATE ledger.payment_intents SET status='failed' WHERE id=$1`,
				evt.IntentID); err != nil {
				serverError(w, "fail payment intent", err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			serverError(w, "commit webhook", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "failed"})
		return
	}

	// A charge-creation error marks the intent failed even though the provider
	// may have created it. A later signed "paid" event proves the user paid,
	// so failed intents may still be credited; expired ones may not.
	if status != "pending" && status != "failed" {
		errorJSON(w, http.StatusConflict, "intent is "+status)
		return
	}
	if evt.ProviderRef != "" && providerRef != "" && evt.ProviderRef != providerRef {
		errorJSON(w, http.StatusBadRequest, "provider_ref mismatch")
		return
	}
	if evt.AmountMMK != amount {
		log.Printf("payments: amount mismatch for intent %s: webhook=%d intent=%d ref=%q",
			evt.IntentID, evt.AmountMMK, amount, evt.ProviderRef)
		errorJSON(w, http.StatusBadRequest, "amount mismatch")
		return
	}
	if time.Now().After(expires) {
		if _, err := tx.Exec(ctx,
			`UPDATE ledger.payment_intents SET status='expired' WHERE id=$1`,
			evt.IntentID); err != nil {
			serverError(w, "expire payment intent", err)
			return
		}
		if err := tx.Commit(ctx); err != nil {
			serverError(w, "commit webhook", err)
			return
		}
		errorJSON(w, http.StatusConflict, "intent expired")
		return
	}
	if _, err := tx.Exec(ctx,
		`UPDATE ledger.payment_intents
		 SET status='paid', paid_at=now(),
		     provider_ref=CASE WHEN $2<>'' THEN $2 ELSE provider_ref END
		 WHERE id=$1`,
		evt.IntentID, evt.ProviderRef); err != nil {
		serverError(w, "mark intent paid", err)
		return
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO ledger.coin_transactions (user_id, amount, kind, request_id)
		 VALUES ($1,$2,'topup',$3)
		 ON CONFLICT (request_id) WHERE request_id IS NOT NULL DO NOTHING`,
		uid, coins, "intent:"+evt.IntentID); err != nil {
		serverError(w, "credit coins", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		serverError(w, "commit webhook", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "paid", "coins": coins})
}

package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// handleCoinBalance returns the caller's ledger-derived balance.
func (s *Server) handleCoinBalance(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var balance int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(amount),0) FROM ledger.coin_transactions WHERE user_id=$1`,
		uid).Scan(&balance); err != nil {
		serverError(w, "read balance", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"balance": balance})
}

type topupRequest struct {
	Amount    int64  `json:"amount"`
	RequestID string `json:"request_id"`
}

// handleCoinTopup credits coins only when the request carries a valid
// HMAC-SHA256 payment confirmation; request_id makes retries idempotent.
// Coins can never be minted by client-side arithmetic.
func (s *Server) handleCoinTopup(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req topupRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Amount <= 0 || req.Amount > 1000000 {
		errorJSON(w, http.StatusBadRequest, "amount must be between 1 and 1000000")
		return
	}
	req.RequestID = trimSpace(req.RequestID)
	if len(req.RequestID) < 8 || len(req.RequestID) > 64 {
		errorJSON(w, http.StatusBadRequest, "request_id must be 8-64 characters")
		return
	}
	mac := hmac.New(sha256.New, []byte(s.Cfg.PaymentSigningKey))
	fmt.Fprintf(mac, "%d:%d:%s", uid, req.Amount, req.RequestID)
	expected := hex.EncodeToString(mac.Sum(nil))
	sig := strings.ToLower(r.Header.Get("X-Payment-Signature"))
	if subtleConstantTimeCompare(expected, sig) != 1 {
		errorJSON(w, http.StatusBadRequest, "invalid payment signature")
		return
	}
	var txnID int64
	err := s.Store.Social.QueryRow(r.Context(),
		`INSERT INTO ledger.coin_transactions (user_id, amount, kind, request_id)
		 VALUES ($1,$2,'topup',$3)
		 ON CONFLICT (request_id) WHERE request_id IS NOT NULL DO NOTHING
		 RETURNING id`,
		uid, req.Amount, req.RequestID).Scan(&txnID)
	if err == pgx.ErrNoRows {
		// Duplicate request: idempotent replay, no double credit.
		var balance int64
		if err := s.Store.Social.QueryRow(r.Context(),
			`SELECT COALESCE(SUM(amount),0) FROM ledger.coin_transactions WHERE user_id=$1`,
			uid).Scan(&balance); err != nil {
			serverError(w, "read balance", err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"idempotent_replay": true, "balance": balance})
		return
	}
	if err != nil {
		serverError(w, "record topup", err)
		return
	}
	var balance int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(amount),0) FROM ledger.coin_transactions WHERE user_id=$1`,
		uid).Scan(&balance); err != nil {
		serverError(w, "read balance", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"credited": req.Amount, "balance": balance})
}

// handleCoinLedger lists the caller's most recent transactions.
func (s *Server) handleCoinLedger(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	rows, err := s.Store.Social.Query(r.Context(),
		`SELECT id, amount, kind, created_at FROM ledger.coin_transactions
		 WHERE user_id=$1 ORDER BY id DESC LIMIT 50`, uid)
	if err != nil {
		serverError(w, "list ledger", err)
		return
	}
	defer rows.Close()
	type txn struct {
		ID        int64     `json:"id"`
		Amount    int64     `json:"amount"`
		Kind      string    `json:"kind"`
		CreatedAt time.Time `json:"created_at"`
	}
	ledger := []txn{}
	for rows.Next() {
		var t txn
		if err := rows.Scan(&t.ID, &t.Amount, &t.Kind, &t.CreatedAt); err != nil {
			serverError(w, "scan ledger", err)
			return
		}
		ledger = append(ledger, t)
	}
	if err := rows.Err(); err != nil {
		serverError(w, "iterate ledger", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ledger": ledger})
}

// handleCoinTopupDev credits coins without a payment signature. It exists
// only for development builds (OTP_DEBUG=1) so the UI demo can top up;
// production builds return 404.
func (s *Server) handleCoinTopupDev(w http.ResponseWriter, r *http.Request) {
	if !s.Cfg.OTPDebug {
		errorJSON(w, http.StatusNotFound, "not found")
		return
	}
	uid, _ := userID(r)
	var req topupRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	if req.Amount <= 0 || req.Amount > 1000000 {
		errorJSON(w, http.StatusBadRequest, "amount must be between 1 and 1000000")
		return
	}
	reqID := "dev-" + trimSpace(req.RequestID)
	if len(reqID) < 8 || len(reqID) > 64 {
		errorJSON(w, http.StatusBadRequest, "request_id must be 8-64 characters")
		return
	}
	if _, err := s.Store.Social.Exec(r.Context(),
		`INSERT INTO ledger.coin_transactions (user_id, amount, kind, request_id)
		 VALUES ($1,$2,'topup',$3) ON CONFLICT (request_id) WHERE request_id IS NOT NULL DO NOTHING`,
		uid, req.Amount, reqID); err != nil {
		serverError(w, "dev topup", err)
		return
	}
	var balance int64
	if err := s.Store.Social.QueryRow(r.Context(),
		`SELECT COALESCE(SUM(amount),0) FROM ledger.coin_transactions WHERE user_id=$1`,
		uid).Scan(&balance); err != nil {
		serverError(w, "read balance", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"credited": req.Amount, "balance": balance})
}

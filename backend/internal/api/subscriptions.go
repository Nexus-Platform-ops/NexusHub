package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nexus/backend/internal/payments"
)

type subscriptionJSON struct {
	ID                 int64     `json:"id"`
	UserID             int64     `json:"user_id"`
	PlanID             string    `json:"plan_id"`
	Provider           string    `json:"provider"`
	ProviderRef        string    `json:"provider_ref,omitempty"`
	Status             string    `json:"status"`
	CurrentPeriodStart time.Time `json:"current_period_start"`
	CurrentPeriodEnd   time.Time `json:"current_period_end"`
	CancelAtPeriodEnd  bool      `json:"cancel_at_period_end"`
	CreatedAt          time.Time `json:"created_at"`
}

type planJSON struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Duration string   `json:"duration"`
	Features []string `json:"features"`
	Active   bool     `json:"active"`
}

type receiptRequest struct {
	Platform string `json:"platform"`
	Token    string `json:"receipt_token"`
	PlanID   string `json:"plan_id"`
}

type receiptJSON struct {
	ID            int64      `json:"id"`
	Platform      string     `json:"platform"`
	Status        string     `json:"status"`
	TransactionID string     `json:"transaction_id,omitempty"`
	ValidatedAt   *time.Time `json:"validated_at,omitempty"`
}

// handleListPlans returns available subscription plans.
func (s *Server) handleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.SubStore.ListPlans(r.Context())
	if err != nil {
		serverError(w, "list plans", err)
		return
	}
	out := make([]planJSON, 0, len(plans))
	for _, p := range plans {
		if !p.Active {
			continue
		}
		out = append(out, planJSON{
			ID:       p.ID,
			Name:     p.Name,
			Duration: p.Duration.String(),
			Features: p.Features,
			Active:   p.Active,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": out})
}

// handleGetSubscription returns the caller's active subscription.
func (s *Server) handleGetSubscription(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	sub, err := s.SubStore.GetActiveSubscription(r.Context(), uid)
	if err != nil {
		if errors.Is(err, payments.ErrSubscriptionNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"subscription": nil})
			return
		}
		serverError(w, "get subscription", err)
		return
	}
	out := subscriptionToJSON(sub)
	if sub.IsExpired() && sub.Status == payments.SubActive {
		out.Status = payments.SubExpired
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": out})
}

// handleCreateSubscription starts a new subscription from a platform receipt.
func (s *Server) handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	var req receiptRequest
	if err := readJSON(w, r, &req); err != nil {
		return
	}
	req.Platform = strings.ToLower(strings.TrimSpace(req.Platform))
	req.Token = strings.TrimSpace(req.Token)
	req.PlanID = strings.TrimSpace(req.PlanID)

	if req.Platform != "apple" && req.Platform != "google" {
		errorJSON(w, http.StatusBadRequest, "platform must be apple or google")
		return
	}
	if req.Token == "" {
		errorJSON(w, http.StatusBadRequest, "receipt_token is required")
		return
	}
	if req.PlanID == "" {
		errorJSON(w, http.StatusBadRequest, "plan_id is required")
		return
	}

	plan, err := s.SubStore.GetPlan(r.Context(), req.PlanID)
	if err != nil {
		if errors.Is(err, payments.ErrPlanNotFound) {
			errorJSON(w, http.StatusBadRequest, "unknown plan_id")
			return
		}
		serverError(w, "get plan", err)
		return
	}
	if !plan.Active {
		errorJSON(w, http.StatusBadRequest, "plan is not active")
		return
	}

	// Check for duplicate receipt.
	existing, _ := s.SubStore.GetReceipt(r.Context(), req.Platform, req.Token)
	if existing != nil {
		errorJSON(w, http.StatusConflict, "receipt already submitted")
		return
	}

	// Validate receipt with platform store.
	var result *payments.ValidationResult
	switch req.Platform {
	case "apple":
		result, err = s.ReceiptValidator.ValidateApple(r.Context(), req.Token)
	case "google":
		result, err = s.ReceiptValidator.ValidateGoogle(r.Context(), req.Token)
	}
	if err != nil {
		serverError(w, "validate receipt", err)
		return
	}
	if !result.Valid {
		errorJSON(w, http.StatusBadRequest, "receipt validation failed: "+result.ErrorMessage)
		return
	}

	now := time.Now().UTC()
	periodEnd := now.Add(plan.Duration)
	if result.ExpiresAt != nil {
		periodEnd = *result.ExpiresAt
	}

	provider := req.Platform
	sub := &payments.Subscription{
		UserID:             uid,
		PlanID:             plan.ID,
		Provider:           provider,
		ProviderRef:        result.TransactionID,
		Status:             payments.SubActive,
		CurrentPeriodStart: now,
		CurrentPeriodEnd:   periodEnd,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	if err := s.SubStore.CreateSubscription(r.Context(), sub); err != nil {
		serverError(w, "create subscription", err)
		return
	}

	validatedAt := now
	receipt := &payments.PurchaseReceipt{
		UserID:         uid,
		SubscriptionID: sub.ID,
		Platform:       req.Platform,
		ReceiptToken:   req.Token,
		TransactionID:  result.TransactionID,
		Status:         "validated",
		ValidatedAt:    &validatedAt,
		CreatedAt:      now,
	}
	if err := s.SubStore.SaveReceipt(r.Context(), receipt); err != nil {
		log.Printf("subscriptions: save receipt for sub %d: %v", sub.ID, err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"subscription": subscriptionToJSON(sub),
		"receipt": receiptToJSON(receipt),
	})
}

// handleCancelSubscription cancels the caller's active subscription.
func (s *Server) handleCancelSubscription(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	if err := s.SubStore.CancelSubscription(r.Context(), uid); err != nil {
		if errors.Is(err, payments.ErrSubscriptionNotFound) {
			errorJSON(w, http.StatusNotFound, "no active subscription")
			return
		}
		serverError(w, "cancel subscription", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "canceled"})
}

// handleCheckFeature gates a premium feature on the caller's subscription.
func (s *Server) handleCheckFeature(w http.ResponseWriter, r *http.Request) {
	uid, _ := userID(r)
	feature := strings.TrimSpace(r.PathValue("feature"))
	if feature == "" {
		errorJSON(w, http.StatusBadRequest, "feature is required")
		return
	}

	sub, err := s.SubStore.GetActiveSubscription(r.Context(), uid)
	if err != nil {
		if errors.Is(err, payments.ErrSubscriptionNotFound) {
			writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "reason": "no_subscription"})
			return
		}
		serverError(w, "check feature", err)
		return
	}

	plan, err := s.SubStore.GetPlan(r.Context(), sub.PlanID)
	if err != nil {
		serverError(w, "get plan for feature check", err)
		return
	}

	enabled := sub.HasFeature(plan, feature)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":  enabled,
		"feature":  feature,
		"plan_id":  sub.PlanID,
		"status":   sub.Status,
	})
}

// handleStripeWebhook processes Stripe webhook events for subscription lifecycle.
func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "invalid request body")
		return
	}

	evt, err := payments.VerifyStripeWebhook(s.Cfg.StripeWebhookSecret, r.Header.Get("Stripe-Signature"), body)
	if err != nil {
		errorJSON(w, http.StatusUnauthorized, "invalid stripe signature")
		return
	}

	switch evt.Type {
	case "payment_intent.succeeded":
		s.handleStripePaymentSucceeded(w, evt)
	case "customer.subscription.updated":
		s.handleStripeSubscriptionUpdated(w, evt)
	case "customer.subscription.deleted":
		s.handleStripeSubscriptionDeleted(w, evt)
	case "charge.dispute.created":
		s.handleStripeDispute(w, evt)
	case "charge.refunded":
		s.handleStripeRefund(w, evt)
	default:
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored", "type": evt.Type})
	}
}

func (s *Server) handleStripePaymentSucceeded(w http.ResponseWriter, evt *payments.StripeWebhookEvent) {
	var pi struct {
		ID       string            `json:"id"`
		Amount   int64             `json:"amount"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.Unmarshal(evt.Data.Object, &pi); err != nil {
		errorJSON(w, http.StatusBadRequest, "decode payment_intent")
		return
	}
	log.Printf("stripe: payment_intent.succeeded id=%s amount=%d metadata=%v", pi.ID, pi.Amount, pi.Metadata)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (s *Server) handleStripeSubscriptionUpdated(w http.ResponseWriter, evt *payments.StripeWebhookEvent) {
	var sub struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(evt.Data.Object, &sub); err != nil {
		errorJSON(w, http.StatusBadRequest, "decode subscription")
		return
	}
	log.Printf("stripe: subscription.updated id=%s status=%s", sub.ID, sub.Status)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (s *Server) handleStripeSubscriptionDeleted(w http.ResponseWriter, evt *payments.StripeWebhookEvent) {
	var sub struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(evt.Data.Object, &sub); err != nil {
		errorJSON(w, http.StatusBadRequest, "decode subscription")
		return
	}
	log.Printf("stripe: subscription.deleted id=%s", sub.ID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (s *Server) handleStripeDispute(w http.ResponseWriter, evt *payments.StripeWebhookEvent) {
	d, err := payments.ParseStripeDispute(evt)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, "decode dispute")
		return
	}
	log.Printf("stripe: dispute.created id=%s charge=%s amount=%d reason=%s",
		d.ID, d.ChargeID, d.Amount, d.Reason)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (s *Server) handleStripeRefund(w http.ResponseWriter, evt *payments.StripeWebhookEvent) {
	var refund struct {
		ID     string `json:"id"`
		Amount int64  `json:"amount"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(evt.Data.Object, &refund); err != nil {
		errorJSON(w, http.StatusBadRequest, "decode refund")
		return
	}
	log.Printf("stripe: charge.refunded id=%s amount=%d status=%s", refund.ID, refund.Amount, refund.Status)
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func subscriptionToJSON(sub *payments.Subscription) subscriptionJSON {
	return subscriptionJSON{
		ID:                 sub.ID,
		UserID:             sub.UserID,
		PlanID:             sub.PlanID,
		Provider:           sub.Provider,
		ProviderRef:        sub.ProviderRef,
		Status:             sub.Status,
		CurrentPeriodStart: sub.CurrentPeriodStart,
		CurrentPeriodEnd:   sub.CurrentPeriodEnd,
		CancelAtPeriodEnd:  sub.CancelAtPeriodEnd,
		CreatedAt:          sub.CreatedAt,
	}
}

func receiptToJSON(r *payments.PurchaseReceipt) receiptJSON {
	return receiptJSON{
		ID:            r.ID,
		Platform:      r.Platform,
		Status:        r.Status,
		TransactionID: r.TransactionID,
		ValidatedAt:   r.ValidatedAt,
	}
}

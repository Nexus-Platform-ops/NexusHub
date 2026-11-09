package payments

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Subscription statuses.
const (
	SubActive    = "active"
	SubPastDue   = "past_due"
	SubCanceled  = "canceled"
	SubExpired   = "expired"
	SubTrialing  = "trialing"
)

// Nexus+ feature gates.
var (
	FeatureRadiusExpansion = "radius_expansion"
	FeatureWhoLikedMe     = "who_liked_me"
	FeatureProfileBoost   = "profile_boost"
)

// AllPremiumFeatures is the full set of Nexus+ features.
var AllPremiumFeatures = []string{
	FeatureRadiusExpansion,
	FeatureWhoLikedMe,
	FeatureProfileBoost,
}

// Subscription represents an active or past subscription.
type Subscription struct {
	ID                 int64
	UserID             int64
	PlanID             string
	Provider           string
	ProviderRef        string
	Status             string
	CurrentPeriodStart time.Time
	CurrentPeriodEnd   time.Time
	CancelAtPeriodEnd  bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Plan represents a subscription plan.
type Plan struct {
	ID        string
	Name      string
	StripeID  string
	AppleID   string
	GoogleID  string
	Duration  time.Duration
	Features  []string
	Active    bool
}

// PurchaseReceipt represents a platform purchase receipt.
type PurchaseReceipt struct {
	ID             int64
	UserID         int64
	SubscriptionID int64
	Platform       string
	ReceiptToken   string
	TransactionID  string
	Status         string
	ValidatedAt    *time.Time
	CreatedAt      time.Time
}

var (
	ErrSubscriptionNotFound = errors.New("subscription not found")
	ErrPlanNotFound         = errors.New("plan not found")
	ErrInvalidReceipt       = errors.New("invalid purchase receipt")
	ErrDuplicateReceipt     = errors.New("duplicate purchase receipt")
)

// SubscriptionStore defines the persistence interface for subscriptions.
type SubscriptionStore interface {
	GetActiveSubscription(ctx context.Context, userID int64) (*Subscription, error)
	GetSubscription(ctx context.Context, id int64) (*Subscription, error)
	CreateSubscription(ctx context.Context, sub *Subscription) error
	UpdateSubscription(ctx context.Context, sub *Subscription) error
	CancelSubscription(ctx context.Context, userID int64) error
	ListPlans(ctx context.Context) ([]Plan, error)
	GetPlan(ctx context.Context, planID string) (*Plan, error)
	SaveReceipt(ctx context.Context, receipt *PurchaseReceipt) error
	GetReceipt(ctx context.Context, platform, token string) (*PurchaseReceipt, error)
	UpdateReceiptStatus(ctx context.Context, id int64, status string, validatedAt *time.Time) error
}

// HasFeature checks whether a subscription's plan includes a given feature.
func (s *Subscription) HasFeature(plan *Plan, feature string) bool {
	if s.Status != SubActive && s.Status != SubTrialing {
		return false
	}
	if time.Now().After(s.CurrentPeriodEnd) {
		return false
	}
	for _, f := range plan.Features {
		if f == feature {
			return true
		}
	}
	return false
}

// IsExpired checks if the subscription period has ended.
func (s *Subscription) IsExpired() bool {
	return time.Now().After(s.CurrentPeriodEnd)
}

// ReceiptValidator validates purchase receipts with platform stores.
type ReceiptValidator interface {
	ValidateApple(ctx context.Context, receipt string) (*ValidationResult, error)
	ValidateGoogle(ctx context.Context, receipt string) (*ValidationResult, error)
}

// ValidationResult is the outcome of a receipt validation.
type ValidationResult struct {
	Valid         bool
	TransactionID string
	ProductID     string
	ExpiresAt     *time.Time
	ErrorMessage  string
}

// StubReceiptValidator is a development-only validator that accepts all receipts.
type StubReceiptValidator struct{}

func (StubReceiptValidator) ValidateApple(_ context.Context, receipt string) (*ValidationResult, error) {
	if receipt == "" {
		return nil, ErrInvalidReceipt
	}
	now := time.Now()
	exp := now.Add(30 * 24 * time.Hour)
	return &ValidationResult{
		Valid:         true,
		TransactionID: fmt.Sprintf("stub_apple_%d", now.UnixNano()),
		ProductID:     "nexus_plus_monthly",
		ExpiresAt:     &exp,
	}, nil
}

func (StubReceiptValidator) ValidateGoogle(_ context.Context, receipt string) (*ValidationResult, error) {
	if receipt == "" {
		return nil, ErrInvalidReceipt
	}
	now := time.Now()
	exp := now.Add(30 * 24 * time.Hour)
	return &ValidationResult{
		Valid:         true,
		TransactionID: fmt.Sprintf("stub_google_%d", now.UnixNano()),
		ProductID:     "nexus_plus_monthly",
		ExpiresAt:     &exp,
	}, nil
}

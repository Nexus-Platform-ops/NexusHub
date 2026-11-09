package payments

import (
	"testing"
	"time"
)

func TestSubscriptionHasFeature(t *testing.T) {
	plan := &Plan{
		ID:       "nexus_plus_monthly",
		Features: []string{FeatureRadiusExpansion, FeatureWhoLikedMe, FeatureProfileBoost},
	}

	active := &Subscription{
		Status:             SubActive,
		CurrentPeriodEnd:   time.Now().Add(24 * time.Hour),
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
	}
	if !active.HasFeature(plan, FeatureRadiusExpansion) {
		t.Error("active sub should have radius_expansion")
	}
	if !active.HasFeature(plan, FeatureWhoLikedMe) {
		t.Error("active sub should have who_liked_me")
	}
	if active.HasFeature(plan, "nonexistent_feature") {
		t.Error("active sub should not have nonexistent feature")
	}

	canceled := &Subscription{
		Status:             SubCanceled,
		CurrentPeriodEnd:   time.Now().Add(24 * time.Hour),
		CurrentPeriodStart: time.Now().Add(-24 * time.Hour),
	}
	if canceled.HasFeature(plan, FeatureRadiusExpansion) {
		t.Error("canceled sub should not have features")
	}

	expired := &Subscription{
		Status:             SubActive,
		CurrentPeriodEnd:   time.Now().Add(-1 * time.Hour),
		CurrentPeriodStart: time.Now().Add(-30 * 24 * time.Hour),
	}
	if expired.HasFeature(plan, FeatureRadiusExpansion) {
		t.Error("expired sub should not have features")
	}
}

func TestSubscriptionIsExpired(t *testing.T) {
	active := &Subscription{
		CurrentPeriodEnd: time.Now().Add(24 * time.Hour),
	}
	if active.IsExpired() {
		t.Error("active sub should not be expired")
	}

	expired := &Subscription{
		CurrentPeriodEnd: time.Now().Add(-1 * time.Hour),
	}
	if !expired.IsExpired() {
		t.Error("past-due sub should be expired")
	}
}

func TestStubReceiptValidatorApple(t *testing.T) {
	v := StubReceiptValidator{}
	result, err := v.ValidateApple(nil, "test-receipt-token")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !result.Valid {
		t.Error("stub should accept all non-empty receipts")
	}
	if result.TransactionID == "" {
		t.Error("missing transaction id")
	}
	if result.ProductID != "nexus_plus_monthly" {
		t.Errorf("product = %q", result.ProductID)
	}
	if result.ExpiresAt == nil {
		t.Error("missing expires_at")
	}

	_, err = v.ValidateApple(nil, "")
	if err != ErrInvalidReceipt {
		t.Errorf("empty receipt = %v, want ErrInvalidReceipt", err)
	}
}

func TestStubReceiptValidatorGoogle(t *testing.T) {
	v := StubReceiptValidator{}
	result, err := v.ValidateGoogle(nil, "test-receipt")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if !result.Valid {
		t.Error("stub should accept")
	}

	_, err = v.ValidateGoogle(nil, "")
	if err != ErrInvalidReceipt {
		t.Errorf("empty receipt = %v", err)
	}
}

func TestAllPremiumFeatures(t *testing.T) {
	if len(AllPremiumFeatures) != 3 {
		t.Errorf("expected 3 features, got %d", len(AllPremiumFeatures))
	}
	featureSet := make(map[string]bool)
	for _, f := range AllPremiumFeatures {
		featureSet[f] = true
	}
	for _, want := range []string{FeatureRadiusExpansion, FeatureWhoLikedMe, FeatureProfileBoost} {
		if !featureSet[want] {
			t.Errorf("missing feature %q", want)
		}
	}
}

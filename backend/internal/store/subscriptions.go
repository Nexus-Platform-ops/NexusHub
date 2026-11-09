package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"nexus/backend/internal/payments"
)

// SubStore implements payments.SubscriptionStore backed by the social pool.
type SubStore struct {
	pool Querier
}

// NewSubStore wraps the social pool for subscription queries.
func NewSubStore(pool Querier) *SubStore {
	return &SubStore{pool: pool}
}

func (s *SubStore) GetActiveSubscription(ctx context.Context, userID int64) (*payments.Subscription, error) {
	var sub payments.Subscription
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, plan_id, provider, provider_ref, status,
		        current_period_start, current_period_end, cancel_at_period_end,
		        created_at, updated_at
		 FROM ledger.subscriptions
		 WHERE user_id=$1 AND status IN ('active','trialing','past_due')
		 ORDER BY current_period_end DESC LIMIT 1`,
		userID).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Provider, &sub.ProviderRef,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, payments.ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get active subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubStore) GetSubscription(ctx context.Context, id int64) (*payments.Subscription, error) {
	var sub payments.Subscription
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, plan_id, provider, provider_ref, status,
		        current_period_start, current_period_end, cancel_at_period_end,
		        created_at, updated_at
		 FROM ledger.subscriptions WHERE id=$1`, id).Scan(
		&sub.ID, &sub.UserID, &sub.PlanID, &sub.Provider, &sub.ProviderRef,
		&sub.Status, &sub.CurrentPeriodStart, &sub.CurrentPeriodEnd,
		&sub.CancelAtPeriodEnd, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, payments.ErrSubscriptionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &sub, nil
}

func (s *SubStore) CreateSubscription(ctx context.Context, sub *payments.Subscription) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO ledger.subscriptions
		   (user_id, plan_id, provider, provider_ref, status,
		    current_period_start, current_period_end, cancel_at_period_end,
		    created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 RETURNING id`,
		sub.UserID, sub.PlanID, sub.Provider, sub.ProviderRef, sub.Status,
		sub.CurrentPeriodStart, sub.CurrentPeriodEnd, sub.CancelAtPeriodEnd,
		sub.CreatedAt, sub.UpdatedAt).Scan(&sub.ID)
}

func (s *SubStore) UpdateSubscription(ctx context.Context, sub *payments.Subscription) error {
	sub.UpdatedAt = time.Now().UTC()
	_, err := s.pool.Exec(ctx,
		`UPDATE ledger.subscriptions
		 SET status=$2, current_period_start=$3, current_period_end=$4,
		     cancel_at_period_end=$5, updated_at=$6
		 WHERE id=$1`,
		sub.ID, sub.Status, sub.CurrentPeriodStart, sub.CurrentPeriodEnd,
		sub.CancelAtPeriodEnd, sub.UpdatedAt)
	return err
}

func (s *SubStore) CancelSubscription(ctx context.Context, userID int64) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE ledger.subscriptions
		 SET cancel_at_period_end=true, status='canceled', updated_at=now()
		 WHERE user_id=$1 AND status IN ('active','trialing')`, userID)
	if err != nil {
		return fmt.Errorf("cancel subscription: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return payments.ErrSubscriptionNotFound
	}
	return nil
}

func (s *SubStore) ListPlans(ctx context.Context) ([]payments.Plan, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, stripe_id, apple_id, google_id, duration, features, active
		 FROM ledger.subscription_plans ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list plans: %w", err)
	}
	defer rows.Close()
	var plans []payments.Plan
	for rows.Next() {
		var p payments.Plan
		var dur string
		if err := rows.Scan(&p.ID, &p.Name, &p.StripeID, &p.AppleID, &p.GoogleID,
			&dur, &p.Features, &p.Active); err != nil {
			return nil, fmt.Errorf("scan plan: %w", err)
		}
		p.Duration = parseInterval(dur)
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func parseInterval(s string) time.Duration {
	s = strings.TrimSpace(s)
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	var months, days int
	fmt.Sscanf(s, "%d mon %d day", &months, &days)
	if months == 0 && days == 0 {
		fmt.Sscanf(s, "%d mon", &months)
	}
	if months == 0 {
		fmt.Sscanf(s, "%d day", &days)
	}
	return time.Duration(months)*30*24*time.Hour + time.Duration(days)*24*time.Hour
}

func (s *SubStore) GetPlan(ctx context.Context, planID string) (*payments.Plan, error) {
	var p payments.Plan
	var dur string
	err := s.pool.QueryRow(ctx,
		`SELECT id, name, stripe_id, apple_id, google_id, duration, features, active
		 FROM ledger.subscription_plans WHERE id=$1`, planID).Scan(
		&p.ID, &p.Name, &p.StripeID, &p.AppleID, &p.GoogleID,
		&dur, &p.Features, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, payments.ErrPlanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get plan: %w", err)
	}
	p.Duration = parseInterval(dur)
	return &p, nil
}

func (s *SubStore) SaveReceipt(ctx context.Context, r *payments.PurchaseReceipt) error {
	return s.pool.QueryRow(ctx,
		`INSERT INTO ledger.purchase_receipts
		   (user_id, subscription_id, platform, receipt_token, transaction_id, status, validated_at, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 RETURNING id`,
		r.UserID, r.SubscriptionID, r.Platform, r.ReceiptToken,
		r.TransactionID, r.Status, r.ValidatedAt, r.CreatedAt).Scan(&r.ID)
}

func (s *SubStore) GetReceipt(ctx context.Context, platform, token string) (*payments.PurchaseReceipt, error) {
	var r payments.PurchaseReceipt
	err := s.pool.QueryRow(ctx,
		`SELECT id, user_id, subscription_id, platform, receipt_token,
		        transaction_id, status, validated_at, created_at
		 FROM ledger.purchase_receipts WHERE platform=$1 AND receipt_token=$2`,
		platform, token).Scan(
		&r.ID, &r.UserID, &r.SubscriptionID, &r.Platform, &r.ReceiptToken,
		&r.TransactionID, &r.Status, &r.ValidatedAt, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get receipt: %w", err)
	}
	return &r, nil
}

func (s *SubStore) UpdateReceiptStatus(ctx context.Context, id int64, status string, validatedAt *time.Time) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE ledger.purchase_receipts SET status=$2, validated_at=$3 WHERE id=$1`,
		id, status, validatedAt)
	return err
}

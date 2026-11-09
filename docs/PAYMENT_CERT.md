# Payment Certification — Phase F

> Status: **IN PROGRESS** — Sandbox testing pending live aggregator credentials.

## 1. Payment Providers

| Provider | Type | Status | Notes |
|----------|------|--------|-------|
| Stripe (International) | Card / Wallet | ✓ Adapter implemented | PaymentIntent API, idempotency keys, webhook HMAC |
| KBZPay | Myanmar Mobile Wallet | ⏳ BLOCKED | Pending live aggregator credentials |
| Wave Money | Myanmar Mobile Wallet | ⏳ BLOCKED | Pending live aggregator credentials |
| Generic HTTP Aggregator | Fallback | ✓ Implemented | Existing provider, HMAC-signed webhooks |

## 2. Security Controls

### 2.1 Idempotency
- **Payment Intents**: `provider_ref` unique index prevents double-credit from same payment reference
- **Stripe**: `Idempotency-Key` header set to intent ID on PaymentIntent creation
- **Coin Ledger**: `request_id` unique index on `coin_transactions` (kind=topup) prevents duplicate credits
- **Webhook Replay**: Already-paid intents return `{"duplicate": true}` without re-crediting

### 2.2 Webhook Verification
- **Generic HTTP**: HMAC-SHA256 over `timestamp + "." + body`, 5-minute tolerance window
- **Stripe**: `Stripe-Signature` header with `t=` timestamp + `v1=` HMAC-SHA256, 5-minute tolerance
- Both reject missing/malformed/stale signatures before any state change

### 2.3 Amount Integrity
- Server-priced packages only — client sends `package_id`, never `amount`
- Webhook amount must match intent amount exactly (mismatch → 400)
- Currency locked to `MMK` for local providers, configurable for Stripe

### 2.4 Secret Management
- `PAYMENT_SIGNING_KEY` ≥ 32 chars, distinct from `JWT_SECRET`
- `STRIPE_SECRET_KEY` and `STRIPE_WEBHOOK_SECRET` required when `PAYMENT_PROVIDER=stripe`
- Dev placeholders rejected in staging/production (config.Validate)

## 3. Test Cases

### 3.1 Coin Payment Lifecycle (8 cases)
| # | Test | File | Status |
|---|------|------|--------|
| 1 | Intent creation with valid package | `api/payments_test.go` | ✓ PASS |
| 2 | Intent privacy (user A can't see B's) | `api/payments_test.go` | ✓ PASS |
| 3 | Webhook credits coins correctly | `api/payments_test.go` | ✓ PASS |
| 4 | Replay doesn't double-credit | `api/payments_test.go` | ✓ PASS |
| 5 | Bad signature rejected | `api/payments_test.go` | ✓ PASS |
| 6 | Stale timestamp rejected | `api/payments_test.go` | ✓ PASS |
| 7 | Wrong amount rejected | `api/payments_test.go` | ✓ PASS |
| 8 | Unknown intent rejected | `api/payments_test.go` | ✓ PASS |

### 3.2 Failure & Recovery (4 cases)
| # | Test | File | Status |
|---|------|------|--------|
| 9 | Failed intent can be credited by late "paid" | `api/payments_test.go` | ✓ PASS |
| 10 | Expired intent rejected | `api/payments_test.go` | ✓ PASS |
| 11 | Provider failure marks intent failed | `api/payments_test.go` | ✓ PASS |
| 12 | Unknown package_id → 400 | `api/payments_test.go` | ✓ PASS |

### 3.3 Stripe Adapter (8 cases)
| # | Test | File | Status |
|---|------|------|--------|
| 13 | Charge creation with idempotency key | `payments/stripe_test.go` | ✓ PASS |
| 14 | Error mapping (card declined) | `payments/stripe_test.go` | ✓ PASS |
| 15 | Bad JSON response handling | `payments/stripe_test.go` | ✓ PASS |
| 16 | Webhook signature verification | `payments/stripe_test.go` | ✓ PASS |
| 17 | Status mapping (6 Stripe → 3 internal) | `payments/stripe_test.go` | ✓ PASS |
| 18 | PaymentIntent parsing from webhook | `payments/stripe_test.go` | ✓ PASS |
| 19 | Dispute event parsing | `payments/stripe_test.go` | ✓ PASS |
| 20 | Refund creation | `payments/stripe_test.go` | ✓ PASS |

### 3.4 Subscription (5 cases)
| # | Test | File | Status |
|---|------|------|--------|
| 21 | Feature gating (active sub) | `payments/subscription_test.go` | ✓ PASS |
| 22 | Feature gating (canceled/expired) | `payments/subscription_test.go` | ✓ PASS |
| 23 | Expiry detection | `payments/subscription_test.go` | ✓ PASS |
| 24 | Apple receipt validation (stub) | `payments/subscription_test.go` | ✓ PASS |
| 25 | Google receipt validation (stub) | `payments/subscription_test.go` | ✓ PASS |

**Total: 25 test cases — 25 PASS**

## 4. Subscription Plans (Nexus+)

| Plan ID | Duration | Features |
|---------|----------|----------|
| `nexus_plus_monthly` | 1 month | Radius Expansion, Who-Liked-Me, Profile Boost |
| `nexus_plus_quarterly` | 3 months | Radius Expansion, Who-Liked-Me, Profile Boost |

### 4.1 Receipt Validation
- **Development**: `StubReceiptValidator` accepts all non-empty receipts
- **Production**: Requires real Apple StoreKit 2 / Google Billing Library validation
- Receipts stored in `ledger.purchase_receipts` with unique constraint on `(platform, receipt_token)`

### 4.2 Feature Gating
- `GET /api/v1/subscriptions/features/{feature}` — returns `{"enabled": true/false}`
- Checks: active status + unexpired period + plan includes feature
- Used by Flutter UI to show/hide premium features

## 5. Database Migrations

| Migration | Tables | Purpose |
|-----------|--------|---------|
| `003_market_ledger.sql` | `ledger.coin_transactions` | Append-only coin ledger with idempotency |
| `007_payments.sql` | `ledger.payment_intents` | Payment lifecycle with provider_ref uniqueness |
| `008_subscriptions.sql` | `ledger.subscription_plans`, `ledger.subscriptions`, `ledger.purchase_receipts` | Nexus+ subscription system |

## 6. API Endpoints

### Coin Payments
| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/coins/intents` | Bearer | Create payment intent |
| GET | `/api/v1/coins/intents/{id}` | Bearer | Poll intent status |
| POST | `/api/v1/payments/webhook` | HMAC | Provider payment callback |
| GET | `/api/v1/coins/balance` | Bearer | Current coin balance |
| GET | `/api/v1/coins/ledger` | Bearer | Transaction history |

### Subscriptions (Phase F)
| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| GET | `/api/v1/subscriptions/plans` | Bearer | List available plans |
| GET | `/api/v1/subscriptions/me` | Bearer | Current subscription |
| POST | `/api/v1/subscriptions` | Bearer | Create from receipt |
| DELETE | `/api/v1/subscriptions` | Bearer | Cancel subscription |
| GET | `/api/v1/subscriptions/features/{feature}` | Bearer | Check feature access |
| POST | `/api/v1/webhooks/stripe` | Stripe Sig | Stripe lifecycle events |

## 7. Outstanding Items

- [ ] Live KBZPay/Wave aggregator credentials for end-to-end testing
- [ ] Stripe sandbox certification (test mode card flows)
- [ ] Apple StoreKit 2 receipt validation (production)
- [ ] Google Play Billing Library receipt validation (production)
- [ ] Dispute handling automation (auto-refund on chargeback)
- [ ] Revenue reconciliation dashboard

## 8. Compliance Notes

- **PCI DSS**: No card data touches our servers — Stripe handles all card processing
- **Myanmar Central Bank**: MMK transactions via licensed aggregators only
- **GDPR/DSR**: Payment records included in data export; deletion follows DSR flow
- **Age Rating**: Financial transactions declared in store privacy labels (17+)

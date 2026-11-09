-- 008_subscriptions.sql — Nexus+ subscription plans and receipts.
-- Subscriptions gate premium features (radius expansion, who-liked-me, boost).
-- Receipts store platform purchase tokens for server-side validation.

CREATE TABLE IF NOT EXISTS ledger.subscription_plans (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    stripe_id   text NOT NULL DEFAULT '',   -- Stripe Price ID (price_...)
    apple_id    text NOT NULL DEFAULT '',   -- App Store product ID
    google_id   text NOT NULL DEFAULT '',   -- Google Play product ID
    duration    interval NOT NULL DEFAULT '1 month'::interval,
    features    text[] NOT NULL DEFAULT '{}',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ledger.subscriptions (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL,
    plan_id         text NOT NULL REFERENCES ledger.subscription_plans(id),
    provider        text NOT NULL DEFAULT '' CHECK (provider IN ('','stripe','apple','google')),
    provider_ref    text NOT NULL DEFAULT '',       -- Stripe sub_xxx / Apple original_tx / Google order
    status          text NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active','past_due','canceled','expired','trialing')),
    current_period_start timestamptz NOT NULL,
    current_period_end   timestamptz NOT NULL,
    cancel_at_period_end boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, provider, provider_ref)
);

CREATE INDEX IF NOT EXISTS idx_subscriptions_user
    ON ledger.subscriptions (user_id, status);
CREATE INDEX IF NOT EXISTS idx_subscriptions_period
    ON ledger.subscriptions (current_period_end)
    WHERE status = 'active';

CREATE TABLE IF NOT EXISTS ledger.purchase_receipts (
    id              bigserial PRIMARY KEY,
    user_id         bigint NOT NULL,
    subscription_id bigint REFERENCES ledger.subscriptions(id),
    platform        text NOT NULL CHECK (platform IN ('apple','google')),
    receipt_token   text NOT NULL,
    transaction_id  text NOT NULL DEFAULT '',
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','validated','invalid','expired')),
    validated_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (platform, receipt_token)
);

CREATE INDEX IF NOT EXISTS idx_receipts_user
    ON ledger.purchase_receipts (user_id, created_at DESC);

-- Seed the default Nexus+ plan.
INSERT INTO ledger.subscription_plans (id, name, duration, features)
VALUES ('nexus_plus_monthly', 'Nexus+ Monthly', '1 month'::interval,
        ARRAY['radius_expansion','who_liked_me','profile_boost'])
ON CONFLICT (id) DO NOTHING;

INSERT INTO ledger.subscription_plans (id, name, duration, features)
VALUES ('nexus_plus_quarterly', 'Nexus+ Quarterly', '3 months'::interval,
        ARRAY['radius_expansion','who_liked_me','profile_boost'])
ON CONFLICT (id) DO NOTHING;

GRANT SELECT, INSERT, UPDATE ON ledger.subscriptions TO nexus_social;
GRANT SELECT, INSERT, UPDATE ON ledger.subscription_plans TO nexus_social;
GRANT SELECT, INSERT, UPDATE ON ledger.purchase_receipts TO nexus_social;
GRANT USAGE ON SEQUENCE ledger.subscriptions_id_seq TO nexus_social;
GRANT USAGE ON SEQUENCE ledger.purchase_receipts_id_seq TO nexus_social;

-- 007_payments.sql — provider-backed coin top-ups.
-- An intent is created with a server-priced package; the provider's signed
-- webhook marks it paid and credits the append-only ledger exactly once.
CREATE TABLE IF NOT EXISTS ledger.payment_intents (
    id           text PRIMARY KEY CHECK (length(id) BETWEEN 16 AND 64),
    user_id      bigint NOT NULL,
    package_id   text NOT NULL,
    coins        bigint NOT NULL CHECK (coins > 0),
    amount_mmk   bigint NOT NULL CHECK (amount_mmk > 0),
    currency     text NOT NULL DEFAULT 'MMK',
    provider     text NOT NULL,
    provider_ref text NOT NULL DEFAULT '',
    payment_url  text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending','paid','failed','expired')),
    created_at   timestamptz NOT NULL DEFAULT now(),
    paid_at      timestamptz,
    expires_at   timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_payment_intents_user
    ON ledger.payment_intents (user_id, created_at DESC);

-- A provider reference may back at most one intent, so a webhook can never
-- credit a second intent with the same payment.
CREATE UNIQUE INDEX IF NOT EXISTS idx_payment_intents_provider_ref
    ON ledger.payment_intents (provider_ref)
    WHERE provider_ref <> '';

GRANT SELECT, INSERT, UPDATE ON ledger.payment_intents TO nexus_social;
-- 003_market_ledger.sql — Nexus Coins ledger and hyper-local marketplace.
-- The ledger is append-only: balances are always derived from SUM(amount).
-- The nexus_social role owns access; dating data never joins these tables.
CREATE SCHEMA IF NOT EXISTS ledger;

CREATE TABLE IF NOT EXISTS ledger.coin_transactions (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL,
    amount     bigint NOT NULL,
    kind       text NOT NULL CHECK (kind IN ('topup','purchase','sale','reward')),
    request_id text,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- Idempotency: one ledger row per payment request id.
CREATE UNIQUE INDEX IF NOT EXISTS idx_ledger_request_id
    ON ledger.coin_transactions (request_id)
    WHERE request_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_ledger_user ON ledger.coin_transactions (user_id, id DESC);

CREATE SCHEMA IF NOT EXISTS market;

CREATE TABLE IF NOT EXISTS market.products (
    id          bigserial PRIMARY KEY,
    seller_id   bigint NOT NULL,
    title       text NOT NULL CHECK (length(title) BETWEEN 1 AND 120),
    description text NOT NULL DEFAULT '',
    price_coins bigint NOT NULL DEFAULT 0 CHECK (price_coins >= 0),
    price_mmk   bigint NOT NULL DEFAULT 0 CHECK (price_mmk >= 0),
    township    text NOT NULL DEFAULT '',
    image_url   text NOT NULL DEFAULT '',
    active      boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    CHECK (price_coins > 0 OR price_mmk > 0)
);

CREATE TABLE IF NOT EXISTS market.orders (
    id          bigserial PRIMARY KEY,
    product_id  bigint NOT NULL REFERENCES market.products(id),
    buyer_id    bigint NOT NULL,
    price_coins bigint NOT NULL CHECK (price_coins > 0),
    status      text NOT NULL DEFAULT 'paid',
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_products_active ON market.products (active, township);

GRANT USAGE ON SCHEMA ledger TO nexus_social;
GRANT SELECT, INSERT, DELETE ON ALL TABLES IN SCHEMA ledger TO nexus_social;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA ledger TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA ledger
    GRANT SELECT, INSERT, DELETE ON TABLES TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA ledger
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_social;

GRANT USAGE ON SCHEMA market TO nexus_social;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA market TO nexus_social;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA market TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA market
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA market
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_social;

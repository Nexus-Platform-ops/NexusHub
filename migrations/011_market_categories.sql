-- 011_market_categories.sql — Add category and search support to market.

-- Add category column for filtering
ALTER TABLE market.products ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT 'other';
CREATE INDEX IF NOT EXISTS idx_products_category ON market.products (category, active);

-- Add full-text search index
ALTER TABLE market.products ADD COLUMN IF NOT EXISTS search_vector tsvector
    GENERATED ALWAYS AS (to_tsvector('english', title || ' ' || description)) STORED;
CREATE INDEX IF NOT EXISTS idx_products_search ON market.products USING gin (search_vector);

-- Add rating system for sellers
CREATE TABLE IF NOT EXISTS market.seller_ratings (
    id         bigserial PRIMARY KEY,
    seller_id  bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    buyer_id   bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    product_id bigint REFERENCES market.products(id) ON DELETE SET NULL,
    rating     smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (buyer_id, product_id)
);

CREATE INDEX IF NOT EXISTS idx_seller_ratings ON market.seller_ratings (seller_id, rating);

-- Add verified seller badge (computed from phone verification + transaction count)
CREATE OR REPLACE VIEW market.verified_sellers AS
SELECT
    u.id as seller_id,
    u.phone_verified,
    COUNT(DISTINCT o.id) as completed_sales,
    COALESCE(AVG(r.rating), 0) as avg_rating,
    COUNT(DISTINCT r.id) as rating_count
FROM social.users u
LEFT JOIN market.orders o ON o.buyer_id != u.id AND o.status = 'paid'
LEFT JOIN market.seller_ratings r ON r.seller_id = u.id
WHERE u.id IN (SELECT DISTINCT seller_id FROM market.products WHERE active = true)
GROUP BY u.id, u.phone_verified;

GRANT SELECT ON market.verified_sellers TO nexus_social;

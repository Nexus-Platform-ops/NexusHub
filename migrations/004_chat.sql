-- 004_chat.sql — unified chat threads and messages (personal + business).
CREATE TABLE IF NOT EXISTS social.chat_threads (
    id         bigserial PRIMARY KEY,
    mode       text NOT NULL CHECK (mode IN ('personal','business')),
    owner_id   bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    peer_id    bigint REFERENCES social.users(id) ON DELETE CASCADE,
    product_id bigint REFERENCES market.products(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT thread_peer_self CHECK (peer_id IS NULL OR peer_id <> owner_id),
    CONSTRAINT business_needs_product CHECK (mode <> 'business' OR product_id IS NOT NULL)
);

CREATE TABLE IF NOT EXISTS social.chat_messages (
    id         bigserial PRIMARY KEY,
    thread_id  bigint NOT NULL REFERENCES social.chat_threads(id) ON DELETE CASCADE,
    sender_id  bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_messages_thread ON social.chat_messages (thread_id, id);

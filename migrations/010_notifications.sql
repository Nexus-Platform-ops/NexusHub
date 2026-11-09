-- 010_notifications.sql — push notification system for user retention.
-- Stores in-app notifications that drive re-engagement:
-- "3 people nearby are interested in you", "New message", etc.

CREATE SCHEMA IF NOT EXISTS social;

CREATE TABLE IF NOT EXISTS social.notifications (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    type       text NOT NULL CHECK (type IN (
        'match',           -- dating match
        'like',            -- someone liked your post
        'comment',         -- someone commented on your post
        'message',         -- new chat message
        'nearby_interest', -- people nearby are interested
        'market_sale',     -- someone bought your item
        'market_inquiry',  -- someone inquired about your item
        'friend_request',  -- friend request received
        'system'           -- system announcement
    )),
    title      text NOT NULL,
    body       text NOT NULL DEFAULT '',
    actor_id   bigint REFERENCES social.users(id) ON DELETE SET NULL,
    target_id  bigint,  -- post_id, product_id, thread_id, etc.
    read       boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_unread
    ON social.notifications (user_id, read, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_created
    ON social.notifications (created_at DESC);

GRANT USAGE ON SCHEMA social TO nexus_social;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA social TO nexus_social;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA social TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA social
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA social
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_social;

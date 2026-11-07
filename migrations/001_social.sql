-- 001_social.sql — social schema: accounts, user profiles, friendships,
-- posts, reactions, comments. Grants are scoped to the nexus_social role;
-- the nexus_dating role receives NO privileges here except the minimal
-- friend_ids view used for dating friend-exclusion.
CREATE SCHEMA IF NOT EXISTS social;

CREATE TABLE IF NOT EXISTS social.users (
    id            bigserial PRIMARY KEY,
    phone         text UNIQUE NOT NULL,
    display_name  text NOT NULL,
    phone_verified boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS social.user_profiles (
    user_id     bigint PRIMARY KEY REFERENCES social.users(id) ON DELETE CASCADE,
    real_name   text NOT NULL DEFAULT '',
    avatar_url  text NOT NULL DEFAULT '',
    township    text NOT NULL DEFAULT '',
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS social.friendships (
    user_id    bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    friend_id  bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    status     text NOT NULL DEFAULT 'accepted',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, friend_id),
    CHECK (user_id <> friend_id)
);

CREATE TABLE IF NOT EXISTS social.posts (
    id         bigserial PRIMARY KEY,
    user_id    bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    content    text NOT NULL CHECK (length(content) BETWEEN 1 AND 2000),
    image_url  text NOT NULL DEFAULT '',
    township   text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS social.reactions (
    post_id    bigint NOT NULL REFERENCES social.posts(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    kind       text NOT NULL DEFAULT 'like' CHECK (kind IN ('like','love')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (post_id, user_id)
);

CREATE TABLE IF NOT EXISTS social.comments (
    id         bigserial PRIMARY KEY,
    post_id    bigint NOT NULL REFERENCES social.posts(id) ON DELETE CASCADE,
    user_id    bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    body       text NOT NULL CHECK (length(body) BETWEEN 1 AND 500),
    created_at timestamptz NOT NULL DEFAULT now()
);

GRANT USAGE ON SCHEMA social TO nexus_social;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA social TO nexus_social;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA social TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA social
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA social
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_social;

-- Minimal friend-id view: the dating role can only see user_id pairs,
-- never names, posts or profile content. USAGE on the schema is required
-- to reach the view; the dating role still has NO table privileges here.
CREATE OR REPLACE VIEW social.friend_ids AS
    SELECT user_id, friend_id FROM social.friendships WHERE status = 'accepted';
GRANT USAGE ON SCHEMA social TO nexus_dating;
GRANT SELECT ON social.friend_ids TO nexus_dating;
REVOKE ALL ON social.users, social.user_profiles, social.friendships,
    social.posts, social.reactions, social.comments FROM nexus_dating;

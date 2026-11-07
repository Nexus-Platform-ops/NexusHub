-- 005_social_logins.sql — social login identities (Facebook / TikTok / Telegram).
--
-- ISOLATION CONTRACT: real identity data retrieved from social providers
-- (real names, profile pictures, e-mails) lives ONLY in the `identity`
-- schema below. It is never copied into social.users, social.user_profiles,
-- dating.* or any API payload that feeds the Dating Profile or Anonymous
-- Radar features. The nexus_dating role holds zero privileges on this schema
-- and the dating handlers never reference it, so a leak is structurally
-- impossible rather than a matter of discipline.
--
-- Social-only accounts: the phone column becomes nullable so a user created
-- through a social provider can exist before (or without) phone verification.

CREATE SCHEMA IF NOT EXISTS identity;

ALTER TABLE social.users ALTER COLUMN phone DROP NOT NULL;

CREATE TABLE IF NOT EXISTS identity.social_identities (
    id               bigserial PRIMARY KEY,
    user_id          bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    provider         text   NOT NULL CHECK (provider IN ('facebook', 'tiktok', 'telegram')),
    provider_user_id text   NOT NULL,
    real_name        text   NOT NULL DEFAULT '',
    avatar_url       text   NOT NULL DEFAULT '',
    email            text   NOT NULL DEFAULT '',
    linked_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS idx_social_identities_user ON identity.social_identities (user_id);

GRANT USAGE ON SCHEMA identity TO nexus_social;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity TO nexus_social;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA identity TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA identity
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexus_social;
ALTER DEFAULT PRIVILEGES IN SCHEMA identity
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_social;

-- Belt and braces: the dating role can never read provider identities even
-- if a future query tried; PUBLIC keeps no schema privileges either.
REVOKE ALL ON SCHEMA identity FROM nexus_dating, PUBLIC;

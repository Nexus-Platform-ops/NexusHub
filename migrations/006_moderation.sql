-- 006_moderation.sql — safety layer for the dating/radar surface:
--
--   * social.blocks        — symmetric suppression between two accounts
--   * social.reports       — user-submitted abuse reports with an admin queue
--   * social.users.birth_date — 18+ gate input (never exposed to the dating role)
--   * social.blocked_ids   — minimal user-id-pair view for the dating role,
--                            mirroring the existing social.friend_ids contract.
--
-- The dating role still holds ZERO privileges on the social tables; it may
-- only read the pair-only view so radar/whisper can suppress blocked users.

ALTER TABLE social.users ADD COLUMN IF NOT EXISTS birth_date date;

CREATE TABLE IF NOT EXISTS social.blocks (
    blocker_id bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    blocked_id bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE INDEX IF NOT EXISTS idx_blocks_blocked ON social.blocks (blocked_id);

CREATE TABLE IF NOT EXISTS social.reports (
    id          bigserial PRIMARY KEY,
    reporter_id bigint NOT NULL REFERENCES social.users(id) ON DELETE CASCADE,
    target_kind text NOT NULL CHECK (target_kind IN ('user','post','product','whisper')),
    target_id   text NOT NULL CHECK (length(target_id) BETWEEN 1 AND 64),
    reason      text NOT NULL CHECK (reason IN
                    ('harassment','spam','scam','nudity','underage','impersonation','other')),
    details     text NOT NULL DEFAULT '' CHECK (length(details) <= 1000),
    status      text NOT NULL DEFAULT 'open'
                    CHECK (status IN ('open','reviewing','actioned','dismissed')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_reports_queue ON social.reports (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_reports_reporter ON social.reports (reporter_id, created_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON social.blocks, social.reports TO nexus_social;
GRANT USAGE, SELECT ON SEQUENCE social.reports_id_seq TO nexus_social;

-- Pair-only projection for the dating role (same pattern as friend_ids).
CREATE OR REPLACE VIEW social.blocked_ids AS
    SELECT blocker_id, blocked_id FROM social.blocks;
GRANT SELECT ON social.blocked_ids TO nexus_dating;

REVOKE ALL ON social.blocks, social.reports FROM nexus_dating;

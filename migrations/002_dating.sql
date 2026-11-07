-- 002_dating.sql — strictly separated dating schema. The nexus_social role
-- holds ZERO privileges here; only nexus_dating may touch these tables, and
-- even it must set app.dating_access='service' (set per connection only for
-- authenticated API requests) because of the row level security policy.
CREATE SCHEMA IF NOT EXISTS dating;

CREATE TABLE IF NOT EXISTS dating.dating_profiles (
    user_id                bigint PRIMARY KEY,
    nickname               text NOT NULL,
    bio                    text NOT NULL DEFAULT '',
    avatar_url             text NOT NULL DEFAULT '',
    is_hidden_from_friends boolean NOT NULL DEFAULT true,
    updated_at             timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS dating.radar_presence (
    user_id    bigint PRIMARY KEY,
    geog       geography(Point, 4326) NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_radar_geog ON dating.radar_presence USING gist (geog);

CREATE TABLE IF NOT EXISTS dating.swipes (
    user_id    bigint NOT NULL,
    target_id  bigint NOT NULL,
    action     text NOT NULL CHECK (action IN ('like','pass')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_id)
);

CREATE TABLE IF NOT EXISTS dating.matches (
    a_id       bigint NOT NULL,
    b_id       bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (a_id, b_id),
    CHECK (a_id < b_id)
);

ALTER TABLE dating.dating_profiles ENABLE ROW LEVEL SECURITY;
ALTER TABLE dating.dating_profiles FORCE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS dating_service_access ON dating.dating_profiles;
CREATE POLICY dating_service_access ON dating.dating_profiles
    USING (current_setting('app.dating_access', true) = 'service')
    WITH CHECK (current_setting('app.dating_access', true) = 'service');

GRANT USAGE ON SCHEMA dating TO nexus_dating;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA dating TO nexus_dating;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA dating TO nexus_dating;
ALTER DEFAULT PRIVILEGES IN SCHEMA dating
    GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO nexus_dating;
ALTER DEFAULT PRIVILEGES IN SCHEMA dating
    GRANT USAGE, SELECT ON SEQUENCES TO nexus_dating;

#!/usr/bin/env bash
# One-time local bootstrap: start PostgreSQL + Redis, create the two isolated
# DB roles and the nexus database. Safe to re-run.
set -euo pipefail

sudo service postgresql start
sudo service redis-server start || true

sudo -u postgres psql -v ON_ERROR_STOP=1 <<'SQL'
ALTER USER postgres PASSWORD 'postgres';
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_social') THEN
    CREATE ROLE nexus_social LOGIN PASSWORD 'dev_social';
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_dating') THEN
    CREATE ROLE nexus_dating LOGIN PASSWORD 'dev_dating';
  END IF;
END $$;
SELECT 'CREATE DATABASE nexus'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'nexus') \gexec
SQL

bash "$(dirname "$0")/migrate.sh"
redis-cli ping
echo "bootstrap complete"

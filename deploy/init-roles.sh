#!/usr/bin/env bash
# Creates the two isolated API roles with passwords supplied by the
# environment. Runs once, on first database initialisation, so production
# never inherits the dev_social / dev_dating credentials from init-roles.sql.
#
# Required: NEXUS_SOCIAL_PASSWORD, NEXUS_DATING_PASSWORD
set -euo pipefail

: "${NEXUS_SOCIAL_PASSWORD:?NEXUS_SOCIAL_PASSWORD is required}"
: "${NEXUS_DATING_PASSWORD:?NEXUS_DATING_PASSWORD is required}"

psql -v ON_ERROR_STOP=1 \
  --username "${POSTGRES_USER:-nexus}" \
  --dbname "${POSTGRES_DB:-nexus}" \
  -v social_pw="$NEXUS_SOCIAL_PASSWORD" \
  -v dating_pw="$NEXUS_DATING_PASSWORD" <<'SQL'
SELECT format('CREATE ROLE nexus_social LOGIN PASSWORD %L', :'social_pw')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_social');
\gexec
SELECT format('CREATE ROLE nexus_dating LOGIN PASSWORD %L', :'dating_pw')
WHERE NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'nexus_dating');
\gexec
SQL

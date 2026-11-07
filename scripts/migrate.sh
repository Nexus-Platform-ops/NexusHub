#!/usr/bin/env bash
# Applies migrations in lexical order as the postgres superuser.
# Requires the database from scripts/bootstrap.sh (idempotent via
# schema_migrations is handled by the API-side runner; psql runs are -f).
set -euo pipefail
DIR="$(cd "$(dirname "$0")/.." && pwd)/migrations"
URL="${MIGRATIONS_URL:-postgres://postgres:postgres@127.0.0.1:5432/nexus?sslmode=disable}"
for f in "$DIR"/*.sql; do
  echo "applying $(basename "$f")"
  psql "$URL" -v ON_ERROR_STOP=1 -f "$f" >/dev/null
done
echo "migrations applied"

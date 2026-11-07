#!/usr/bin/env bash
# Daily logical backup of the Nexus database.
#
# Two modes:
#   * compose (default in production): set BACKUP_COMPOSE_FILE so pg_dump runs
#     inside the db container (the production DB publishes no host port).
#   * direct: set DATABASE_URL (or BACKUP_DATABASE_URL) and run where a
#     PostgreSQL client is available.
#
# Optional:
#   BACKUP_DIR             local directory (default ./backups)
#   BACKUP_S3_URI          s3://bucket/prefix — requires the aws CLI
#   BACKUP_RETENTION_DAYS  local dump retention (default 14)
#
# Cron example (/etc/cron.d/nexus-backup):
#   30 2 * * * root cd /srv/nexus && set -a && . ./.env.production && set +a \
#     && ./scripts/backup.sh >> /var/log/nexus-backup.log 2>&1
#
# Restore (fresh database, roles already created from the env):
#   dc="docker compose --env-file .env.production -f docker-compose.prod.yml"
#   1. $dc stop api
#   2. $dc exec -T db pg_restore -U nexus -d nexus --no-owner --clean --if-exists < dump
#   3. $dc run --rm migrate   # re-applies grants / pending schema
#   4. $dc start api
set -euo pipefail

dir="${BACKUP_DIR:-./backups}"
retention="${BACKUP_RETENTION_DAYS:-14}"
stamp="$(date -u +%Y%m%dT%H%M%SZ)"
file="$dir/nexus-$stamp.dump"
mkdir -p "$dir"

if [[ -n "${BACKUP_COMPOSE_FILE:-}" ]]; then
  compose_env="${BACKUP_COMPOSE_ENV:-.env.production}"
  docker compose --env-file "$compose_env" -f "$BACKUP_COMPOSE_FILE" \
    exec -T db pg_dump -U nexus -d nexus --format=custom \
    --no-owner --no-privileges >"$file"
else
  url="${BACKUP_DATABASE_URL:-${DATABASE_URL:-}}"
  if [[ -z "$url" ]]; then
    echo "DATABASE_URL (or BACKUP_DATABASE_URL / BACKUP_COMPOSE_FILE) is required" >&2
    exit 2
  fi
  pg_dump --format=custom --no-owner --no-privileges --file="$file" "$url"
fi

if [[ ! -s "$file" ]]; then
  echo "backup failed: $file is empty" >&2
  exit 3
fi

if [[ -n "${BACKUP_S3_URI:-}" ]]; then
  if ! command -v aws >/dev/null 2>&1; then
    echo "aws CLI is required for BACKUP_S3_URI" >&2
    exit 4
  fi
  aws s3 cp "$file" "${BACKUP_S3_URI%/}/$(basename "$file")" --only-show-errors
fi

find "$dir" -maxdepth 1 -name 'nexus-*.dump' -type f \
  -mtime +"$retention" -delete

echo "backup written: $file ($(du -h "$file" | cut -f1))"

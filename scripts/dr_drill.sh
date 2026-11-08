#!/usr/bin/env bash
# DR drill: primary AZ failure → standby promotion.
#
#   Targets (spec): RPO ≤ 15min, RTO ≤ 1h.
#
# Usage:
#   PRIMARY_URL=postgres://user:pass@primary:5432/db \
#   STANDBY_URL=postgres://user:pass@standby:5432/db \
#   ./scripts/dr_drill.sh [evidence/dr-drill-result.json]
#
# What it measures:
#   RPO — replication lag (seconds) at the moment of failure, i.e. how much
#         WAL the standby has not replayed yet. Also reported in bytes via
#         primary flush LSN minus standby replay LSN.
#   RTO — wall time from "primary declared dead" (t_fail) to "standby
#         accepts writes" (t_promote, verified by a write probe).
#
# Standby-less environments: the failover/RPO steps are recorded as "skipped"
# and the script still runs the pg_dump → fresh-restore timing test, which is
# the locally measurable RTO proxy. Exit code is 0 whenever the JSON was
# written; pass/fail is in the document, not the exit code.
set -uo pipefail

OUT="${1:-evidence/dr-drill-result.json}"
PRIMARY_URL="${PRIMARY_URL:-}"
STANDBY_URL="${STANDBY_URL:-}"
RPO_TARGET_SECONDS=900     # 15 min
RTO_TARGET_SECONDS=3600    # 1 h

now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }
now_epoch() { date +%s; }

PSQL_PRIMARY=()
PSQL_STANDBY=()
command -v psql >/dev/null 2>&1 && PSQL_PRIMARY=(psql) && PSQL_STANDBY=(psql)
if [ -n "$PRIMARY_URL" ] && [ ${#PSQL_PRIMARY[@]} -eq 0 ]; then
  echo "dr_drill: psql not found on PATH and PRIMARY_URL is set" >&2
fi

q() { # q <url> <sql>
  psql "$1" -X -q -t -A -c "$2" 2>&1
}

t_start=$(now_epoch)
started_at=$(now_iso)

failover_status="skipped"
standby_promote_seconds="null"
rpo_lag_seconds="null"
rpo_wal_delta_bytes="null"
promote_probe="skipped"
notes=()

# ---------------------------------------------------------------- standby path
if [ -n "$PRIMARY_URL" ] && [ -n "$STANDBY_URL" ] && [ ${#PSQL_PRIMARY[@]} -gt 0 ]; then
  primary_in_recovery=$(q "$PRIMARY_URL" "SELECT pg_is_in_recovery()")
  standby_in_recovery=$(q "$STANDBY_URL" "SELECT pg_is_in_recovery()")
  if [ "$primary_in_recovery" = "t" ] || [ "$standby_in_recovery" != "t" ]; then
    notes+=("unexpected recovery flags: primary_in_recovery=$primary_in_recovery standby_in_recovery=$standby_in_recovery")
  fi

  # WAL positions just before the "failure".
  primary_lsn=$(q "$PRIMARY_URL" "SELECT pg_current_wal_lsn()")
  standby_replay=$(q "$STANDBY_URL" "SELECT pg_last_wal_replay_lsn()")
  replay_ts=$(q "$STANDBY_URL" "SELECT EXTRACT(EPOCH FROM now() - pg_last_xact_replay_timestamp())")

  # t_fail: primary declared dead. t_promote: standby promoted + write probe.
  t_fail=$(now_epoch)
  q "$STANDBY_URL" "SELECT pg_promote()" >/dev/null 2>&1
  promote_rc=$?
  t_promote=$(now_epoch)

  # Write probe: the promoted standby must accept a write.
  probe=$(q "$STANDBY_URL" "CREATE TEMP TABLE dr_drill_probe(x int); INSERT INTO dr_drill_probe VALUES (1); SELECT count(*) FROM dr_drill_probe")
  if [ "$probe" = "1" ]; then
    promote_probe="write_ok"
    failover_status="performed"
  else
    promote_probe="FAILED: $probe"
    failover_status="failed"
  fi
  [ $promote_rc -ne 0 ] && notes+=("pg_promote returned $promote_rc")

  standby_promote_seconds=$((t_promote - t_fail))
  rpo_lag_seconds=${replay_ts%.*}
  rpo_wal_delta_bytes=$(q "$PRIMARY_URL" "SELECT pg_wal_lsn_diff('$primary_lsn', '$standby_replay')")
  [ -z "$rpo_wal_delta_bytes" ] && rpo_wal_delta_bytes="null"
  notes+=("primary_lsn=$primary_lsn standby_replay_lsn=$standby_replay")
else
  notes+=("PRIMARY_URL/STANDBY_URL not both set (or psql missing): failover steps skipped")
fi

# -------------------------------------------------- restore-timing proxy (RTO)
dump_seconds="null"
restore_seconds="null"
restore_db=""
if [ -n "$PRIMARY_URL" ] && [ ${#PSQL_PRIMARY[@]} -gt 0 ]; then
  restore_db="dr_drill_restore_$(date +%s)"
  dumpfile=$(mktemp /tmp/dr_drill.XXXXXX.dump)
  t0=$(now_epoch)
  pg_dump -Fc "$PRIMARY_URL" > "$dumpfile" 2>/dev/null
  dump_rc=$?
  t1=$(now_epoch)
  if [ $dump_rc -eq 0 ]; then
    createdb -h "$(q "$PRIMARY_URL" "SELECT inet_server_addr()")" 2>/dev/null \
      || q "$PRIMARY_URL" "SELECT 1" >/dev/null
    # Restore into a throwaway database on the same server when we can create one.
    if q "$PRIMARY_URL" "CREATE DATABASE $restore_db" >/dev/null 2>&1; then
      t2=$(now_epoch)
      pg_restore --no-owner --no-privileges -d "${PRIMARY_URL%/*}/$restore_db" "$dumpfile" >/dev/null 2>&1
      t3=$(now_epoch)
      restore_seconds=$((t3 - t2))
      q "$PRIMARY_URL" "DROP DATABASE $restore_db" >/dev/null 2>&1
    fi
    dump_seconds=$((t1 - t0))
  else
    notes+=("pg_dump failed (rc=$dump_rc)")
  fi
  rm -f "$dumpfile"
else
  notes+=("no PRIMARY_URL: dump/restore timing skipped")
fi

finished_at=$(now_iso)

# ------------------------------------------------------------------ verdict
status="partial"
[ "$failover_status" = "performed" ] && status="performed"
rpo_pass="null"
rto_pass="null"
[ "$rpo_lag_seconds" != "null" ] && {
  [ "$rpo_lag_seconds" -le "$RPO_TARGET_SECONDS" ] && rpo_pass=true || rpo_pass=false
}
if [ "$standby_promote_seconds" != "null" ]; then
  [ "$standby_promote_seconds" -le "$RTO_TARGET_SECONDS" ] && rto_pass=true || rto_pass=false
elif [ "$restore_seconds" != "null" ]; then
  [ "$restore_seconds" -le "$RTO_TARGET_SECONDS" ] && rto_pass=true || rto_pass=false
fi

mkdir -p "$(dirname "$OUT")"
notes_json="[]"
if [ ${#notes[@]} -gt 0 ]; then
  notes_json="["
  for i in "${!notes[@]}"; do
    esc=${notes[$i]//\"/\\\"}
    if [ "$i" -gt 0 ]; then notes_json+=", "; fi
    notes_json+="\"$esc\""
  done
  notes_json+="]"
fi
cat > "$OUT" <<JSON
{
  "drill": "primary-az-failure-to-standby",
  "started_at": "$started_at",
  "finished_at": "$finished_at",
  "targets": {
    "rpo_seconds": $RPO_TARGET_SECONDS,
    "rto_seconds": $RTO_TARGET_SECONDS
  },
  "failover": {
    "status": "$failover_status",
    "promote_probe": "$promote_probe",
    "promote_wall_seconds": $standby_promote_seconds
  },
  "rpo": {
    "standby_replay_lag_seconds": $rpo_lag_seconds,
    "wal_delta_bytes": $rpo_wal_delta_bytes,
    "within_target": $rpo_pass
  },
  "rto": {
    "pg_dump_seconds": $dump_seconds,
    "pg_restore_seconds": $restore_seconds,
    "within_target": $rto_pass
  },
  "status": "$status",
  "notes": $notes_json
}
JSON

echo "dr_drill: wrote $OUT (status=$status)"

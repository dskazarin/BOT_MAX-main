#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8082}"
PATIENT_ID="${PATIENT_ID:-PAT-001}"
LOGIN="${LOGIN:-smoke_pat}"
PASSWORD="${PASSWORD:-smoke_pass_$(date +%s)}"
ACC_ID="PACC-SMOKE-$$"
JAR="$(mktemp /tmp/pat_jar.XXXXXX)"

cleanup() {
  sqlite3 bot_max.db "DELETE FROM patient_accounts WHERE id='$ACC_ID';" 2>/dev/null || true
  rm -f "$JAR"
}
trap cleanup EXIT

echo "[1/6] bcrypt hash via /api/patient-auth/dev-hash..."
HASH="$(curl -sS -X POST "$BASE/api/patient-auth/dev-hash" \
  -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASSWORD\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["hash"])')"
if [ -z "$HASH" ] || [ "${HASH#\$2}" = "$HASH" ]; then
  echo "FAIL: no bcrypt hash from dev-hash: $HASH" >&2
  exit 1
fi

echo "[2/6] seed patient_accounts id=$ACC_ID login=$LOGIN"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sqlite3 bot_max.db "INSERT INTO patient_accounts (id, patient_id, login, password_hash, status, created, updated) VALUES ('$ACC_ID', '$PATIENT_ID', '$LOGIN', '$HASH', 'active', '$NOW', '$NOW');"

echo "[3/6] POST /api/patient-auth/login"
curl -sS -c "$JAR" -X POST "$BASE/api/patient-auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}" \
  -w '\nHTTP %{http_code}\n'

echo "[4/6] PUT /api/patient/vitals"
curl -sS -b "$JAR" -X PUT "$BASE/api/patient/vitals" \
  -H 'Content-Type: application/json' \
  -d '{"bp_sys":120,"bp_dia":80,"spo2":98,"temp":36.6}' \
  -w '\nHTTP %{http_code}\n'

echo "[5/6] GET /api/patient/vitals"
curl -sS -b "$JAR" "$BASE/api/patient/vitals" -w '\nHTTP %{http_code}\n'

echo "[6/6] GET /api/patient/vitals/history"
curl -sS -b "$JAR" "$BASE/api/patient/vitals/history" -w '\nHTTP %{http_code}\n'

echo "OK: smoke complete"

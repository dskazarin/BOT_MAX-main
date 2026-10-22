#!/usr/bin/env bash
# smoke_b30_8.sh — B30.8b: self-vitals на ВРЕМЕННОМ пациенте.
#
# Зачем: smoke_b30_7.sh и старые прогоны писали в PAT-001, history копилась.
# Здесь создаём временного пациента PAT-SMOKE8-$$ и аккаунт на него,
# пишем vitals, проверяем GET и history, затем удаляем обоих.
#
# KEEP_ACCOUNT=1 — оставить пациента и аккаунт для разбора.
set -euo pipefail

BASE="${BASE:-http://localhost:8082}"
PATIENT_ID="PAT-SMOKE8-$$"
ACC_ID="PACC-SMOKE8-$$"
LOGIN="smoke_pat8_$$"
PASSWORD="smoke_pass_$(date +%s)"
PAT_JAR="$(mktemp /tmp/pat8_jar.XXXXXX)"

cleanup() {
  if [ -z "${KEEP_ACCOUNT:-}" ]; then
    sqlite3 bot_max.db "DELETE FROM patient_accounts WHERE id='$ACC_ID';" 2>/dev/null || true
    sqlite3 bot_max.db "DELETE FROM patients WHERE id='$PATIENT_ID';" 2>/dev/null || true
    sqlite3 bot_max.db "DELETE FROM vitals_history WHERE patient_id='$PATIENT_ID';" 2>/dev/null || true
    sqlite3 bot_max.db "DELETE FROM vitals WHERE patient_id='$PATIENT_ID';" 2>/dev/null || true
  else
    echo "KEEP_ACCOUNT=1: пациент $PATIENT_ID и аккаунт $ACC_ID оставлены" >&2
  fi
  rm -f "$PAT_JAR"
}
trap cleanup EXIT

echo "[1/6] bcrypt hash via /api/patient-auth/dev-hash..."
HASH="$(curl -sS -X POST "$BASE/api/patient-auth/dev-hash" \
  -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASSWORD\"}" | sed -E 's/.*"hash":"([^"]+)".*/\1/')"
[ -n "$HASH" ] || { echo "FAIL: no bcrypt hash from dev-hash" >&2; exit 1; }

echo "[2/6] seed patient $PATIENT_ID + account $ACC_ID"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sqlite3 bot_max.db "INSERT INTO patients (id, name, created) VALUES ('$PATIENT_ID', 'Smoke Patient 8', '$NOW');"
sqlite3 bot_max.db "INSERT INTO patient_accounts (id, patient_id, login, password_hash, status, created, updated) VALUES ('$ACC_ID', '$PATIENT_ID', '$LOGIN', '$HASH', 'active', '$NOW', '$NOW');"

echo "[3/6] POST /api/patient-auth/login"
LOGIN_RESP="$(curl -sS -c "$PAT_JAR" -X POST "$BASE/api/patient-auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}")"
case "$LOGIN_RESP" in
  *'"accessToken"'*|*'"ok":true'*) echo "OK: login" ;;
  *) echo "FAIL: login: $LOGIN_RESP" >&2; exit 1 ;;
esac

echo "[4/6] PUT /api/patient/vitals (временный пациент)"
PUT_CODE="$(curl -sS -o /tmp/put8.$$ -w '%{http_code}' -b "$PAT_JAR" -X PUT "$BASE/api/patient/vitals" \
  -H 'Content-Type: application/json' \
  -d '{"bp":"120/80","temp":36.6,"spo2":98}')"
[ "$PUT_CODE" = "200" ] || { echo "FAIL: PUT status $PUT_CODE: $(cat /tmp/put8.$$)" >&2; exit 1; }
echo "OK: PUT 200"

echo "[5/6] GET /api/patient/vitals — значения применились"
GET_BODY="$(curl -sS -b "$PAT_JAR" "$BASE/api/patient/vitals")"
echo "$GET_BODY"
case "$GET_BODY" in
  *'"bp":"120/80"'*) echo "OK: bp" ;;
  *) echo "FAIL: bp: $GET_BODY" >&2; exit 1 ;;
esac
case "$GET_BODY" in
  *'"spo2":98'*) : ;;
  *) echo "FAIL: spo2: $GET_BODY" >&2; exit 1 ;;
esac

echo "[6/6] GET /api/patient/vitals/history — запись есть у ЭТОГО пациента"
HIST_BODY="$(curl -sS -b "$PAT_JAR" "$BASE/api/patient/vitals/history?limit=5")"
echo "$HIST_BODY"
case "$HIST_BODY" in
  *'"bp":"120/80"'*) echo "OK: history" ;;
  *) echo "FAIL: history: $HIST_BODY" >&2; exit 1 ;;
esac

echo "OK: B30.8b smoke complete (patient=$PATIENT_ID)"

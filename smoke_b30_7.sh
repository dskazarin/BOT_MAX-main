#!/usr/bin/env bash
set -euo pipefail

BASE="${BASE:-http://localhost:8082}"
PATIENT_ID="${PATIENT_ID:-PAT-001}"
LOGIN="${LOGIN:-smoke_pat7}"
PASSWORD="${PASSWORD:-smoke_pass_$(date +%s)}"
ACC_ID="PACC-SMOKE7-$$"
PAT_JAR="$(mktemp /tmp/pat7_jar.XXXXXX)"
DOC_JAR="$(mktemp /tmp/doc7_jar.XXXXXX)"
DOC_EMAIL="${DOC_EMAIL:-doctor@local}"
DOC_PASSWORD="${DOC_PASSWORD:-doctor@local}"

cleanup() {
  if [ -z "${KEEP_ACCOUNT:-}" ]; then
    sqlite3 bot_max.db "DELETE FROM patient_accounts WHERE id='$ACC_ID';" 2>/dev/null || true
  else
    echo "KEEP_ACCOUNT=1: аккаунт $ACC_ID оставлен" >&2
  fi
  rm -f "$PAT_JAR" "$DOC_JAR"
}
trap cleanup EXIT

echo "[1/7] bcrypt hash via /api/patient-auth/dev-hash..."
HASH="$(curl -sS -X POST "$BASE/api/patient-auth/dev-hash" \
  -H 'Content-Type: application/json' \
  -d "{\"password\":\"$PASSWORD\"}" \
  | python3 -c 'import sys,json; print(json.load(sys.stdin)["hash"])')"
if [ -z "$HASH" ] || [ "${HASH#\$2}" = "$HASH" ]; then
  echo "FAIL: no bcrypt hash from dev-hash: $HASH" >&2
  exit 1
fi

echo "[2/7] seed patient_accounts id=$ACC_ID login=$LOGIN"
NOW="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
sqlite3 bot_max.db "INSERT INTO patient_accounts (id, patient_id, login, password_hash, status, created, updated) VALUES ('$ACC_ID', '$PATIENT_ID', '$LOGIN', '$HASH', 'active', '$NOW', '$NOW');"

echo "[3/7] POST /api/patient-auth/login"
LOGIN_RESP=$(curl -sS -c "$PAT_JAR" -X POST "$BASE/api/patient-auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}" \
  -w '\n%{http_code}')
LOGIN_BODY="${LOGIN_RESP%$'\n'*}"; LOGIN_CODE="${LOGIN_RESP##*$'\n'}"
echo "$LOGIN_BODY"; echo "HTTP $LOGIN_CODE"
[ "$LOGIN_CODE" = "200" ] || { echo "FAIL: login $LOGIN_CODE"; exit 1; }

echo "[4/7] GET /api/patient/profile (ожидаем 200, есть name)"
GET_RESP=$(curl -sS -b "$PAT_JAR" "$BASE/api/patient/profile" -w '\n%{http_code}')
GET_BODY="${GET_RESP%$'\n'*}"; GET_CODE="${GET_RESP##*$'\n'}"
echo "$GET_BODY"; echo "HTTP $GET_CODE"
[ "$GET_CODE" = "200" ] || { echo "FAIL: GET profile $GET_CODE"; exit 1; }
case "$GET_BODY" in
  *'"name":'*) echo "OK: profile has name" ;;
  *) echo "FAIL: no name in GET: $GET_BODY"; exit 1 ;;
esac

echo "[5/7] PUT /api/patient/profile (birthDate/gender/height/weight)"
PUT_BODY='{"birthDate":"1990-01-01","gender":"male","height":180,"weight":75}'
PUT_RESP=$(curl -sS -b "$PAT_JAR" -X PUT "$BASE/api/patient/profile" \
  -H 'Content-Type: application/json' -d "$PUT_BODY" -w '\n%{http_code}')
P_BODY="${PUT_RESP%$'\n'*}"; P_CODE="${PUT_RESP##*$'\n'}"
echo "$P_BODY"; echo "HTTP $P_CODE"
[ "$P_CODE" = "200" ] || { echo "FAIL: PUT profile $P_CODE"; exit 1; }
case "$P_BODY" in
  *'"birthDate":"1990-01-01"'*) : ;;
  *) echo "FAIL: birthDate not echoed: $P_BODY"; exit 1 ;;
esac
case "$P_BODY" in
  *'"ageDisplay":"'*'"'*) : ;;
  *) echo "FAIL: no ageDisplay: $P_BODY"; exit 1 ;;
esac

echo "[6/7] повторный GET — значения сохранились"
GET2=$(curl -sS -b "$PAT_JAR" "$BASE/api/patient/profile")
echo "$GET2"
case "$GET2" in
  *'"birthDate":"1990-01-01"'*'"gender":"male"'*) : ;;
  *) echo "FAIL: not persisted: $GET2"; exit 1 ;;
esac
case "$GET2" in
  *'"height":180'*'"weight":75'*) : ;;
  *) echo "FAIL: height/weight not persisted: $GET2"; exit 1 ;;
esac
echo "OK: profile round-trip persisted"

echo "[7/7] invalid PUT → 400 (gender/birthDate/height)"
for BAD in '{"gender":"xxx"}' '{"birthDate":"2999-01-01"}' '{"height":999}' '{"weight":999}' '{"name":"   "}'; do
  CODE=$(curl -sS -b "$PAT_JAR" -o /dev/null -X PUT "$BASE/api/patient/profile" \
    -H 'Content-Type: application/json' -d "$BAD" -w '%{http_code}')
  if [ "$CODE" != "400" ]; then
    echo "FAIL: expected 400 for $BAD, got $CODE" >&2; exit 1
  fi
  echo "OK 400: $BAD"
done

echo "[bonus] doctor login + GET /api/patients/$PATIENT_ID видит обновления"
DOC_RESP=$(curl -sS -c "$DOC_JAR" -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$DOC_EMAIL\",\"password\":\"$DOC_PASSWORD\"}" \
  -w '\n%{http_code}')
DOC_CODE="${DOC_RESP##*$'\n'}"
if [ "$DOC_CODE" != "200" ]; then
  echo "WARN: doctor login $DOC_CODE — пропускаю doctor-view (не критично для 7c)" >&2
else
  DOC_GET=$(curl -sS -b "$DOC_JAR" "$BASE/api/patients/$PATIENT_ID")
  echo "$DOC_GET"
  case "$DOC_GET" in
    *'"birthDate":"1990-01-01"'*) echo "OK: doctor sees birthDate" ;;
    *) echo "FAIL: doctor does not see birthDate: $DOC_GET" >&2; exit 1 ;;
  esac
  case "$DOC_GET" in
    *'"gender":"male"'*) : ;;
    *) echo "FAIL: doctor does not see gender" >&2; exit 1 ;;
  esac
fi

echo "OK: B30.7 smoke complete"

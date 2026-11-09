#!/usr/bin/env bash
# B30.11 — end-to-end проверка: врач видит и скачивает файл пациента (consent files_read).
# Запуск:  bash scripts/doctor_files_smoke.sh
# Требует: make run (сервер на :8082, dev-сиды DOC-001 / PAT-DEMO).
set -euo pipefail

BASE=${BASE:-http://localhost:8082}
PAT=${PAT:-PAT-DEMO}
DOC_EMAIL=${DOC_EMAIL:-doctor@local}
DOC_PASS=${DOC_PASS:-doctor@local}
PAT_LOGIN=${PAT_LOGIN:-patient@local}
PAT_PASS=${PAT_PASS:-patient@local}

TMP=$(mktemp -d /tmp/botmax-doc-smoke.XXXXXX)
trap 'rm -rf "$TMP"' EXIT

step(){ printf '\n== %s ==\n' "$*"; }
fail(){ echo "FAIL: $*" >&2; exit 1; }
http(){ curl -s -o /dev/null -w '%{http_code}' "$@"; }

step "0. сервер"
code=$(http "$BASE/api/health") || true
[ "$code" = 200 ] || fail "backend не отвечает на $BASE (запусти: make run)"

step "1. логин пациента → cookie botmax_pat_access"
curl -fsS -c "$TMP/pat.jar" -X POST "$BASE/api/patient-auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"login\":\"$PAT_LOGIN\",\"password\":\"$PAT_PASS\"}" > "$TMP/pat_login.json" \
  || fail "patient login"
grep -q 'botmax_pat_access' "$TMP/pat.jar" || fail "cookie botmax_pat_access не выставлена"

step "2. пациент грузит файл (data_base64)"
printf 'doctor-smoke %s' "$(date -Is)" > "$TMP/src.bin"
B64=$(base64 -w0 "$TMP/src.bin")
curl -fsS -b "$TMP/pat.jar" -X POST "$BASE/api/patient/records/files" \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"doctor-smoke.txt\",\"data_base64\":\"$B64\"}" > "$TMP/up.json" \
  || fail "patient upload"
FID=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))["id"])' "$TMP/up.json")
[ -n "$FID" ] || fail "FID пуст"
echo "FID=$FID"

step "3. sanity: пациент видит файл, has_data=1"
curl -fsS -b "$TMP/pat.jar" "$BASE/api/patient/records/files" > "$TMP/list_pat.json"
python3 - "$TMP/list_pat.json" "$FID" <<'PY' || fail "пациент не видит FID/has_data!=1"
import json,sys
d=json.load(open(sys.argv[1])); items=d.get("items",d) if isinstance(d,dict) else d
ok=any(str(i.get("id"))==sys.argv[2] and int(i.get("has_data") or 0)==1 for i in items)
sys.exit(0 if ok else 1)
PY

step "4. логин врача → Bearer accessToken"
DOC_TOKEN=$(curl -fsS -X POST "$BASE/api/auth/login" \
  -H 'Content-Type: application/json' \
  -d "{\"email\":\"$DOC_EMAIL\",\"password\":\"$DOC_PASS\"}" \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["accessToken"])') \
  || fail "doctor login"
[ -n "$DOC_TOKEN" ] || fail "doctor accessToken пуст"
DA=(-H "Authorization: Bearer $DOC_TOKEN")

step "5. НЕГАТИВЫ до consent"
[ "$(http "${DA[@]}" "$BASE/api/patients/$PAT/files")" = 403 ] \
  || fail "list без consent: ждали 403"
[ "$(http "${DA[@]}" "$BASE/api/patients/$PAT/files/$FID/data")" = 403 ] \
  || fail "data без consent: ждали 403"
# patient-cookie без Bearer на doctor-путь → 401 doctor only
[ "$(http -b "$TMP/pat.jar" "$BASE/api/patients/$PAT/files/$FID/data")" = 401 ] \
  || fail "patient-cookie без Bearer: ждали 401"

step "6. consent files_read: request (doctor) → approve (patient)"
CID=$(curl -fsS -X POST "$BASE/api/consents/request" "${DA[@]}" \
  -H 'Content-Type: application/json' \
  -d "{\"patientId\":\"$PAT\",\"scope\":\"files_read\"}" \
  | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])') \
  || fail "consent request"
[ -n "$CID" ] || fail "consent id пуст"
curl -fsS -b "$TMP/pat.jar" -X POST "$BASE/api/consents/$CID/approve" >/dev/null \
  || fail "consent approve"

step "7. ПОЗИТИВ: врач видит файл в списке и качает blob"
curl -fsS "${DA[@]}" "$BASE/api/patients/$PAT/files" > "$TMP/list_doc.json" \
  || fail "doctor list"
python3 - "$TMP/list_doc.json" "$FID" <<'PY' || fail "FID не найден в doctor-list (карточка врача его не покажет)"
import json,sys
d=json.load(open(sys.argv[1])); items=d.get("items",d) if isinstance(d,dict) else d
ok=any(str(i.get("id"))==sys.argv[2] for i in items)
sys.exit(0 if ok else 1)
PY
curl -fsS "${DA[@]}" -o "$TMP/dl.bin" "$BASE/api/patients/$PAT/files/$FID/data" \
  || fail "doctor download"
cmp -s "$TMP/src.bin" "$TMP/dl.bin" || fail "blob не совпал с исходником"
echo "blob OK ($(wc -c < "$TMP/dl.bin") bytes)"

step "8. НЕГАТИВЫ после consent"
c=$(http "${DA[@]}" "$BASE/api/patients/$PAT/files/F-NOPE/data")
[ "$c" = 404 ] || [ "$c" = 403 ] || fail "чужой fid: $c (ждали 403/404)"
curl -fsS -X POST "$BASE/api/consents/$CID/revoke" "${DA[@]}" >/dev/null \
  || fail "consent revoke"
[ "$(http "${DA[@]}" "$BASE/api/patients/$PAT/files/$FID/data")" = 403 ] \
  || fail "после revoke: ждали 403"

echo
echo "PASS: doctor files smoke OK (FID=$FID, consent=$CID)"

#!/usr/bin/env bash
# B30.15 — smoke S3-хранилища: upload → presign → HEAD → delete.
#
# Режимы:
#   1) BOTMAX_STORAGE_S3_ENDPOINT задан  → против готового S3/minio.
#   2) endpoint пуст, есть docker        → поднимает minio/minio, сам создаёт бакет.
#   3) ни endpoint, ни docker            → SKIP (exit 0), инфра недоступна.
#
# Ограничения: только http:// (TLS не настроен); --network host требует Linux/Codespaces.
#
# Запуск:
#   bash scripts/storage_s3_smoke.sh
#   BOTMAX_STORAGE_S3_ENDPOINT=127.0.0.1:9000 BOTMAX_STORAGE_S3_BUCKET=botmax-smoke \
#     BOTMAX_STORAGE_S3_ACCESS_KEY=minioadmin BOTMAX_STORAGE_S3_SECRET_KEY=minioadmin \
#     bash scripts/storage_s3_smoke.sh
set -euo pipefail

EP=${BOTMAX_STORAGE_S3_ENDPOINT:-}
BUCKET=${BOTMAX_STORAGE_S3_BUCKET:-botmax-smoke}
AK=${BOTMAX_STORAGE_S3_ACCESS_KEY:-minioadmin}
SK=${BOTMAX_STORAGE_S3_SECRET_KEY:-minioadmin}

TMP=$(mktemp -d /tmp/botmax-s3-smoke.XXXXXX)
CONTAINER="botmax-minio-smoke-$$"
STARTED_CONTAINER=0

cleanup() {
  if [ "$STARTED_CONTAINER" = 1 ]; then
    docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

step(){ printf '\n== %s ==\n' "$*"; }
fail(){ echo "FAIL: $*" >&2; exit 1; }
skip(){ echo "SKIP: $*"; exit 0; }

# ---- 0. выбрать режим ----
step "0. инфраструктура"
if [ -n "$EP" ]; then
  echo "endpoint=$EP (внешний)"
elif command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  echo "docker доступен → поднимаю minio"
  docker run -d --rm --name "$CONTAINER" \
    -p 127.0.0.1:9000:9000 \
    -e MINIO_ROOT_USER="$AK" -e MINIO_ROOT_PASSWORD="$SK" \
    minio/minio:latest server /data >/dev/null \
    || fail "docker run minio"
  STARTED_CONTAINER=1
  EP=127.0.0.1:9000
  for i in $(seq 1 30); do
    curl -fsS "http://$EP/minio/health/live" >/dev/null 2>&1 && break
    sleep 1
  done
  curl -fsS "http://$EP/minio/health/live" >/dev/null 2>&1 \
    || fail "minio не поднялся на $EP"
  echo "minio up: $EP"
else
  skip "infra unavailable (нет BOTMAX_STORAGE_S3_ENDPOINT и нет рабочего docker)"
fi

# ---- 1. endpoint / бакет ----
step "1. создаю бакет $BUCKET"
# Используем mc внутри контейнера-аналога, чтобы не требовать установленного mc.
docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest mb -p "myminio/$BUCKET" >/dev/null 2>&1 || true
docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest stat "myminio/$BUCKET" >/dev/null 2>&1 \
  || fail "бакет $BUCKET недоступен (endpoint=$EP)"

# ---- 2. upload ----
step "2. upload (PUTObject)"
KEY_PID=PAT-SMOKE
KEY_ID="smoke-$(date +%s)"
OBJ="patients/$KEY_PID/$KEY_ID"
printf 's3-smoke %s' "$(date -Is)" > "$TMP/src.bin"
docker run --rm --network host -v "$TMP:/t:ro" \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest cp "/t/src.bin" "myminio/$BUCKET/$OBJ" >/dev/null \
  || fail "upload $OBJ"
echo "uploaded: $OBJ"

# ---- 3. HEAD (stat) ----
step "3. HEAD (stat)"
docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest stat "myminio/$BUCKET/$OBJ" >/dev/null \
  || fail "stat $OBJ (объект не найден)"

# ---- 4. presign (GET) ----
step "4. presign (GET, TTL 60s)"
URL=$(docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest share download --expire 60s "myminio/$BUCKET/$OBJ" 2>/dev/null)
[ -n "$URL" ] || fail "presign вернул пустой URL"
echo "url=${URL%%\?*}?..."

# ---- 5. delete + verify ----
step "5. delete + verify отсутствия"
docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest rm "myminio/$BUCKET/$OBJ" >/dev/null \
  || fail "delete $OBJ"
if docker run --rm --network host \
  -e MC_HOST_myminio="http://$AK:$SK@$EP" \
  minio/mc:latest stat "myminio/$BUCKET/$OBJ" >/dev/null 2>&1; then
  fail "объект всё ещё существует после delete"
fi

step "OK"
echo "B30.15 smoke: upload → presign → HEAD → delete — пройдено"

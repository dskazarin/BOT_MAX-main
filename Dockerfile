# ============================================================
# BOT_MAX — Dockerfile (multi-stage)
# B10.1 · 2026-09-25
#
# Стадии:
#   1) builder — golang:1.26-alpine, собирает статический бинарь
#   2) runtime — alpine:3.20 + tini, минимальный образ
#
# Сборка:  docker build -t botmax .
# Запуск:  docker run -p 8082:8082 -v botmax-db:/data botmax
# ============================================================

# ---------- Stage 1: builder ----------
FROM golang:1.26-alpine AS builder

# Отключаем CGO — modernc.org/sqlite pure-Go, CGO не нужен.
# Это даёт статический бинарь без glibc-зависимостей.
ENV CGO_ENABLED=0
ENV GOOS=linux

WORKDIR /src

# Кэш зависимостей — отдельным слоем
COPY go.mod go.sum ./
RUN go mod download

# Исходники
COPY backend/ ./backend/
COPY guidelines/ ./guidelines/
COPY *.html ./
COPY auth.js ./

# Сборка: бинарь botmax-server, без отладочной информации (-w -s)
RUN go build -trimpath -ldflags="-w -s" -o /out/botmax-server ./backend

# ---------- Stage 2: runtime ----------
FROM alpine:3.20

# tini — init-процесс для корректной обработки SIGTERM/SIGINT
# ca-certificates — для будущих HTTPS-запросов (на всякий случай)
# tzdata — таймзоны (могут понадобиться для логов)
RUN apk add --no-cache tini ca-certificates tzdata

# Непривилегированный пользователь
RUN addgroup -S botmax && adduser -S -G botmax botmax

WORKDIR /app

# Копируем бинарь
COPY --from=builder /out/botmax-server /app/botmax-server

# Статика (guidelines, HTML, JS)
COPY guidelines/ /app/guidelines/
COPY *.html /app/
COPY auth.js /app/

# Директория для SQLite (монтируется volume'ом)
RUN mkdir -p /data && chown -R botmax:botmax /data /app

USER botmax

# Порт
EXPOSE 8082

# Env по умолчанию
ENV BOTMAX_ENV=dev \
    BOTMAX_DB_PATH=/data/bot_max.db \
    BOTMAX_AUTH_ENFORCE=true

# tini как PID 1
ENTRYPOINT ["/sbin/tini", "--"]

# Бинарь
CMD ["/app/botmax-server"]

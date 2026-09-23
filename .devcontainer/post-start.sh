#!/bin/bash
# post-start.sh — автозапуск BOT_MAX (Go + SQLite)
# Обновлено: 2026-09-23, шаг 4 — Go-сервер вместо Python http.server

cd /workspaces/BOT_MAX-main

echo "=========================================="
echo "🌐 BOT_MAX autostart (Go backend)"
echo "=========================================="

# 1. Убить всех возможных захватчиков порта
echo "🧹 Освобождение порта 8082..."
pkill -f "python3 -m http.server" 2>/dev/null || true
pkill -f "go run ./backend" 2>/dev/null || true
pkill -f "/tmp/botmax_server" 2>/dev/null || true
fuser -k 8082/tcp 2>/dev/null || true
sleep 2

# 2. Проверить, что порт свободен
if lsof -i :8082 > /dev/null 2>&1; then
    echo "⚠️ Порт всё ещё занят, повторная очистка..."
    fuser -k 8082/tcp 2>/dev/null || true
    sleep 2
fi

# 3. Собрать свежий бинарник
echo "🔨 Сборка Go-сервера..."
if ! go build -o /tmp/botmax_server ./backend 2> /tmp/botmax_build.log; then
    echo "❌ Ошибка сборки:"
    cat /tmp/botmax_build.log
    exit 1
fi
echo "✅ Бинарник собран: /tmp/botmax_server"

# 4. Запустить сервер
echo "🚀 Запуск сервера на порту 8082..."
nohup /tmp/botmax_server > /tmp/botmax.log 2>&1 &
sleep 5

# 5. Проверить health
if curl -s http://localhost:8082/api/health | grep -q '"status":"ok"'; then
    echo ""
    echo "=========================================="
    echo "✅ BOT_MAX УСПЕШНО ЗАПУЩЕН"
    echo "=========================================="
    echo "🌐 http://localhost:8082"
    echo "📋 Проверить: make status"
    echo "📋 Остановить: make stop"
    echo "📋 Логи: tail -f /tmp/botmax.log"
    echo "=========================================="
else
    echo "❌ Сервер не отвечает. Логи:"
    cat /tmp/botmax.log
    exit 1
fi

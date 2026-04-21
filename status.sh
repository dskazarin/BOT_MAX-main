#!/bin/bash

echo "📊 Статус BOT_MAX:"
if pgrep -f "botmax_server" > /dev/null; then
    echo "✅ Сервер запущен (PID: $(pgrep -f botmax_server))"
    echo "🌐 http://localhost:8082"
else
    echo "❌ Сервер не запущен"
fi

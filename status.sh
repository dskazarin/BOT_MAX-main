#!/bin/bash
if pgrep -f "python3 -m http.server" > /dev/null; then
    echo "✅ Сервер запущен (PID: $(pgrep -f 'python3 -m http.server'))"
    echo "🌐 http://localhost:8082"
else
    echo "❌ Сервер не запущен"
fi

#!/bin/bash
cd /workspaces/BOT_MAX-main
echo "🚀 Запуск сервера..."
python3 -m http.server 8082 > server.log 2>&1 &
sleep 1
echo "✅ Сервер запущен на http://localhost:8082"

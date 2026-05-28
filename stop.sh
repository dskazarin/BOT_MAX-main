#!/bin/bash
echo "🛑 Остановка сервера BOT_MAX..."
pkill -f "python3 -m http.server"
sudo fuser -k 8082/tcp 2>/dev/null
echo "✅ Сервер остановлен"

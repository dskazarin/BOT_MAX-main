#!/bin/bash
echo "🛑 Остановка сервера..."
pkill -f "python3 -m http.server" 2>/dev/null
sudo fuser -k 8082/tcp 2>/dev/null
echo "✅ Сервер остановлен"

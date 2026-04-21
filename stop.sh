#!/bin/bash

echo "🛑 Остановка BOT_MAX..."
pkill -f "botmax_server" 2>/dev/null
lsof -ti:8082 | xargs kill -9 2>/dev/null
echo "✅ Сервер остановлен"

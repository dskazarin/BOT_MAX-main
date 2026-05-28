#!/bin/bash
pkill -f "start_cabinet\|go run" 2>/dev/null
sudo fuser -k 8082/tcp 2>/dev/null
echo "✅ Сервер остановлен"

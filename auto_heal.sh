#!/bin/bash
cd /workspaces/BOT_MAX

# Компиляция бинарника при первом запуске
if [ ! -f "main_fixed.bin" ]; then
    go build -o main_fixed.bin main_fixed.go
fi

while true; do
    COUNT=$(pgrep -f "main_fixed.bin" | wc -l)
    
    if [ "$COUNT" -gt 1 ]; then
        echo "$(date): ⚠️ Обнаружено $COUNT экземпляров, перезапуск..." >> heal.log
        pkill -9 -f "main_fixed.bin" 2>/dev/null
        sudo fuser -k 8082/tcp 2>/dev/null
        sleep 2
        ./main_fixed.bin &
        sleep 3
        echo "$(date): ✅ Запущен один экземпляр" >> heal.log
    elif [ "$COUNT" -eq 0 ]; then
        echo "$(date): 🔄 Сервер не запущен, запускаю..." >> heal.log
        ./main_fixed.bin &
        sleep 3
        echo "$(date): ✅ Сервер запущен" >> heal.log
    elif ! curl -s http://localhost:8082/health > /dev/null 2>&1; then
        echo "$(date): 🔄 Health check failed, перезапуск..." >> heal.log
        pkill -9 -f "main_fixed.bin" 2>/dev/null
        sudo fuser -k 8082/tcp 2>/dev/null
        sleep 2
        ./main_fixed.bin &
        sleep 3
        echo "$(date): ✅ Сервер восстановлен" >> heal.log
    fi
    
    sleep 10
done

#!/bin/bash

echo "=========================================="
echo "🩺 BOT_MAX - Личный кабинет пациента"
echo "=========================================="

cd /workspaces/BOT_MAX-main

# Проверяем наличие файлов
if [ ! -f "patient_cabinet_full.html" ]; then
    echo "❌ Ошибка: patient_cabinet_full.html не найден!"
    exit 1
fi

# Останавливаем старый сервер если есть
pkill -f "python3 -m http.server" 2>/dev/null
sudo fuser -k 8082/tcp 2>/dev/null

# Запускаем сервер
echo "🚀 Запуск сервера на порту 8082..."
nohup python3 -m http.server 8082 > server.log 2>&1 &

sleep 2

# Проверяем запуск
if pgrep -f "python3 -m http.server" > /dev/null; then
    echo "✅ Сервер запущен успешно!"
    echo ""
    echo "🌐 Откройте в браузере:"
    echo "   http://localhost:8082"
    echo ""
    echo "📝 Лог сервера: tail -f server.log"
    echo "🛑 Остановить: pkill -f 'python3 -m http.server'"
    
    # Пытаемся открыть браузер
    if command -v xdg-open > /dev/null; then
        xdg-open http://localhost:8082
    elif command -v open > /dev/null; then
        open http://localhost:8082
    fi
else
    echo "❌ Ошибка запуска сервера!"
    echo "Проверьте лог: cat server.log"
    exit 1
fi

echo "=========================================="

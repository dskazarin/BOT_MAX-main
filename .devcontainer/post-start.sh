#!/bin/bash

echo "=========================================="
echo "🌐 Автозапуск личного кабинета BOT_MAX"
echo "=========================================="

cd /workspaces/BOT_MAX-main

# Запускаем сервер
nohup python3 -m http.server 8082 > server.log 2>&1 &

sleep 2

if pgrep -f "python3 -m http.server" > /dev/null; then
    echo "✅ Личный кабинет запущен на http://localhost:8082"
    
    # Для Codespaces показываем специальный URL
    if [ -n "$CODESPACE_NAME" ]; then
        echo "🌐 Codespaces URL: https://${CODESPACE_NAME}-8082.preview.app.github.dev"
    fi
else
    echo "❌ Ошибка запуска"
fi

echo "=========================================="

#!/bin/bash

echo "=========================================="
echo "🌐 Автозапуск личного кабинета BOT_MAX"
echo "=========================================="

cd /workspaces/BOT_MAX-main

# Функция для проверки файлов
check_files() {
    if [ ! -f "patient_cabinet_full.html" ]; then
        echo "❌ Ошибка: patient_cabinet_full.html не найден!"
        return 1
    fi
    echo "✅ Файлы личного кабинета найдены"
    return 0
}

# Очистка порта
clean_port() {
    echo "🧹 Очистка порта 8082..."
    pkill -f "python3 -m http.server" 2>/dev/null
    sudo fuser -k 8082/tcp 2>/dev/null
    sleep 1
}

# Запуск сервера
start_server() {
    echo "🚀 Запуск Python сервера на порту 8082..."
    nohup python3 -m http.server 8082 > server.log 2>&1 &
    local server_pid=$!
    sleep 2
    
    if ps -p $server_pid > /dev/null 2>&1 && pgrep -f "python3 -m http.server" > /dev/null; then
        echo "✅ Сервер запущен (PID: $server_pid)"
        return 0
    else
        echo "❌ Ошибка запуска сервера"
        if [ -f "server.log" ]; then
            echo "Последние строки лога:"
            tail -5 server.log
        fi
        return 1
    fi
}

# Проверка доступности
test_server() {
    echo "🔍 Проверка доступности..."
    sleep 1
    if curl -s -o /dev/null -w "%{http_code}" http://localhost:8082 | grep -q "200"; then
        echo "✅ Сервер отвечает на запросы"
        return 0
    else
        echo "⚠️  Сервер не отвечает (возможно запускается...)"
        return 1
    fi
}

# Основная логика
main() {
    echo ""
    check_files || exit 1
    clean_port
    start_server || exit 1
    test_server
    
    echo ""
    echo "=========================================="
    echo "✅ ЛИЧНЫЙ КАБИНЕТ УСПЕШНО ЗАПУЩЕН"
    echo "=========================================="
    echo "🌐 Локальный доступ: http://localhost:8082"
    echo ""
    
    # Показываем URL для Codespaces
    if [ -n "$CODESPACE_NAME" ]; then
        echo "🌐 GitHub Codespaces URL:"
        echo "   https://${CODESPACE_NAME}-8082.preview.app.github.dev"
        echo ""
    fi
    
    echo "📋 Полезные команды:"
    echo "   Проверить статус: ./status.sh"
    echo "   Остановить: ./stop.sh"
    echo "   Посмотреть логи: tail -f server.log"
    echo "=========================================="
}

main

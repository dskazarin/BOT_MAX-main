#!/bin/bash

echo "═══════════════════════════════════════════════════════════════"
echo "🚀 ЗАПУСК BOT_MAX - Медицинская платформа"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Остановка старых процессов
pkill -f "botmax_server" 2>/dev/null
pkill -f "main_fixed" 2>/dev/null
lsof -ti:8082 | xargs kill -9 2>/dev/null

# Проверка наличия зависимостей
if [ ! -f "go.mod" ]; then
    echo "📦 Инициализация Go модуля..."
    go mod init botmax
    go get github.com/gorilla/mux
    go get github.com/gorilla/websocket
fi

# Скачивание зависимостей
go mod tidy

# Запуск правильного файла
echo "🚀 Запуск сервера с русским интерфейсом..."
nohup go run botmax_server.go > server.log 2>&1 &

sleep 3

# Проверка
if curl -s http://localhost:8082/health > /dev/null; then
    echo ""
    echo "✅ СЕРВЕР УСПЕШНО ЗАПУЩЕН!"
    echo ""
    echo "🌐 Откройте в браузере: http://localhost:8082"
    echo ""
    echo "🔐 Доступ к админ-панели:"
    echo "   Логин: admin"
    echo "   Пароль: admin123"
    echo ""
else
    echo "❌ Ошибка запуска. Проверьте логи: tail -f server.log"
fi

echo ""
echo "═══════════════════════════════════════════════════════════════"

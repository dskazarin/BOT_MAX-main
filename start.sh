#!/bin/bash
cd /workspaces/BOT_MAX

# Установка зависимостей
echo "📦 Установка зависимостей..."
go mod tidy 2>/dev/null
go get github.com/mattn/go-sqlite3 2>/dev/null

# Остановка старых процессов
echo "🛑 Остановка старых процессов..."
pkill -9 -f "main_fixed" 2>/dev/null
pkill -9 -f "main_fixed.bin" 2>/dev/null
sudo fuser -k 8082/tcp 2>/dev/null
sleep 2

# Компиляция бинарника
echo "🔧 Компиляция сервера..."
go build -o main_fixed.bin main_fixed.go

# Запуск бинарника (только 1 процесс!)
echo "🚀 Запуск защищенного сервера..."
./main_fixed.bin &
sleep 3

# Проверка
if curl -s http://localhost:8082/health > /dev/null 2>&1; then
    echo "✅ BOT_MAX Secure Server started"
    echo "✅ Server is healthy"
    echo "✅ Процессов: $(pgrep -f "main_fixed.bin" | wc -l)"
else
    echo "❌ Server failed to start"
fi

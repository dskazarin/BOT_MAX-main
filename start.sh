#!/bin/bash

echo "═══════════════════════════════════════════════════════════════"
echo "🚀 ЗАПУСК BOT_MAX - Медицинская платформа"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Остановка старых процессов
pkill -f "final_patient_cabinet" 2>/dev/null
pkill -f "botmax" 2>/dev/null
lsof -ti:8082 | xargs kill -9 2>/dev/null
sleep 2

# Убеждаемся что директория uploads существует
mkdir -p uploads

# Запуск правильного файла
echo "🚀 Запуск обновленной версии..."
go run final_patient_cabinet.go &

sleep 3

# Проверка
if curl -s http://localhost:8082/ > /dev/null; then
    echo ""
    echo "✅ СЕРВЕР ЗАПУЩЕН!"
    echo ""
    echo "🌐 Откройте: http://localhost:8082"
    echo "🔑 Вход: patient@demo.com / 123"
    echo ""
else
    echo "❌ Ошибка запуска. Проверьте: tail -f server.log"
fi

echo "═══════════════════════════════════════════════════════════════"

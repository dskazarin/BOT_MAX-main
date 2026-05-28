#!/bin/bash

echo "=========================================="
echo "📦 Настройка окружения BOT_MAX"
echo "=========================================="

cd /workspaces/BOT_MAX-main

# Проверяем наличие Python
if command -v python3 &> /dev/null; then
    echo "✅ Python $(python3 --version) установлен"
else
    echo "❌ Python не найден"
    exit 1
fi

# Создаем index.html если отсутствует
if [ ! -f "index.html" ]; then
    echo "📄 Создаю index.html..."
    cat > index.html << 'HTML'
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="0; url=patient_cabinet_full.html">
    <title>Личный кабинет BOT_MAX</title>
</head>
<body>
    <p>Загрузка личного кабинета...</p>
</body>
</html>
HTML
    echo "✅ index.html создан"
fi

# Создаем управляющие скрипты если их нет
for script in start.sh stop.sh status.sh; do
    if [ ! -f "$script" ]; then
        echo "⚠️  $script не найден, создаю..."
    fi
done

echo ""
echo "✅ Готово! Сервер запустится автоматически"
echo "=========================================="

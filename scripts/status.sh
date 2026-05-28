#!/bin/bash
if curl -s http://localhost:8082/ > /dev/null 2>&1; then
    echo "✅ Сервер работает на http://localhost:8082"
    echo "📊 Страница: Личный кабинет пациента"
else
    echo "❌ Сервер не запущен"
fi

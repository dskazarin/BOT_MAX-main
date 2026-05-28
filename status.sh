#!/bin/bash
if pgrep -f "python3 -m http.server" > /dev/null; then
    echo "✅ Сервер запущен (PID: $(pgrep -f 'python3 -m http.server'))"
    echo "🌐 http://localhost:8082"
    echo ""
    echo "📊 Файлы личного кабинета:"
    ls -lh patient_cabinet*.html 2>/dev/null || echo "   HTML файлы не найдены"
else
    echo "❌ Сервер не запущен"
    echo "Запустите: ./start.sh"
fi

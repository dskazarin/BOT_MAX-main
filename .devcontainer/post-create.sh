#!/bin/bash

echo "=========================================="
echo "🚀 Настройка BOT_MAX Patient Cabinet"
echo "=========================================="

cd /workspaces/BOT_MAX-main

# Создаем index.html если его нет
if [ ! -f "index.html" ]; then
    cat > index.html << 'HTMLEOF'
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <meta http-equiv="refresh" content="0; url=patient_cabinet_full.html">
    <title>Личный кабинет BOT_MAX</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            height: 100vh;
            background: linear-gradient(135deg, #667eea, #764ba2);
            margin: 0;
        }
        .loader {
            text-align: center;
            color: white;
        }
        .spinner {
            border: 4px solid rgba(255,255,255,0.3);
            border-top: 4px solid white;
            border-radius: 50%;
            width: 40px;
            height: 40px;
            animation: spin 1s linear infinite;
            margin: 20px auto;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="loader">
        <div class="spinner"></div>
        <p>Загрузка личного кабинета...</p>
    </div>
</body>
</html>
HTMLEOF
    echo "✅ index.html создан"
fi

echo "✅ Готово к запуску!"

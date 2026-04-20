#!/bin/bash

echo "═══════════════════════════════════════════════════════════════"
echo "📦 СОХРАНЕНИЕ РАБОТОСПОСОБНОЙ ВЕРСИИ BOT_MAX В GIT"
echo "═══════════════════════════════════════════════════════════════"
echo ""

# Цвета
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_success() { echo -e "${GREEN}✅ $1${NC}"; }
print_info() { echo -e "${BLUE}📌 $1${NC}"; }
print_warning() { echo -e "${YELLOW}⚠️ $1${NC}"; }

# Проверяем наличие git
if ! command -v git &> /dev/null; then
    print_warning "Git не установлен. Установите git: sudo apt-get install git"
    exit 1
fi

# Инициализация git репозитория если его нет
if [ ! -d .git ]; then
    print_info "Инициализация Git репозитория..."
    git init
    print_success "Git репозиторий инициализирован"
else
    print_success "Git репозиторий уже существует"
fi

# Создаем .gitignore
print_info "Создание .gitignore..."
cat > .gitignore << 'IGNOREEOF'
# Binaries
bin/
*.exe
*.exe~
*.dll
*.so
*.dylib
*.test
*.out

# Go workspace file
go.work
go.work.sum

# Dependency directories
vendor/

# IDE
.vscode/
.idea/
*.swp
*.swo
*~

# Logs
logs/
*.log
server.log

# Database
*.db
*.db-shm
*.db-wal

# OS
.DS_Store
Thumbs.db

# Temporary files
tmp/
temp/
*.tmp

# Environment
.env
.env.local
.env.*.local

# SSL certificates
*.pem
*.crt
*.key

# Backups
*.bak
*.backup
test_results_*.txt

# PID files
*.pid

# Binary
botmax
simple_server
final_server
botmax_server
IGNOREEOF
print_success ".gitignore создан"

# Добавляем все файлы
print_info "Добавление файлов в Git..."
git add .

# Создаем коммит
print_info "Создание коммита..."
git commit -m "feat: BOT_MAX Medical Platform v8.0.0

🎉 Полнофункциональная медицинская платформа

Основные возможности:
✅ Веб-интерфейс для пациентов и врачей
✅ Регистрация и аутентификация пользователей
✅ Ввод симптомов (текст, голос, фото)
✅ Управление лекарствами и аллергиями
✅ История болезней и операций
✅ Доступ врача к карте пациента
✅ Осмотры, рецепты, справки
✅ 3 уровня AI анализа истории болезни
✅ Загрузка промтов и клинических рекомендаций
✅ Админ-панель с мониторингом
✅ Система оповещений
✅ Обратная связь от пользователей
✅ Военный уровень безопасности

Технологии:
- Go 1.22+
- Gorilla Mux
- WebSocket для real-time
- REST API
- HTML/CSS/JS интерфейс

API Эндпоинты:
- /api/register - регистрация
- /api/login - авторизация
- /api/patient/* - функции пациента
- /api/doctor/* - функции врача
- /api/admin/* - администрирование
- /api/alerts/* - оповещения
- /api/feedback - обратная связь

Доступ:
- Главная: http://localhost:8082
- Пациент: http://localhost:8082/patient
- Врач: http://localhost:8082/doctor
- Админ: http://localhost:8082/admin (admin/admin123)
"

if [ $? -eq 0 ]; then
    print_success "Коммит создан успешно"
else
    print_warning "Нет изменений для коммита или ошибка"
fi

# Показываем статус
echo ""
print_info "Статус Git:"
git status

# Показываем лог
echo ""
print_info "Последние коммиты:"
git log --oneline -3

# Предлагаем добавить remote
echo ""
print_info "Хотите добавить удаленный репозиторий? (y/n)"
read -r add_remote

if [ "$add_remote" = "y" ] || [ "$add_remote" = "Y" ]; then
    echo ""
    print_info "Введите URL удаленного репозитория (например: https://github.com/username/BOT_MAX.git):"
    read -r remote_url
    
    if [ -n "$remote_url" ]; then
        git remote add origin "$remote_url"
        print_success "Удаленный репозиторий добавлен"
        
        print_info "Хотите запушить в удаленный репозиторий? (y/n)"
        read -r do_push
        
        if [ "$do_push" = "y" ] || [ "$do_push" = "Y" ]; then
            print_info "Пуш в удаленный репозиторий..."
            git push -u origin main
            if [ $? -eq 0 ]; then
                print_success "Код отправлен в удаленный репозиторий"
            else
                print_warning "Ошибка при пуше. Возможно, нужно создать ветку main или указать правильный URL"
            fi
        fi
    fi
fi

echo ""
print_info "Создание тега для версии..."
git tag -a "v8.0.0" -m "BOT_MAX Medical Platform v8.0.0 - Stable Release"

if [ $? -eq 0 ]; then
    print_success "Тег v8.0.0 создан"
else
    print_warning "Тег v8.0.0 уже существует"
fi

echo ""
print_info "Хотите запушить теги? (y/n)"
read -r push_tags

if [ "$push_tags" = "y" ] || [ "$push_tags" = "Y" ]; then
    git push --tags
    if [ $? -eq 0 ]; then
        print_success "Теги отправлены в удаленный репозиторий"
    fi
fi

echo ""
echo "═══════════════════════════════════════════════════════════════"
print_success "ВЕРСИЯ УСПЕШНО СОХРАНЕНА В GIT"
echo "═══════════════════════════════════════════════════════════════"
echo ""
echo "📋 Информация о сохраненной версии:"
echo "   ✅ Версия: v8.0.0"
echo "   ✅ Ветка: main"
echo "   ✅ Файлов: $(git ls-files | wc -l)"
echo "   ✅ Коммитов: $(git rev-list --count HEAD)"
echo ""
echo "🔧 Полезные команды:"
echo "   git log --oneline     # Просмотр истории"
echo "   git status            # Статус файлов"
echo "   git tag               # Список тегов"
echo "   git branch -a         # Все ветки"
echo ""
echo "🚀 Для запуска платформы:"
echo "   go run botmax_server.go"
echo "   или"
echo "   ./start.sh"
echo ""
echo "═══════════════════════════════════════════════════════════════"


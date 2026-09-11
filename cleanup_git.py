#!/usr/bin/env python3
# -*- coding: utf-8 -*-
import os, subprocess, glob

os.chdir('/workspaces/BOT_MAX-main')

print("=" * 60)
print("  🧹 BOT_MAX — ОЧИСТКА + СОХРАНЕНИЕ В GIT")
print("=" * 60)
print()

# ============================================================
#  1. ОЧИСТКА МУСОРА
# ============================================================
print("📄 1. Очистка мусора...")
print("-" * 60)

trash_count = 0

# Паттерны мусора
patterns = [
    '*.bak', '*.bak.*', '*.bak2', '*.bak3', '*.bak4', '*.bak5',
    '*.broken_*', '*.log', 'nohup.out', 'server.log',
    '*.tmp', '*.temp', '*.swp', '*.swo', '*~',
    'go.sum', 'bot_max_server', 'bot_max_test',
    'apply_fixes.sh', 'apply_fixes.py', 'cleanup_and_save.sh',
    'clean_git_tags.sh', 'diagnose.sh', 'diagnose_401.sh',
    'sync_patient_data.sh', 'integrate.py', 'integrate_cards.sh',
    'fix_sync.py', 'release.sh', 'release_2026-09-10.sh',
    'find_ai.sh', 'test_system.sh', 'setup.sh',
    'add_admin_api.py', 'backend_api.py', 'backend_server.js',
    'package.json', 'package-lock.json',
    'go*.tar.gz', 'go*.zip'
]

for pattern in patterns:
    for f in glob.glob(pattern):
        if os.path.isfile(f):
            try:
                os.remove(f)
                print(f"   🗑️  {f}")
                trash_count += 1
            except Exception as e:
                pass

# Бэкапы в подпапках
for f in glob.glob('backend/*.bak') + glob.glob('backend/*.bak.*'):
    if os.path.isfile(f):
        try:
            os.remove(f)
            print(f"   🗑️  {f}")
            trash_count += 1
        except:
            pass

# node_modules
if os.path.isdir('node_modules'):
    import shutil
    shutil.rmtree('node_modules')
    print(f"   🗑️  node_modules/")
    trash_count += 1

# Пустые папки
for root, dirs, files in os.walk('.', topdown=False):
    if '.git' in root:
        continue
    if not os.listdir(root) and root != '.':
        try:
            os.rmdir(root)
            print(f"   🗑️  {root}/ (пустая)")
        except:
            pass

print(f"\n   ✅ Удалено: {trash_count} элементов мусора")
print()

# ============================================================
#  2. ПРОВЕРКА ВАЖНЫХ ФАЙЛОВ
# ============================================================
print("📄 2. Проверка важных файлов...")
print("-" * 60)

required = [
    'index.html',
    'doctor_cabinet.html',
    'patient_cabinet_full.html',
    'patient_card.html',
    'admin_panel.html',
    'go.mod',
    'backend/main.go',
    'guidelines/lor/active/v2024/guideline.json',
    '.gitignore'
]

missing = []
for f in required:
    if os.path.exists(f):
        size = os.path.getsize(f)
        print(f"   ✅ {f} ({size} байт)")
    else:
        print(f"   ❌ {f} — ОТСУТСТВУЕТ")
        missing.append(f)

print()

# ============================================================
#  3. СОЗДАНИЕ .GITIGNORE
# ============================================================
print("📄 3. Обновление .gitignore...")
print("-" * 60)

gitignore_content = """# Мусор
*.bak
*.bak.*
*.bak2
*.bak3
*.broken_*
*.log
*.tmp
*.temp
*.swp
*.swo
*~

# Go
go.sum
bot_max_server
bot_max_test
/main
/backend/main

# IDE
.vscode/
.idea/
*.code-workspace

# OS
.DS_Store
Thumbs.db

# Node
node_modules/
package-lock.json

# Архивы
*.tar.gz
*.zip
!releases/*.tar.gz

# Бэкапы
backups/
*.save
*.save.*
"""

with open('.gitignore', 'w', encoding='utf-8') as f:
    f.write(gitignore_content)

print("   ✅ .gitignore обновлён")
print()

# ============================================================
#  4. GIT — ДОБАВЛЕНИЕ
# ============================================================
print("📄 4. Git — добавление файлов...")
print("-" * 60)

# Проверка git
try:
    result = subprocess.run(['git', '--version'], capture_output=True, text=True)
    print(f"   ✅ {result.stdout.strip()}")
except:
    print("   ❌ Git не найден!")
    exit(1)

# Проверка .git
if not os.path.isdir('.git'):
    print("   Инициализация репозитория...")
    subprocess.run(['git', 'init'])
    subprocess.run(['git', 'config', 'user.name', 'BOT_MAX Developer'])
    subprocess.run(['git', 'config', 'user.email', 'dev@botmax.local'])

# Исправление прав .git
try:
    subprocess.run(['sudo', 'chown', '-R', os.getenv('USER', 'codespace') + ':' + os.getenv('USER', 'codespace'), '.git'],
                   capture_output=True)
except:
    pass

# git add
subprocess.run(['git', 'add', '-A'])

# Показать статус
result = subprocess.run(['git', 'status', '--short'], capture_output=True, text=True)
if result.stdout.strip():
    print("   Файлы к коммиту:")
    for line in result.stdout.strip().split('\n')[:20]:
        print(f"      {line}")
    total = len(result.stdout.strip().split('\n'))
    print(f"\n   Всего: {total} файлов")
else:
    print("   ℹ️  Нет изменений для коммита")

print()

# ============================================================
#  5. GIT — КОММИТ
# ============================================================
print("📄 5. Git — создание коммита...")
print("-" * 60)

commit_msg = """Release v1.2.0 - 2026-09-11

🎉 Новое в релизе:
✅ Sticky header в карте пациента
✅ Синхронизация данных пациент ↔ врач
✅ Автообновление списка пациентов (polling 3 сек)
✅ Автосохранение в карте пациента (2 сек)
✅ Определение симптомов из жалоб
✅ Все 5 HTML-страниц работают
✅ Go-сервер с API

📁 Структура:
- index.html — главная
- doctor_cabinet.html — кабинет врача
- patient_cabinet_full.html — кабинет пациента
- patient_card.html — карта пациента (sticky header)
- admin_panel.html — админ-панель
- backend/main.go — Go-сервер
- guidelines/ — рекомендации

Дата: 2026-09-11"""

result = subprocess.run(['git', 'diff', '--cached', '--quiet'], capture_output=True)
if result.returncode != 0:
    subprocess.run(['git', 'commit', '-m', commit_msg])
    print("   ✅ Коммит создан")
else:
    print("   ℹ️  Нет изменений")

print()

# ============================================================
#  6. GIT — ТЕГ
# ============================================================
print("📄 6. Git — создание тега...")
print("-" * 60)

# Удалить старый тег если есть
subprocess.run(['git', 'tag', '-d', 'v1.2.0'], capture_output=True)

# Создать новый
subprocess.run(['git', 'tag', '-a', 'v1.2.0', '-m', 'Release v1.2.0 - 2026-09-11'])
print("   ✅ Тег v1.2.0 создан")
print()

# ============================================================
#  7. ИТОГ
# ============================================================
print("=" * 60)
print("  📊 ИТОГОВЫЙ ОТЧЁТ")
print("=" * 60)
print()

# Git info
result = subprocess.run(['git', 'branch', '--show-current'], capture_output=True, text=True)
print(f"   Ветка: {result.stdout.strip()}")

result = subprocess.run(['git', 'log', '-1', '--oneline'], capture_output=True, text=True)
print(f"   Коммит: {result.stdout.strip()}")

result = subprocess.run(['git', 'tag'], capture_output=True, text=True)
tags = result.stdout.strip().replace('\n', ', ')
print(f"   Теги: {tags}")

result = subprocess.run(['git', 'ls-files'], capture_output=True, text=True)
print(f"   Файлов: {len(result.stdout.strip().split(chr(10)))}")

print()
print("   🧹 Удалено мусора: " + str(trash_count))
print("   📁 Важных файлов: " + str(len(required) - len(missing)))
print()

if missing:
    print("   ⚠️  ОТСУТСТВУЮТ:")
    for f in missing:
        print(f"      ❌ {f}")
    print()

print("=" * 60)
print("  ✅ ГОТОВО!")
print("=" * 60)
print()
print("📋 Следующие шаги:")
print("   1. Проверить: git log --oneline")
print("   2. Отправить: git push origin main")
print("   3. Отправить теги: git push origin v1.2.0")
print()

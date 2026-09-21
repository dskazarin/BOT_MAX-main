# BOT_MAX — заметки для продолжения

## Запуск

- make run — запуск сервера (Go, backend/main.go)
- make status — проверка
- make stop — остановка

## Стек

- Backend: Go (backend/main.go) + SQLite
- Frontend: HTML + CSS + JS (инлайн)
- Хранилище: localStorage (doctorPatients, patient_vitals_*, patient_card_*)
- API: /api/health, /api/specialties, /api/check
- Рекомендации: guidelines/lor/active/v2024/guideline.json

## Что сделано (22 коммита)

- Модалки, телефон/email, автоформат, валидация
- Поиск, сортировка, фильтры
- Экспорт/импорт JSON
- Редактирование (список + карточка)
- Диаграмма Chart.js
- 12 карточек статистики
- Тёмная/светлая темы
- Makefile, README, .gitignore, devcontainer

## В бэклоге

- Backend + SQLite (персистентное хранилище пациентов)
- Авторизация (JWT)
- Мобильная адаптация
- Тёмная тема для patient_card.html

## Принципы

- Целостность кода, патчи через tempfile
- Проверка баланса скобок и дубликатов
- Разведка с cat -A перед anchor
- Тесты в браузере перед коммитом

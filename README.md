# 🏥 BOT_MAX — ЛОР-платформа

Медицинская система для ЛОР-врачей: рекомендации, пациенты, витальные,
административная панель. Go-бэкенд + ванильный JS-фронт.

## Стек

| Слой | Технология |
|------|------------|
| Backend | Go 1.26, `net/http`, `database/sql` |
| БД | SQLite (`modernc.org/sqlite`, pure-Go, без CGO) |
| Auth | JWT HS256 (access 15m, refresh 7d), HttpOnly-cookie, refresh-ротация |
| Frontend | Ванильный JS + `auth.js` (IIFE, `window.BotMaxAuth`) |
| Tests | `go test` + `:memory:` SQLite |
| CI/CD | GitHub Actions (`vet`/`build`/`test -race` + docker smoke) |

## Быстрый старт (локально)

Установите Go 1.26+ и запустите:

    git clone <repo> && cd BOT_MAX-main
    go run ./backend

Сервер сам создаст `bot_max.db` (SQLite) и сидирует dev-аккаунты:

- `doctor@local` / `doctor@local` (doctor)
- `admin@botmax.local` / `admin` (admin)
- `superadmin@botmax.local` / `superadmin` (superadmin)

Откройте http://localhost:8082

## Docker

    cp .env.example .env
    docker compose up --build

→ http://localhost:8082

## Конфигурация

Все переменные окружения (см. `.env.example`):

| Переменная | Значения | Default | Что делает |
|---|---|---|---|
| `BOTMAX_ENV` | `dev` или `prod` | `dev` | prod: строгая валидация JWT, cookie Secure |
| `BOTMAX_JWT_SECRET` | строка | пусто (dev-секрет) | Обязателен в prod, >= 32 байт |
| `BOTMAX_AUTH_ENFORCE` | `1/0/true/false/yes/no/on/off` | `false` | Без токена: 401 (true) или soft-mode (false) |
| `BOTMAX_COOKIE_SECURE` | то же | auto (prod=true, dev=false) | Secure-флаг на auth-cookie |

## API

Все защищённые эндпоинты требуют access-токен (cookie `botmax_access`
или `Authorization: Bearer`).

### Auth (публичные)

| Метод | URL | Описание |
|---|---|---|
| `GET` | `/api/health` | Health check |
| `POST` | `/api/auth/login` | Логин: `{"email","password"}` → токены + cookie |
| `POST` | `/api/auth/refresh` | Обновление пары (cookie или `{"refreshToken"}`) |
| `POST` | `/api/auth/logout` | Отзыв refresh + снятие cookie |

### Patients (doctor+)

| Метод | URL | Описание |
|---|---|---|
| `GET` | `/api/patients` | Список |
| `POST` | `/api/patients` | Создать |
| `GET` | `/api/patients/{id}` | Один |
| `PUT` | `/api/patients/{id}` | Обновить |
| `DELETE` | `/api/patients/{id}` | Удалить (только admin/superadmin) |
| `GET`/`PUT` | `/api/patients/{id}/vitals` | Витальные |
| `GET` | `/api/patients/{id}/vitals/history` | История витальных |
| `GET`/`PUT` | `/api/patients/{id}/card` | Карта пациента |

### Doctors (admin/superadmin)

| Метод | URL | Описание |
|---|---|---|
| `GET` | `/api/doctors` | Список (`?include_inactive=1` — все) |
| `POST` | `/api/doctors` | Создать (admin — только doctor) |
| `GET` | `/api/doctors/{id}` | Один |
| `PATCH` | `/api/doctors/{id}` | Частичное обновление |
| `DELETE` | `/api/doctors/{id}` | Soft-delete (active=0) |

### Audit (admin/superadmin)

| Метод | URL | Описание |
|---|---|---|
| `GET` | `/api/audit` | Журнал: `?limit=&doctor_id=&action=&entity=&since=&until=` |

## Роли

Иерархия: `superadmin` > `admin` > `doctor`.

| Действие | doctor | admin | superadmin |
|---|---|---|---|
| Читать свои пациенты | да | да | да |
| CRUD пациентов | да | да | да |
| Удалять пациентов | нет | да | да |
| CRUD докторов | нет | да (только doctor) | да (любых) |
| Трогать superadmin | нет | нет | да |
| Audit-log | нет | да | да |

## Разработка

    go vet ./...                              # статический анализ
    go build -o /tmp/botmax ./backend         # сборка
    go test -count=1 -race ./backend/...      # тесты с race-детектором

CI (GitHub Actions) прогоняет vet / build / test -race + собирает
Docker-образ и делает smoke-проверку `/api/health` в контейнере.

## Структура

    backend/
      main.go        — старт сервера, роуты, seed, middleware
      db.go          — SQLite схема + базовые CRUD
      auth.go        — JWT, bcrypt, login/refresh/logout, middleware, roles
      cookies.go     — установка/снятие auth-cookie
      refresh.go     — refresh-ротация (store, replay-каскад)
      env.go         — fail-fast валидация окружения
      seed.go        — dev-seed (только в dev)
      ratelimit.go   — rate-limit на /api/auth/login
      patients.go    — CRUD пациентов
      doctors.go     — CRUD докторов (admin/superadmin)
      audit.go       — audit-log (запись + read-API)
    *.html           — ванильный фронт
    auth.js          — клиентский модуль авторизации

## Безопасность (B1–B10)

- **B1 (6.10)** — fail-fast env: prod без JWT-секрета не стартует.
- **B2 (6.11)** — dev-seed не выполняется в prod.
- **B3 (6.12)** — rate-limit на `/api/auth/login` (per-IP + per-email).
- **B4 (6.13)** — одноразовые refresh + replay-каскад + logout-отзыв.
- **B5 (6.14)** — admin CRUD по докторам (self-delete защита, admin≠superadmin).
- **B6 (6.15)** — фронт-guard'ы: `requireAuth()`, `isAdmin()`.
- **B7 (6.16)** — HttpOnly-cookie + SameSite=Strict (CSRF-защита).
- **B8 (6.17)** — полный переход на cookie (localStorage/Bearer убраны).
- **B9 (6.18)** — audit-log: кто, когда, что.
- **B10 (6.19)** — Docker + CI (текущий).

SQL-инъекции — параметризованные запросы. XSS — escape на выходе.
Пароли — bcrypt. Токены — HS256, короткий TTL, refresh-ротация.

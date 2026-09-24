# SNAPSHOTS

Журнал подпатчей. Один подпатч = один или несколько атомарных коммитов.
Формат записи: номер (буква) — тема — дата — коммиты — файлы — что сделано.

---

## Завершённые

### 6.10 (B1) — env validation fail-fast
- **Дата:** 2026-09-24
- **Коммиты:**
  - `472b047` feat(env): подпатч 6.10 (ч.1) — validateEnv + перенос devJWTSecret
  - `6fd71d3` feat(env): подпатч 6.10 (ч.2) — jwtSecret/authEnforce через currentEnv + validateEnv в main
  - `f1829b5` test(env): подпатч 6.10 (ч.4) — env_test.go
- **Файлы:**
  - `backend/env.go` (new) — validateEnv, Env, devJWTSecret, currentEnv
  - `backend/env_test.go` (new) — табличные тесты
  - `backend/auth.go` — jwtSecret() / authEnforceEnabled() через currentEnv; удалён дубликат devJWTSecret; убран import "os"
  - `backend/main.go` — validateEnv() первой строкой main(), log.Fatalf при ошибке
- **Что сделано:**
  - `BOTMAX_ENV`: `"" | dev | development | test → dev`, `prod | production → prod`, мусор → ошибка.
  - `BOTMAX_JWT_SECRET` в prod: обязателен, ≥ 32 байт → иначе `log.Fatalf` на старте.
  - `BOTMAX_AUTH_ENFORCE`: строгий парсинг `1|0|true|false|yes|no|on|off`, мусор → ошибка (fail-fast).
  - `jwtSecret()`/`authEnforceEnabled()` больше не читают env напрямую — только через `currentEnv`.
  - Тесты: `TestNormalizeEnvName`, `TestParseAuthEnforce`, `TestValidateEnv` (10 подкейсов), `TestJwtSecret_*` — все PASS.
- **Smoke:**
  - dev без env — стартует, `env=dev jwt_secret_len=0`.
  - prod без секрета / короткий секрет — `exit=1`, лог с причиной.
  - prod с длинным секретом — `health: ok`.

---

### 6.11 (B2) — seed dev-аккаунтов только в dev
- **Дата:** 2026-09-24
- **Коммиты:**
  - `5b5035b` feat(seed): подпатч 6.11 (B2.1) — dev-аккаунты не сидятся в prod
  - `f6b8eb8` test(seed): подпатч 6.11 (B2.2) — seed_test.go
- **Файлы:**
  - `backend/seed.go` (new) — ShouldSeedDevAccounts + runDevSeeds + envNameOrUnknown
  - `backend/seed_test.go` (new) — TestShouldSeedDevAccounts, TestEnvNameOrUnknown
  - `backend/main.go` — два вызова (seedDefaultDoctor + seedAdminRoles) заменены на один runDevSeeds(db, currentEnv)
- **Что сделано:**
  - В prod dev-seed'ы (DOC-001, ADMIN-001, SUPER-001) НЕ выполняются; в лог — явный «🌱 seed: пропущен (env=prod ...)».
  - Гейт через ShouldSeedDevAccounts(env *Env) bool — смотрит только на env.IsProd (env == nil → false).
  - Тесты: 5 подкейсов ShouldSeedDevAccounts + 4 envNameOrUnknown — все PASS.
- **Smoke (end-to-end через /api/auth/login):**
  - PROD (чистая БД): «🌱 seed: пропущен»; admin/superadmin/doctor → HTTP 401 invalid credentials.
  - DEV  (чистая БД): три 🌱 Создан ...; admin/superadmin/doctor → HTTP 200 + accessToken + role.

---

### 6.12 (B3) — rate-limit на /api/auth/login
- **Дата:** 2026-09-24
- **Коммиты:**
  - `5ed6b5e` feat(ratelimit): подпатч 6.12 (B3.1) — in-memory limiter
  - `a38e2b9` feat(auth): подпатч 6.12 (B3.2) — rate-limit на login
  - `691c2d5` test(ratelimit): подпатч 6.12 (B3.3) — ratelimit_test.go
- **Файлы:**
  - `backend/ratelimit.go` (new) — Limiter (скользящее окно, sync.Mutex), clientIP (XFF+RemoteAddr), writeTooManyRequests (429 + Retry-After + JSON)
  - `backend/ratelimit_test.go` (new) — 4 теста Limiter + TestClientIP (5 подкейсов)
  - `backend/auth.go` — врезка в handleAuthLogin: rate-limit до bcrypt, Reset после успешного логина
- **Что сделано:**
  - Лимитеры: per-IP 10/15min, per-email 5/15min (in-memory, скользящее окно).
  - На превышение — HTTP 429 + заголовок Retry-After + тело {"error":"too many requests","retryAfter":N}.
  - Успешный логин — сбрасывает оба счётчика (per-IP + per-email).
  - Проверка стоит до getDoctorByEmail/bcrypt — чтобы 429 не стоил дорогой bcrypt-операции.
  - XFF учитывается первым, иначе RemoteAddr; помечено, что XFF — только за доверенным прокси.
- **Тесты:**
  - TestLimiterAllow, TestLimiterReset, TestLimiterWindowExpiry, TestLimiterIndependentKeys — PASS.
  - TestClientIP: no-xff-with-port, no-xff-no-port, xff-single, xff-multi, xff-with-spaces — PASS.
- **Smoke (end-to-end через /api/auth/login):**
  - dev, чистая БД: попытки 1–5 → 401; попытка 6 → 429 retry=899.
  - 7-й запрос с ВЕРНЫМ паролем → тоже 429 (счётчик исчерпан — правильно).
  - Свежий процесс (счётчики in-memory обнулились) → 200 + "role":"admin".

---

## Следующий

### 6.13 (B4) — refresh-ротация (черновик)
- **Статус:** запланирован
- **Цель:** одноразовые refresh-токены (rotation) + blacklist jti при logout; повторное использование старого refresh → инвалидация всей сессии.
- **Опоры:** IssueAccessToken/IssueRefreshToken/ParseToken/registerAuthRoutes (auth.go), env-валидация (B1).
- **Файлы (ожидаемо):** `backend/auth.go`, возможно новый `backend/refresh.go`, миграция таблицы (если уже не хранится jti).
- **Проверки (ожидаемо):** go test + smoke: login → refresh → refresh → old refresh отбивается; logout → refresh больше не работает.

---

## Правила
- 1 подпатч = 1 коммит (или несколько — если подпатч намеренно разбит на части, как 6.10).
- Перед завершением подпатча — запись в этом файле: что сделано + следующий номер.
- Никогда не коммитить без зелёных `go vet ./...`, `go build ./backend`, `go test ./backend/...`.
- Никогда не пушить в prod без проверки на localhost.

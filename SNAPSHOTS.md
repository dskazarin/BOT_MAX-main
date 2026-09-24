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

### 6.13 (B4) — refresh-ротация
- **Дата:** 2026-09-24
- **Коммиты:**
  - `75a7768` feat(refresh): 6.13 (B4.1) — refresh_tokens + store
  - `1634c64` feat(refresh): 6.13 (B4.2) — jti регистрируется при выдаче
  - `bf4765d` feat(refresh): 6.13 (B4.3) — ротация + replay-каскад
  - `60375ca` feat(refresh): 6.13 (B4.4) — logout отзывает jti
  - `c4ff590` test(refresh): 6.13 (B4.5) — refresh_test.go
- **Файлы:**
  - `backend/refresh.go` (new) — store: SaveRefreshJTI, IsRefreshRevoked, RevokeRefreshJTI, RevokeAllForDoctor; lazy-cleanup expired
  - `backend/refresh_test.go` (new) — 7 тестов через :memory: SQLite
  - `backend/db.go` — миграция: таблица refresh_tokens + 2 индекса
  - `backend/auth.go` — IssueRefreshToken возвращает (token, jti, exp, err); handleAuthLogin/handleAuthRefresh регистрируют jti; handleAuthRefresh — ротация + replay-каскад; handleAuthLogout — отзыв jti из тела запроса
- **Что сделано:**
  - Одноразовые refresh-токены: при ротации старый jti помечается revoked_at + replaced_by.
  - Replay: если приходит уже отозванный jti — каскад RevokeAllForDoctor + 401.
  - Logout с телом {refreshToken} отзывает jti; без тела/с мусором — 200 (не падает).
  - SaveRefreshJTI чистит expired (lazy-cleanup: DELETE WHERE expires_at < now).
  - IsRefreshRevoked: неизвестный/пустой jti → true (безопасный дефолт).
- **Тесты (`:memory:` SQLite):**
  - TestRefresh_SaveAndNotRevoked, TestRefresh_UnknownIsRevoked, TestRefresh_EmptyJTIRevoked, TestRefresh_Revoke, TestRefresh_RevokeIdempotent, TestRefresh_RevokeAllForDoctor, TestRefresh_SaveCleansExpired — все PASS.
- **Smoke (end-to-end через /api/auth):**
  - login → refresh(R1) → 200 (R2). refresh(R1) повторно → 401 + 🚨 REPLAY (каскад, отозвано 1).
  - refresh(R2) после каскада → 401 (уже revoked).
  - logout(R1) → 200. refresh(R1) → 401 (отозван).
  - logout без тела / с мусором → 200.

---

## Следующий

### 6.14 (B5) — admin CRUD по докторам (черновик)
- **Статус:** запланирован
- **Цель:** POST/PATCH/DELETE /api/doctors, доступ только admin/superadmin (через requireRole).
- **Опоры:** enum Role + requireRole (6.8), resolveDoctorID (6.5), initDB/schema в db.go.
- **Файлы (ожидаемо):** `backend/doctors.go` (new — CRUD-хендлеры + store), `backend/auth.go` (регистрация маршрутов), возможно `backend/doctors_test.go`.
- **Проверки (ожидаемо):** go test + smoke: admin создаёт/меняет/удаляет доктора; doctor → 403; аноним → 401.

---

## Правила
- 1 подпатч = 1 коммит (или несколько — если подпатч намеренно разбит на части, как 6.10).
- Перед завершением подпатча — запись в этом файле: что сделано + следующий номер.
- Никогда не коммитить без зелёных `go vet ./...`, `go build ./backend`, `go test ./backend/...`.
- Никогда не пушить в prod без проверки на localhost.

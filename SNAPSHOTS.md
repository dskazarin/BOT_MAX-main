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

### 6.14 (B5) — admin CRUD по докторам
- **Дата:** 2026-09-25
- **Коммиты:**
  - `2dffef7` feat(doctors): 6.14 (B5.1) — store
  - `7486ee7` feat(doctors): 6.14 (B5.2) — HTTP handlers + routes
  - `a43d24f` feat(doctors): 6.14 (B5.3) — бизнес-правила
  - `49530f7` test(doctors): 6.14 (B5.4) — doctors_test.go
- **Файлы:**
  - `backend/doctors.go` (new) — store + HTTP handlers + registerDoctorRoutes
  - `backend/doctors_test.go` (new) — 7 тестов через :memory: SQLite
  - `backend/main.go` — вызов registerDoctorRoutes()
- **Что сделано:**
  - Store: listDoctors, createDoctor, updateDoctor, deleteDoctorByID (soft-delete active=0), getDoctorRowByID.
  - HTTP: GET/POST /api/doctors, GET/PATCH/DELETE /api/doctors/{id} — все под guard requireRole(admin, superadmin).
  - DoctorRow (расширенная структура) — не трогает минимальный Doctor из db.go.
  - Бизнес-правила: self-delete/self-deactivate → 400; admin не трогает superadmin (PATCH/DELETE) → 403; admin не создаёт admin/superadmin → 403.
- **Тесты (:memory: SQLite):**
  - TestDoctors_CreateAndGet, TestDoctors_CreateDefaults, TestDoctors_CreateValidation (4 подкейса), TestDoctors_CreateUniqueEmail, TestDoctors_List, TestDoctors_Update, TestDoctors_DeleteSoft — все PASS.
- **Smoke (end-to-end через /api/doctors, BOTMAX_AUTH_ENFORCE=1):**
  - Аноним → 401; admin GET → 200; admin POST → 201; admin PATCH → 200; admin DELETE → 200.
  - doctor GET → 403; admin self-delete → 400; admin self-deactivate → 400; admin PATCH SUPER → 403; admin DELETE SUPER → 403; admin POST superadmin → 403.
  - superadmin PATCH ADMIN → 200; superadmin DELETE ADMIN → 200.

---

### 6.15 (B6) — frontend auth-интеграция
- **Дата:** 2026-09-25
- **Коммиты:**
  - `240fa35` feat(frontend): 6.15 (B6.1) — admin_panel защищена
  - `c376dd1` feat(frontend): 6.15 (B6.3) — admin-кнопка скрыта для не-admin
  - `52ce4c1` feat(frontend): 6.15 (B6.2) — patient_cabinet_full защищён requireAuth
- **Файлы:**
  - `admin_panel.html` — подключён auth.js + guard requireAuth()+isAdmin()
  - `index.html` — admin-кнопка (#admin-entry-btn) скрывается для не-admin
  - `patient_cabinet_full.html` — подключён auth.js + requireAuth()
- **Что сделано:**
  - admin_panel.html: аноним → login.html, doctor → doctor_cabinet.html, admin/superadmin → видит панель.
  - index.html: admin-кнопка видна анониму (для логина) и админам; скрыта для doctor.
  - patient_cabinet_full.html: requireAuth() — аноним → login.html.
  - Уже было (шаг 6.7): auth.js с BotMaxAuth (authFetch, refreshAccess, isAdmin/isDoctor, applyRoleVisibility); doctor_cabinet.html и patient_card.html — защищены.
- **Smoke (curl -L, файл отдаётся + guard присутствует):**
  - admin_panel.html → 200, 2× B6.1
  - index.html → 200, 1× B6.3, 2× admin-entry-btn
  - patient_cabinet_full.html → 200, 2× B6.2, 1× BotMaxAuth.requireAuth
  - auth.js → 200

---

### 6.16 (B7) — CSRF / httpOnly-cookies
- **Дата:** 2026-09-25
- **Коммиты:**
  - `a118425` feat(env): 6.16 (B7.1) — CookieSecure в Env
  - `aeabf42` test(env): 6.16 (B7.1.1) — TestValidateEnv + CookieSecure
  - `d373e19` feat(auth): 6.16 (B7.2) — cookie access+refresh в login/refresh/logout
  - `3f1b3be` feat(auth): 6.16 (B7.3) — middleware читает access из cookie
  - `b7f369e` feat(frontend): 6.16 (B7.4) — credentials include в auth.js + login.html
- **Файлы:**
  - `backend/env.go` — Env.CookieSecure + parseCookieSecure (BOTMAX_COOKIE_SECURE, default=IsProd)
  - `backend/env_test.go` — TestParseCookieSecure (12 кейсов) + TestValidateEnv (13 кейсов)
  - `backend/cookies.go` (new) — setAuthCookies / clearAuthCookies (HttpOnly, SameSite=Strict, Secure=IsProd, Path=/api и /api/auth)
  - `backend/auth.go` — cookie в login/refresh/logout; extractAccessToken (Bearer → cookie)
  - `auth.js` — credentials: include в refreshAccess, buildOpts, logout
  - `login.html` — credentials: include в fetch login
- **Что сделано:**
  - BOTMAX_COOKIE_SECURE: "" → IsProd; 1/true/yes/on → true; 0/false/no/off → false; мусор → ошибка.
  - login/refresh ставят Set-Cookie botmax_access + botmax_refresh (параллельно JSON — обратная совместимость).
  - logout снимает cookie (Max-Age=0).
  - middleware читает access из Authorization: Bearer ИЛИ из cookie botmax_access.
  - SameSite=Strict достаточно для same-origin SPA — отдельный CSRF-токен не нужен.
- **Smoke (end-to-end через curl):**
  - login → 2× Set-Cookie (Path=/api, Path=/api/auth, HttpOnly, SameSite=Strict).
  - GET /api/doctors с cookie (без Bearer) → 200.
  - GET /api/doctors без cookie и Bearer → 401.
  - GET /api/doctors с Bearer (обратная совместимость) → 200.
  - logout с cookie → 200 + 2× Set-Cookie с Max-Age=0.

---

### 6.17 (B8) — полный переход на cookie
- **Дата:** 2026-09-25
- **Коммиты:**
  - `370d14f` feat(auth): 6.17 (B8.1) — refresh/logout принимают refresh из cookie
  - `645f9fc` refactor(frontend): 6.17 (B8.2) — auth.js только на cookie (без localStorage/Bearer)
  - `f871b81` refactor(frontend): 6.17 (B8.3) — HTML переведены на cookie-auth
- **Файлы:**
  - `backend/auth.go` — handleAuthRefresh/handleAuthLogout: best-effort Decode + fallback на cookie botmax_refresh
  - `auth.js` — убраны K_ACCESS/K_REFRESH, getToken/getRefreshToken/setTokens/getAuthHeaders/Bearer; добавлены isLoggedIn/setDoctor/clearDoctor
  - `admin_panel.html`, `index.html`, `login.html` — getToken() → isLoggedIn(); setTokens() → setDoctor()
  - `doctor_cabinet.html`, `patient_card.html` — удалены мёртвые обёртки getAuthHeaders
- **Что сделано:**
  - Токены хранятся ТОЛЬКО в HttpOnly-cookie (JS не видит).
  - localStorage содержит только botmax_doctor (для UI).
  - refresh/logout работают без тела — сервер берёт refresh из cookie.
  - Никаких Bearer-заголовков на фронте.
- **Smoke (end-to-end через curl):**
  - login → cookie jar (HttpOnly botmax_access + botmax_refresh).
  - refresh без тела → 200 + новая пара Set-Cookie.
  - GET /api/doctors с cookie → 200; без cookie → 401.
  - logout без тела → 200 + Set-Cookie Max-Age=0.
  - GET после logout (jar обновлён) → 401.

---

### 6.18 (B9) — audit-log
- **Дата:** 2026-09-25
- **Коммиты:**
  - `c82dd85` feat(audit): 6.18 (B9.1) — audit_log схема + store + read-API
  - `c74f399` feat(audit): 6.18 (B9.2) — audit в auth + регистрация /api/audit
  - `c1ea053` feat(audit): 6.18 (B9.3) — audit в doctors CRUD
  - `e7eb130` feat(audit): 6.18 (B9.4) — audit в patients CRUD
  - `a358260` test(audit): 6.18 (B9.6) — audit_test.go
- **Файлы:**
  - `backend/audit.go` (new) — auditLog, auditCleanup, handleAuditList, registerAuditRoutes
  - `backend/audit_test.go` (new) — 6 тестов store
  - `backend/db.go` — миграция: таблица audit_log + 3 индекса
  - `backend/auth.go` — audit на login (успех/отказ), refresh, replay, logout
  - `backend/doctors.go` — audit на doctor.create/update/delete
  - `backend/patients.go` — audit на patient.create/update/delete
  - `backend/main.go` — вызов registerAuditRoutes()
- **Что сделано:**
  - Best-effort запись в audit_log: ошибки не влияют на HTTP-ответ.
  - Фиксируются: doctor_id (actor), action, entity, entity_id, ip, detail (JSON), ts.
  - Read-API GET /api/audit с фильтрами (limit, doctor_id, action, entity, since, until), только admin/superadmin.
  - ts формируется в Go (time.Now().UTC()), не SQLite — легче тестировать.
  - auditCleanup(days) — задел на retention (не вызывается в B9).
- **Тесты (:memory: SQLite):**
  - TestAudit_InsertAndFetch, TestAudit_DetailJSON, TestAudit_NilDB, TestAudit_EmptyAction, TestAudit_Cleanup, TestAudit_Order — все PASS.
- **Smoke (end-to-end через curl):**
  - auth: login ×2, login.fail ×1, refresh ×1, logout ×1 — GET /api/audit показывает все.
  - doctors: create/update/delete — doctor.create/update/delete в audit.
  - patients: create/update/delete — patient.create/update/delete в audit.
  - GET /api/audit без cookie — 401.

---

### 6.19 (B10) — Docker + CI
- **Дата:** 2026-09-25
- **Коммиты:**
  - dfd9897 — Dockerfile
  - e86ec7d — .dockerignore
  - 0edde1a — docker-compose.yml
  - 79f88fd — GitHub Actions CI
  - e4f2b86 — .env.example + .gitignore
  - 5237d0a — README переписан
- **Файлы:** Dockerfile, .dockerignore, docker-compose.yml, .github/workflows/ci.yml, .env.example, .gitignore, README.md
- **Что сделано:** multi-stage Dockerfile (alpine + tini), CI (vet/build/test + docker smoke), compose с заделом на Postgres.
- **Проверки:** go vet / build / test -race — зелёные; docker build — через CI.
---
## Следующий

### 6.20 (B11) — BOTMAX_DB_PATH в main.go (черновик)
- **Статус:** запланирован
- **Цель:** вынести путь к SQLite из хардкода в env BOTMAX_DB_PATH (default — rootDir/bot_max.db).
- **Опоры:** main.go (InitDB call), env.go (Env struct).
- **Файлы (ожидаемо):** backend/env.go (+DBPath), backend/main.go (использовать env.DBPath), .env.example.
- **Проверки (ожидаемо):** go test + smoke: BOTMAX_DB_PATH=/tmp/foo.db → файл там; docker-compose volume /data работает.

---

## Правила
- 1 подпатч = 1 коммит (или несколько — если подпатч намеренно разбит на части, как 6.10).
- Перед завершением подпатча — запись в этом файле: что сделано + следующий номер.
- Никогда не коммитить без зелёных `go vet ./...`, `go build ./backend`, `go test ./backend/...`.
- Никогда не пушить в prod без проверки на localhost.

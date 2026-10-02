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
### 6.20 (B11) — BOTMAX_DB_PATH в main.go
- **Дата:** 2026-09-25
- **Коммиты:**
  - c02d873 — BOTMAX_DB_PATH в Env
  - 426b546 — main.go использует BOTMAX_DB_PATH
- **Файлы:** backend/env.go, backend/env_test.go, backend/main.go, .env.example
- **Что сделано:** путь к SQLite вынесен в env; fallback на rootDir/bot_max.db; docker volume /data работает.
- **Smoke:** BOTMAX_DB_PATH=/tmp/foo.db → файл создан, /api/health → ok; без env → rootDir (как раньше).

---

### 6.21 (B12) — E2E-тесты
- **Дата:** 2026-09-25
- **Коммиты:**
  - edb8d5c — каркас httptest + smoke health
  - 9cca582 — auth-сценарии (login/refresh/logout)
  - 155a19f — роли (401/403/200)
  - 383c182 — CRUD + audit
- **Файлы:** backend/e2e_test.go (new) — 20 тестов
- **Что сделано:** httptest.NewServer поверх http.DefaultServeMux, :memory: SQLite, seed (admin/superadmin/doctor). Покрыты: health, auth (login ok/fail, refresh cookie, logout + replay), роли (401/403/200), CRUD patients/doctors, audit-записи и фильтры.
- **Проверки:** go test -v -run TestE2E → 20 PASS.

---

### 6.22 (B13) — Admin UI для /api/audit
- **Дата:** 2026-09-25
- **Коммиты:**
  - 7561af2 — admin_audit.html (audit UI)
  - 5cd1383 — ссылка на журнал в admin_panel
- **Файлы:** admin_audit.html (new), admin_panel.html (ссылка)
- **Что сделано:** тёмная страница admin_audit.html — таблица audit_log с фильтрами (action, doctor_id, since, limit), кнопки «Применить»/«Сбросить»/«Обновить», guard requireAuth+isAdmin, authFetch. Ссылка «📜 Журнал» в шапке admin_panel.html.
- **Проверки:** go test — 20 E2E PASS; smoke: admin_audit.html → 200, admin_panel.html содержит ссылку.

---

### 6.23 (B14) — Structured logging
- **Дата:** 2026-09-25
- **Коммиты:**
  - 84546e9 — logging.go (slog setup)
  - ac77485 — main.go на slog
  - 78f5818 — auth.go на slog
  - e4389de — doctors/audit/seed/db на slog
- **Файлы:** logging.go (new), main.go, auth.go, doctors.go, audit.go, seed.go, db.go
- **Что сделано:** setupLogging(env) → slog.SetDefault: JSON в stdout (prod), text в stderr (dev). Все log.* / fmt.Print* заменены на slog.Info/Warn/Error с key-value полями (44 вызова). log.Fatalf → slog.Error + os.Exit(1).
- **Проверки:** go test — 20 E2E PASS; smoke: лог в slog-формате (time=… level=INFO msg=… key=value).

---

### 6.24 (B15) — Admin UI для doctors CRUD
- **Дата:** 2026-09-25
- **Коммиты:**
  - e745200 — admin_doctors.html (каркас, read-only)
  - 93c953d — ссылка «👨⚕️ Доктора» в admin_panel
  - b86c00a — create-модалка (POST /api/doctors)
  - f056b08 — edit/toggle/delete (PATCH + DELETE)
- **Файлы:** admin_doctors.html (new, ~645 строк), admin_panel.html (ссылка)
- **Что сделано:**
  - Тёмная страница admin_doctors.html по шаблону admin_audit.html: inline-guard requireAuth → login.html, isAdmin → doctor_cabinet.html; authFetch, escapeHtml.
  - Таблица докторов: ID, Email, Имя, Роль, Специальность, Телефон, Активен, Создан, Последний вход, Действия.
  - Фильтр «Показать неактивных» → GET /api/doctors?include_inactive=1.
  - Create-модалка: email/password/name/specialty/role/phone → POST. Селект admin/superadmin виден только superadmin. Ошибки 400/403/409 — в alert внутри модалки.
  - Edit-модалка: name/specialty/phone/active → PATCH. Self-защита: свои строки — active disabled.
  - Deactivate/Activate → PATCH {active:bool}; Delete → DELETE (confirm). Self + superadmin-строки заблокированы на фронте (совпадает с 400/403 бэка).
- **Проверки:** go vet — OK; go build -o /tmp/botmax_server ./backend — OK; go test ./backend/... — PASS.
- **Smoke (localhost:8082, BOTMAX_ENV=dev, BOTMAX_DB_PATH=/tmp/b15_smoke.db):**
  - /api/health → ok
  - admin_doctors.html → HTTP 200
  - admin_panel.html содержит ссылки 📜 Журнал + 👨⚕️ Доктора
  - login superadmin@botmax.local → 200
  - GET /api/doctors → seed SUPER-001, ADMIN-001, DOC-001
  - POST → DOC-1790334820-887b756c (201)
  - PATCH name/phone → OK; PATCH active=false → OK
  - GET ?include_inactive=1 → 4 записи (включая деактивированного)
  - DELETE → {"ok":true}
  - audit: doctor.create, doctor.update×2, doctor.delete — все присутствуют.
  - **Примечание:** фильтр /api/audit?action=... — точное совпадение (doctor.create ✓, doctor. ✗). Особенность B9, не баг.

---

### 6.25 (B16) — Префиксный фильтр /api/audit?action=doctor.*
- **Дата:** 2026-10-01
- **Коммиты:**
  - 3c0a622 — префиксный фильтр action=doctor.* (backend + тесты)
  - d2e9905 — placeholder в admin_audit.html
- **Файлы:** backend/audit.go, backend/audit_test.go, backend/e2e_test.go, admin_audit.html
- **Что сделано:**
  - `auditActionClause(action)` — чистая функция, строит SQL-условие.
    - `"doctor.create"` → `action = ?` (точное, как раньше).
    - `"doctor.*"` → `action LIKE ? ESCAPE '\'` с prefix=`doctor.%`.
  - Wildcards `%`, `_`, `\` в пользовательском префиксе экранируются через `strings.NewReplacer`.
  - `handleAuditList` использует новый helper для параметра `action`.
  - `admin_audit.html` — placeholder: «auth.login, doctor.* (звёздочка = префикс)».
- **Тесты:**
  - `TestAudit_ActionClause` — 6 кейсов: empty / exact / prefix / prefix2 / escape_pct / escape_us.
  - `TestE2E_Audit_Filters` расширен: `action=doctor.*` → только doctor.*, `action=auth.login` → только auth.login.
- **Проверки:** go vet — OK; go build -o /tmp/botmax_server ./backend — OK; go test ./backend/... — PASS.
- **Smoke (localhost:8082, BOTMAX_ENV=dev, BOTMAX_DB_PATH=/tmp/b16_smoke.db):**
  - POST/PATCH/DELETE доктора → в audit_log: doctor.create, doctor.update, doctor.delete.
  - `?action=doctor.*` → count=3 (delete, update, create). ✅
  - `?action=doctor.create` (exact) → count=1. ✅
  - `?action=auth.login` (exact) → count=1. ✅
  - `?action=doctor.` (без `*`, точное) → count=0. ✅ (обратная совместимость сохранена)

## Следующий

### 6.26 (B17.1) — ЗАКРЫТ
- **Статус:** закрыт (коммит 8b41965 + prep 1ae7f8a)
- **Что сделано:** префиксный фильтр `entity=doctor.*` (симметрия с `action`).
  - `auditActionClause` отрефакторена в общий `auditPrefixClause(column, value)`
    + обёртки `auditActionClause("action", …)` / `auditEntityClause("entity", …)`.
  - `handleAuditList` теперь использует `auditEntityClause` вместо `"entity = ?"`.
  - Тесты: `TestAudit_EntityClause` (6 кейсов, зеркало action) +
    `TestAudit_EntityPrefix_Filter` (интеграционный, составные entity).
  - **Семантика:** `.*` — это LIKE-prefix (`doctor.%`), не матчит однословное
    `doctor`. В текущей системе entity однословные → `entity=doctor.*` = `[]`;
    `entity=doctor` (exact) работает. Симметрия с `action=doctor.*` сохранена.
  - **Smoke:** HTTP `?entity=doctor` → 1 запись `doctor.create`;
    `?action=doctor.*` (B16) → та же запись (регрессии нет);
    `?entity=doctor.*` → `[]` (ожидаемо).

### 6.26 (B17.2) — ЗАКРЫТ
- **Статус:** закрыт (коммиты 2dcee6e + ec9d736 + 13bfbf4 + 8009782)
- **Что сделано:** UI-фильтр `entity` в `admin_audit.html` (симметрия с `action`, B16.2).
  - **prep 2dcee6e** `style(backend): gofmt -w backend/ (prep B17.2)` — погашен
    предсуществующий техдолг: 10 файлов `backend/*.go` были с 4-пробельными
    отступами, `gofmt -l backend/` репортил их в baseline. Приведены к gofmt
    (табы + выравнивание struct-полей), семантика не менялась.
  - **ec9d736** `feat(admin-ui): input fEntity в admin_audit.html` —
    новый `<input id="fEntity">` после `fDoctor`.
  - **13bfbf4** `feat(admin-ui): params.push(entity=) в buildQuery` —
    `var e = ...fEntity...` + `if (e) params.push('entity=' + encodeURIComponent(e));`.
  - **8009782** `feat(admin-ui): сброс fEntity в resetFilters` —
    `document.getElementById('fEntity').value = '';`.
- **Placeholder в UI:** `doctor, patient (точное совпадение)` —
  сознательно **не** `doctor.* (звёздочка = префикс)`, потому что по семантике
  B17.1 `entity=doctor.*` на однословных entity даёт `[]`; показывать
  «префикс» в placeholder было бы вводящим в заблуждение.
- **Smoke (полный, БД /tmp/b17.db):**
  - `POST /api/doctors` (email/password/name) → `DOC-1790855360-8554ff68`
    + audit `doctor.create`.
  - `?entity=doctor` → 1 запись (`doctor.create`).
  - `?action=doctor.*` → 1 запись (B16.2 не сломан).
  - `?entity=doctor.*` → `[]` (семантика B17.1 подтверждена).
  - `?action=doctor.create` (exact) → 1 запись.
  - HTML `/admin_audit.html` содержит `fEntity` (input + buildQuery + resetFilters).
- **Коммит-сообщения:**
  - `feat(admin-ui): 6.26 (B17.2) — input fEntity в admin_audit.html`
  - `feat(admin-ui): 6.26 (B17.2) — params.push(entity=) в buildQuery`
  - `feat(admin-ui): 6.26 (B17.2) — сброс fEntity в resetFilters`
  - `style(backend): gofmt -w backend/ (prep B17.2)`

<!-- B17_2_SNAPSHOT_APPLIED -->

### 6.27 (B18) — ЗАКРЫТ
- **Дата:** 2026-10-01
- **Статус:** закрыт (коммиты 7a6caab + a8d9953 + 064e6e3)
- **Цель:** role-scoped visibility в `admin_doctors` — non-superadmin не видит
  admin/superadmin в `GET /api/doctors`. Взят кандидат №1 из трёх возможных
  (см. исходный TODO); кандидаты №2 («глазами доктора») и №3 (audit retention)
  перенесены в §6.28 (B19).
- **Файлы:**
  - `backend/doctors.go` — фильтр в `handleDoctorsList`
  - `backend/doctors_test.go` — `TestHandleDoctorsList_RoleFilter` (4 субтеста)
  - `admin_doctors.html` — defense-in-depth в `renderRows`
- **Что сделано:**
  - **7a6caab** `fix(backend): 6.27 (B18) — admin не видит admin/superadmin в GET /api/doctors`
    `handleDoctorsList` читает `doctorRoleFromCtx(r)` и, если актор не superadmin,
    фильтрует список: остаются только `RoleDoctor`. Плюс — `docs == nil → []` для
    корректного JSON (`[]`, а не `null`).
  - **a8d9953** `test(backend): 6.27 (B18) — кейсы admin/superadmin в handleDoctorsList`
    `TestHandleDoctorsList_RoleFilter`: 4 субтеста — admin видит только doctor;
    superadmin видит всех (superadmin, admin, doctor); без роли в ctx — безопасный
    дефолт (только doctor); admin + `include_inactive=1` — видит оба doctor
    (active + soft-deleted). Подмена глобальной `db` через `saved := db; db = newTestDB(t)`;
    тесты не параллельные — подмена безопасна.
  - **064e6e3** `fix(admin-ui): 6.27 (B18) — defense-in-depth фильтр ролей в renderRows`
    В `renderRows` (после `iAmSuper`) — `if (!iAmSuper) rows = rows.filter(r => r.role === 'doctor')`.
    Backend уже фильтрует; UI дублирует от регрессий (тот же паттерн, что `blockedBySuper`
    из B5.3).
- **UI-smoke (полный, БД /tmp/b18.db):**
  - `POST /api/auth/login` (superadmin) → JWT `role=superadmin`.
  - `GET /api/doctors` (superadmin) → `['admin', 'doctor', 'superadmin']`.
  - `POST /api/doctors` (superadmin) → `role=admin` — создан тестовый админ.
  - `POST /api/auth/login` (новый admin) → JWT `role=admin`.
  - `GET /api/doctors` (admin) → **`['doctor']`** — фильтр работает.
  - `POST /api/doctors` (admin, role=superadmin) → **403** — backend-запрет B5.3 не сломан.
- **Коммит-сообщения:**
  - `fix(backend): 6.27 (B18) — admin не видит admin/superadmin в GET /api/doctors`
  - `test(backend): 6.27 (B18) — кейсы admin/superadmin в handleDoctorsList`
  - `fix(admin-ui): 6.27 (B18) — defense-in-depth фильтр ролей в renderRows`

<!-- B18_SNAPSHOT_APPLIED -->

### 6.28 (B19) — ЗАКРЫТ
- **Дата:** 2026-10-01
- **Статус:** закрыт (коммиты 9163921 + 4a17538 + ce991fd)
- **Цель:** audit retention — включить `auditCleanup` в рантайм без внешнего планировщика.
  Взят кандидат №2 из §6.27 (retention); кандидат №1 (impersonation «глазами доктора»)
  перенесён в §6.29 (B20).
- **Файлы:**
  - `backend/env.go` — `parseNonNegativeInt`, `parseDuration`, поля `AuditRetentionDays` + `AuditCleanupInterval`
  - `backend/env_test.go` — `TestParseNonNegativeInt`, `TestParseDuration`, расширение `TestValidateEnv`
  - `backend/audit.go` — `runAuditRetention(ctx, db, days, interval)`
  - `backend/audit_test.go` — 3 теста `TestRunAuditRetention_*`
  - `backend/main.go` — запуск runner'а в горутине + остановка перед `srv.Shutdown`
- **Env-переменные:**
  - `BOTMAX_AUDIT_RETENTION_DAYS` — int >= 0, default `0` (retention выключен).
  - `BOTMAX_AUDIT_CLEANUP_INTERVAL` — Go duration > 0, default `24h`.
- **Что сделано:**
  - **9163921** `feat(env): 6.28 (B19.1) — BOTMAX_AUDIT_RETENTION_DAYS + BOTMAX_AUDIT_CLEANUP_INTERVAL`
    `parseNonNegativeInt` (пусто → def; мусор/отрицательное → ошибка) и `parseDuration`
    (пусто → def; мусор/<= 0 → ошибка). Оба — fail-fast, по образцу `parseAuthEnforce`.
    В `Env` добавлены `AuditRetentionDays int` и `AuditCleanupInterval time.Duration`,
    заполняются в `validateEnv()`.
  - **4a17538** `feat(audit): 6.28 (B19.2) — фоновый runAuditRetention (ticker + первый прогон сразу)`
    `runAuditRetention`: первый прогон сразу, затем `time.NewTicker(interval)` в select
    с `ctx.Done()`. `days <= 0` → лог и выход; `interval <= 0` → защита 24h.
    Best-effort: ошибки `auditCleanup` только логируются. Комментарий над `auditCleanup`
    обновлён («B9: только в тестах; B19: вызывается из runAuditRetention»).
  - **ce991fd** `feat(main): 6.28 (B19.3) — запуск runAuditRetention в main() с graceful shutdown`
    `bgCtx/bgCancel/bgDone` создаются сразу после `SQLite готова`; runner — отдельной
    горутиной. Перед `srv.Shutdown(ctx)` — `bgCancel()` + ожидание `bgDone` до 3s
    (fallback-лог, не блокирует shutdown).
- **UI-smoke (БД /tmp/b19.db, retention=30d, interval=2s):**
  - `audit retention: запущен days=30 interval=2s` сразу после «SQLite готова».
  - Первый `cleanup deleted=0` — сразу; далее — каждые 2s по тикеру.
  - `POST /api/auth/login` (superadmin) → JWT `role=superadmin`.
  - `GET /api/health` до/после 2+ тиков → `{"status":"ok"}`.
  - `kill -TERM $SRV_PID` → `получен сигнал, graceful shutdown signal=terminated`
    + `audit retention: остановлен reason=context canceled`
    + `audit retention: goroutine остановлена`
    + `сервер остановлен` (завершение чистое, без 3s-fallback).
- **Коммит-сообщения:**
  - `feat(env): 6.28 (B19.1) — BOTMAX_AUDIT_RETENTION_DAYS + BOTMAX_AUDIT_CLEANUP_INTERVAL`
  - `feat(audit): 6.28 (B19.2) — фоновый runAuditRetention (ticker + первый прогон сразу)`
  - `feat(main): 6.28 (B19.3) — запуск runAuditRetention в main() с graceful shutdown`
- **Известные ограничения:**
  - Retention работает только для `audit_log`; другие таблицы (refresh-tokens и пр.)
    пока не чистятся — вне скоупа B19.
  - Admin-UI кнопки «прогнать retention сейчас» нет (возможный B19.1 — не запланирован).

<!-- B19_SNAPSHOT_APPLIED -->

### 6.29 (B20) — ЗАКРЫТ
- **Дата:** 2026-10-01
- **Статус:** закрыт (коммиты bf6effb + 060db35 + 708326b + 09948a7 + a4b9afa + 136cfc6)
- **Цель:** impersonation «глазами доктора» — переход `admin_doctors.html` → `doctor_cabinet.html`.
- **Решения по §6.29 (были — открытые вопросы):**
  - **Кто может impersonate:** только `superadmin` (requireRole(RoleSuperadmin)
    в registerImpersonationRoutes). Admin-роль доступа не имеет.
  - **Форма токена:** отдельные claims в access-JWT: `actor_id`, `actor_role`,
    `act_as` (без отдельной сессии). `DoctorID=target`, `Role=RoleDoctor` —
    запрет эскалации уже на уровне токена. Refresh для impersonation
    не выдаётся.
  - **TTL:** 15 минут (константа `ImpersonationTTL = 15 * time.Minute`),
    без env-флага.
  - **Аудит:** `impersonation.start` / `impersonation.stop` в `audit_log`
    (doctor_id=actor, entity=doctor, entity_id=target,
    detail={actor_role, target_role}).
  - **UI:** кнопка 👁 «Войти как» в admin_doctors.html (только superadmin,
    только active-doctor, не сам себя); sticky-баннер + кнопка
    «Выйти из режима» в doctor_cabinet.html (init через
    `GET /api/impersonation/whoami`).
  - **Запрет эскалации:**
    - impersonation-токен всегда `role=doctor` → `requireRole(Admin/Superadmin)`
      автоматически отсекает опасные роуты;
    - `requireNonImpersonated` на `/api/doctors/*` и `/api/audit` —
      defense-in-depth;
    - guard B20.4.1: нельзя имперсонировать superadmin'а (400),
      даже если дёрнуть API напрямую.
- **Файлы:**
  - `backend/auth.go` — `Claims.ActorID/ActorRole/ActAs`, `ImpersonationTTL`,
    `IssueImpersonationToken`, `withActor`, `actorIDFromCtx`, `actorRoleFromCtx`,
    `requireNonImpersonated`, ctx-ключи `ctxKeyActorID/Role`, `authMiddleware`
    прокидывает actor в context.
  - `backend/auth_test.go` — `setupImpersonationEnv`, 5 тестов
    `TestIssueImpersonationToken_*`, 6 тестов B20.2 (withActor / actorFromCtx /
    requireNonImpersonated / интеграционный authMiddleware).
  - `backend/impersonation.go` — endpoints `/api/impersonation/{start,stop,whoami}`,
    типы ответов, guard target≠superadmin.
  - `backend/impersonation_test.go` — 14 тестов (unit + integration).
  - `backend/doctors.go`, `backend/audit.go` — `requireNonImpersonated`
    на CRUD-докторов и audit-list.
  - `backend/main.go` — вызов `registerImpersonationRoutes()`.
  - `admin_doctors.html` — кнопка 👁 + функция `impersonate()`.
  - `doctor_cabinet.html` — sticky-баннер `#impBanner` +
    `initImpersonationBanner()` + `stopImpersonation()`.
- **Env-переменные:** новых нет. TTL — константа в коде.
- **UI-smoke (БД /tmp/b20.db, AUTH_ENFORCE=true):**
  - login superadmin → whoami: `{"impersonating": false}`.
  - start (target=DOC-001): `{ok:true, target:{DOC-001,doctor}, ttl_seconds:900}`
    + `Set-Cookie botmax_access=<imp-token>`.
  - whoami: `{impersonating:true, actor_id:"SUPER-001",
    target_id:"DOC-001", target_name:"Тестовый врач", target_role:"doctor"}`.
  - GET /api/doctors под имперс. → 403.
  - GET /api/audit под имперс. → 403.
  - stop → `{ok:true, restored:true}` + `Set-Cookie botmax_access=<actor-access>`.
  - whoami → `{"impersonating": false}`; GET /api/doctors → 200.
  - audit: `impersonation.start` + `impersonation.stop`
    (doctor_id=SUPER-001, entity_id=DOC-001).
- **Коммиты:**
  - `bf6effb` feat(auth): 6.29 (B20.1) — Claims.ActorID/ActorRole/ActAs + IssueImpersonationToken
  - `060db35` feat(auth): 6.29 (B20.2) — actor в контексте + requireNonImpersonated
  - `708326b` feat(impersonation): 6.29 (B20.3) — endpoints /api/impersonation/{start,stop}
  - `09948a7` feat(impersonation): 6.29 (B20.4) — whoami + requireNonImpersonated + UI (кнопка/баннер)
  - `a4b9afa` fix(impersonation): 6.29 (B20.4.1) — guard: нельзя имперсонировать superadmin
  - `136cfc6` test(impersonation): 6.29 (B20.5) — unit + integration тесты /start,/stop,/whoami
- **Известные ограничения:**
  - Имперсонировать можно только `role=doctor` (active). Admin-цель — UI
    скрывает кнопку, backend не блокирует отдельным guard'ом (только
    superadmin, B20.4.1).
  - TTL фиксирован 15 минут; нет env-флага `BOTMAX_IMPERSONATION_TTL`.
  - Нет серверной инвалидации jti при stop: старый impersonation-access
    живёт до истечения TTL (≤15 мин), но cookie перезаписывается,
    refresh для impersonation не выдаётся.
  - В UI нет countdown-таймера «осталось N минут».
  - Impersonation «протекает» между табами: если открыть doctor_cabinet.html
    в двух табах, stop в одном не закроет баннер в другом до перезагрузки.

<!-- B20_SNAPSHOT_APPLIED -->

### 6.30 (B21) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммиты 5e54c7d + d4a48d8)
- **Цель:** `BOTMAX_IMPERSONATION_TTL` (env-флаг TTL impersonation) + countdown
  до конца impersonation в баннере `doctor_cabinet.html`.
- **Что сделано:**
  - **5e54c7d** `feat(env): 6.30 (B21.1) — BOTMAX_IMPERSONATION_TTL`
    В `Env` добавлено `ImpersonationTTL time.Duration`, заполняется через
    `parseDuration("BOTMAX_IMPERSONATION_TTL", …, 15m)`. В `auth.go` константа
    `ImpersonationTTL` убрана, добавлена функция `impersonationTTL()`
    (читает `currentEnv.ImpersonationTTL`, fallback 15m при `currentEnv == nil`
    для тестов). `IssueImpersonationToken` и `handleImpersonationStart` используют
    её. Тесты `TestValidateEnv` расширены 4 кейсами
    (empty-default / 30m / junk-err / negative-err). `clearEnvVars` сбрасывает
    новый флаг.
  - **d4a48d8** `feat(impersonation): 6.30 (B21.2) — countdown`
    Backend: `ctxKeyTokenExp` + `withTokenExp`/`tokenExpFromCtx` в `auth.go`;
    `authMiddleware` прокидывает `claims.ExpiresAt.Unix()` в context;
    `impersonationWhoamiResp.ExpiresAtUnix` заполняется в `/whoami`.
    Frontend: в баннер добавлен `<span id="impRemaining">` (⏱ mm:ss),
    `startImpersonationCountdown(expUnix)` — `setInterval(1000)`, при `left<=0`
    показывает «истёк» и через 2с делает `location.reload()`. Заодно почищен
    дубль `display:none` в inline-стиле баннера.
- **Файлы:**
  - `backend/env.go` — `Env.ImpersonationTTL` + `parseDuration(...)` в `validateEnv`.
  - `backend/env_test.go` — 4 кейса `impersonation-ttl-*` + `clearEnvVars`.
  - `backend/auth.go` — `impersonationTTL()` вместо константы; `ctxKeyTokenExp` +
    `withTokenExp`/`tokenExpFromCtx`; `authMiddleware` кладёт exp в context.
  - `backend/impersonation.go` — `ExpiresAtUnix` в `whoami`-ответе.
  - `backend/impersonation_test.go` — `TestTokenExpFromCtx` + расширенный
    `TestHandleImpersonationWhoami_Impersonating_NoDB` (проверка `ExpiresAtUnix`).
  - `doctor_cabinet.html` — `#impRemaining` в баннере +
    `startImpersonationCountdown`.
- **Env-переменные:**
  - `BOTMAX_IMPERSONATION_TTL` — Go duration > 0, default `15m`
    (по образцу `BOTMAX_AUDIT_CLEANUP_INTERVAL`).
- **UI-smoke (БД /tmp/b21.db, AUTH_ENFORCE=true, IMPERSONATION_TTL=2m):**
  - login superadmin → start(target=DOC-001) → `"ttl_seconds": 120`.
  - whoami → `{impersonating:true, actor_id:"SUPER-001", target_id:"DOC-001",
    target_name:"Тестовый врач", target_role:"doctor", expires_at_unix:1790924463}`.
  - `diff = exp - now = 120` — env-флаг и countdown синхронны.
- **Коммиты:**
  - `5e54c7d` feat(env): 6.30 (B21.1) — BOTMAX_IMPERSONATION_TTL (env-флаг TTL impersonation)
  - `d4a48d8` feat(impersonation): 6.30 (B21.2) — countdown до конца impersonation в баннере
- **Известные ограничения:**
  - Countdown в баннере показывает остаток корректно, но при истечении TTL
    страница делает `location.reload()` через 2с — это UX-компромисс (лучше
    иметь auto-stop, но авто-stop требовал бы держать таймер даже в фоне).
  - Impersonation не синхронизирован между табами: если открыть doctor_cabinet.html
    в двух табах, stop/reload в одном не закроет баннер в другом.
  - Запрет имперсонировать admin'а всё ещё на UI-уровне (backend-гвард только
    для superadmin, B20.4.1).

<!-- B21_SNAPSHOT_APPLIED -->

### 6.31 (B22) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммиты 26a9f3e + 97cd63b)
- **Цель:** доработки impersonation: guard на admin + auto-stop по истечении TTL.
- **Что сделано:**
  - **26a9f3e** `fix(impersonation): 6.31 (B22.1) — guard: нельзя имперсонировать admin`
    В `handleImpersonationStart` добавлен guard `target.Role == RoleAdmin` → 400
    `{"error":"cannot impersonate admin"}`. Симметрично B20.4.1 (superadmin).
    UI и так скрывал кнопку 👁 для admin-целей — backend дублирует
    (defense-in-depth). Тест `TestHandleImpersonationStart_TargetAdmin` добавлен
    (проверяет ПОЛНУЮ фразу, чтобы не спутать с сообщением для superadmin).
  - **97cd63b** `feat(impersonation): 6.31 (B22.2) — auto-stop за 30с до истечения TTL`
    В `doctor_cabinet.html` countdown за `AUTO_STOP_LEAD_SECONDS = 30` секунд
    до конца TTL вызывает `autoStopImpersonation()`:
    `POST /api/impersonation/stop` (пока access валиден) → redirect в
    `admin_doctors.html`. Fallback при ошибке `/stop` — `window.location.reload()`
    (сработает refresh-fallback, см. ниже). Защита от повторного срабатывания —
    флаг `_impAutoStopFired`.
- **Файлы:**
  - `backend/impersonation.go` — guard `RoleAdmin` (3 строки).
  - `backend/impersonation_test.go` — `TestHandleImpersonationStart_TargetAdmin`.
  - `doctor_cabinet.html` — `AUTO_STOP_LEAD_SECONDS`, `_impAutoStopFired`,
    `autoStopImpersonation()`.
- **UI-smoke (БД /tmp/b22.db, AUTH_ENFORCE=true):**
  - B22.1: `start(target=ADMIN-001)` → 400 `{"error":"cannot impersonate admin"}`.
  - B22.2: `IMPERSONATION_TTL=45s` → start → `ttl_seconds:45`,
    `expires_at_unix ≈ now+45`; через 20с stop → `{ok:true, restored:true}`;
    whoami → `impersonating:false`; `/api/doctors` → 200 (superadmin восстановлен).
- **Коммиты:**
  - `26a9f3e` fix(impersonation): 6.31 (B22.1) — guard: нельзя имперсонировать admin
  - `97cd63b` feat(impersonation): 6.31 (B22.2) — auto-stop impersonation за 30с до истечения TTL
- **Признано ненужным:**
  - **Refresh-fallback для `/stop` (B22.3):** закрыт как «не нужен». Обоснование:
    refresh-токен в cookie остаётся actor'а (в `/start` его не трогаем). Если
    impersonation-access протухнет, `authFetch` словит 401 → `/api/auth/refresh`
    → сервер выдаст новый access с `role=superadmin` → юзер вернётся в нормальную
    сессию автоматически. Auto-stop за 30с до истечения покрывает UI-сценарий
    без ожидания 401.
- **Оставшиеся кандидаты (→ §6.32):**
  - UI-кнопка «прогнать retention сейчас» в админке (§6.28).
  - Retention для `refresh_tokens` (§6.28).
  - Межтабовый sync impersonation (BroadcastChannel / SSE).

<!-- B22_SNAPSHOT_APPLIED -->

### 6.32 (B23) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммиты 3c8bef6 + 98a13e3)
- **Цель:** закрыть retention-бэклог §6.28: retention для `refresh_tokens` +
  UI-кнопка ручного прогона в админке.
- **Что сделано:**
  - **3c8bef6** `feat(retention): 6.32 (B23.1) — BOTMAX_REFRESH_RETENTION_DAYS`
    В `Env` добавлено `RefreshRetentionDays int`, заполняется через
    `parseNonNegativeInt("BOTMAX_REFRESH_RETENTION_DAYS", …, 0)`.
    В `refresh.go` — `refreshCleanup(db, days)` (симметрично `auditCleanup`,
    cutoff — `time.Now().AddDate(0, 0, -days)` в формате `2006-01-02 15:04:05`).
    В `audit.go` — общий `runRetention(ctx, db, auditDays, refreshDays, interval)`:
    в одном тикере чистит обе таблицы; каждый cleanup независим (days<=0 → skip).
    `runAuditRetention` оставлена обёрткой для обратной совместимости B19.
    `main.go` — вызов `runRetention` с обоими флагами. `AuditCleanupInterval`
    теперь общий для audit + refresh.
    Тесты: `TestRefreshCleanup_DeletesOld`, `TestRefreshCleanup_KeepsFresh`, `TestRefreshCleanup_ZeroDays_Error`;
    `TestValidateEnv` — 4 кейса `refresh-retention-*`; `clearEnvVars` сбрасывает флаг.
  - **98a13e3** `feat(retention): 6.32 (B23.2) — POST /api/audit/retention/run`
    Новый handler `handleAuditRetentionRun`: POST, admin/superadmin +
    `requireNonImpersonated`, запускает auditCleanup + refreshCleanup **сразу**
    (по параметрам из env). 400, если оба дня == 0. Пишет audit-запись
    `action="retention.run", entity="system", detail={audit_deleted, refresh_deleted}`.
    UI: кнопка «🧹 Прогнать retention» в `admin_audit.html` + функция
    `runRetentionNow()` (confirm → POST → alert с результатом → reloadAudit).
    Тесты: `TestHandleAuditRetentionRun_Disabled`, `_MethodNotAllowed`, `_AuditOnly`, `_Success_Both`.
- **Файлы:**
  - `backend/env.go` — `Env.RefreshRetentionDays`.
  - `backend/env_test.go` — 4 кейса + сброс в `clearEnvVars`.
  - `backend/refresh.go` — `refreshCleanup`.
  - `backend/refresh_test.go` — 3 теста `TestRefreshCleanup_*`.
  - `backend/audit.go` — `runRetention` + обёртка + `handleAuditRetentionRun` +
    типы `retentionScopeResp`/`retentionRunResp`.
  - `backend/audit_test.go` — 4 теста `TestHandleAuditRetentionRun_*`.
  - `backend/main.go` — вызов `runRetention`.
  - `admin_audit.html` — кнопка + `runRetentionNow()`.
- **Env-переменные:**
  - `BOTMAX_REFRESH_RETENTION_DAYS` — int >= 0, default `0` (retention выключен).
  - `BOTMAX_AUDIT_CLEANUP_INTERVAL` теперь общий тикер для audit + refresh.
- **UI-smoke (БД /tmp/b23.db, AUTH_ENFORCE=true):**
  - B23.1: `AUDIT=30 REFRESH=1 INTERVAL=2s` → в логе `retention: запущен`,
    первый cleanup сразу, затем по тикеру; `old` (10 дней) удалён, `fresh`
    (1 час) сохранён.
  - B23.2: `POST /api/audit/retention/run` →
    `{ok:true, audit:{days:30,deleted:1}, refresh:{days:1,deleted:1}}`;
    audit: `retention.run, entity=system, detail={audit_deleted:1, refresh_deleted:1}`.
- **Коммиты:**
  - `3c8bef6` feat(retention): 6.32 (B23.1) — BOTMAX_REFRESH_RETENTION_DAYS + общий runRetention (audit + refresh)
  - `98a13e3` feat(retention): 6.32 (B23.2) — POST /api/audit/retention/run + кнопка в admin_audit.html
- **Известные ограничения:**
  - `SaveRefreshJTI` продолжает делать lazy-cleanup `< datetime('now')` —
    он жёстче retention'а (удаляет всё протухшее при сохранении).
    Retention нужен для cold-path: если логинов нет, lazy-cleanup не сработает.
  - Кнопка «Прогнать retention» синхронная — при больших объёмах запрос
    может висеть. Для текущих объёмов не проблема.
- **Оставшиеся кандидаты (→ §6.33):**
  - Межтабовый sync impersonation (BroadcastChannel / SSE).

<!-- B23_SNAPSHOT_APPLIED -->

### 6.33 (B24) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммит 9b8ff4a)
- **Цель:** межтабовый sync impersonation через BroadcastChannel.
- **Что сделано:**
  - **9b8ff4a** `feat(impersonation): 6.33 (B24.1) — межтабовый sync через BroadcastChannel`
    В `doctor_cabinet.html` — канал `botmax-impersonation` (BroadcastChannel
    без fallback, но с проверкой `typeof BroadcastChannel !== 'undefined'`).
    `stopImpersonation` / `autoStopImpersonation` при выходе (в `then` и `catch`)
    отправляют `{type:'stopped'}`. `onmessage` в других табах: если баннер
    виден — делает `window.location.reload()` (после reload `/whoami` вернёт
    `impersonating:false`, либо refresh превратит сессию в actor'а).
    В `admin_doctors.html` при успешном `/start` — broadcast
    `{type:'started', target_id}` (для будущего; сейчас никто не слушает).
- **Файлы:**
  - `doctor_cabinet.html` — `_impChannel`, `_impBroadcastStopped()`, `onmessage`.
  - `admin_doctors.html` — broadcast `started` при успешном `/start`.
- **UI-smoke (ручной, БД /tmp/b24.db, AUTH_ENFORCE=true):**
  - Регрессия B20–B23 через curl: login → start → whoami → stop — все ответы
    корректные (`ttl_seconds:900`, `impersonating:true`, `restored:true`).
  - Межтабовый sync — ручная проверка в браузере: два таба `doctor_cabinet.html`,
    stop в одном → reload во втором; в консоли `impersonation: stop в другом табе, reload`.
- **Коммиты:**
  - `9b8ff4a` feat(impersonation): 6.33 (B24.1) — межтабовый sync через BroadcastChannel
- **Признано сделанным ранее (вне скоупа B24):**
  - `.gitignore` уже содержит `*.bak` и `*.bak.*` (секция «Мусор»).
  - 92 файла `.bak.*` на диске, но **не tracked** (`git ls-files | grep .bak.` = 0).
  - `git rm --cached` не требуется.

<!-- B24_SNAPSHOT_APPLIED -->

### 6.34 (B25) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммиты 6b6f694 + e43c5c5)
- **Цель:** retention status + UI-счётчик (для ops-мониторинга).
- **Что сделано:**
  - **6b6f694** `feat(retention): 6.34 (B25.1) — retention status + расширение /api/health`
    Новый файл `backend/retention.go` (перенос `runRetention`/`runAuditRetention`
    из `audit.go`):
    - `RetentionStatus{LastRunAt, LastError, AuditDeleted, RefreshDeleted, HasRun}`
      + mutex-guarded глобал `retentionStatusVal`;
    - `updateRetentionStatusSuccess`, `updateRetentionStatusError` (last_error
      сбрасывается при успехе);
    - `retentionHealthy(auditDays, refreshDays, interval)` — false при
      `!HasRun`, `LastError != ""`, `last_run_at > interval*2`; true при
      выключенной фиче;
    - `runRetention.runOnce` обновляет статус после каждого прогона.
    `/api/health` вынесен в `handleHealth` в `main.go`: если retention
    включён — добавляется секция `retention` c `enabled/healthy/last_run_at/
    seconds_since_last_run/audit_deleted/refresh_deleted/last_error/audit_days/
    refresh_days`. При выключенной фиче секции нет.
    Тесты: `TestRetentionStatus_Empty`, `_Disabled`, `_UpdatedOnRun`,
    `_UnhealthyAfterInterval`, `_ErrorClearedOnSuccess`,
    `TestHealth_NoRetentionSection`, `TestHealth_RetentionSection`.
  - **e43c5c5** `feat(retention): 6.34 (B25.2) — UI-статус retention в admin_audit.html`
    В `admin_audit.html` рядом с кнопкой 🧹 добавлен `<span id="retentionStatus">`
    с компактным статусом: `🧹 retention: 15s назад · audit 0 / refresh 5 · ✅`.
    `formatRelativeTime(iso)` — s/m/h/d; `refreshRetentionStatus()` читает
    `/api/health`, span скрывается при выключенной фиче; цвет краснеет при
    `healthy=false`, в `title` — текст ошибки. Обновление: при загрузке,
    после ручного прогона, каждые 30с (`setInterval`).
- **Файлы:**
  - `backend/retention.go` (новый) — статус + перенос runner'а.
  - `backend/retention_test.go` (новый) — 7 тестов.
  - `backend/audit.go` — `runRetention`/`runAuditRetention` удалены (перенос).
  - `backend/main.go` — `handleHealth` (секция retention), /api/health вызывает её.
  - `admin_audit.html` — span + `refreshRetentionStatus()` + `formatRelativeTime()`.
- **Env-переменные:** новых нет.
- **UI-smoke (БД /tmp/b25.db, AUTH_ENFORCE=true):**
  - без retention (`BOTMAX_AUDIT_RETENTION_DAYS` не задан) → `/api/health` = `{"status":"ok"}`; span скрыт.
  - с retention (`AUDIT=30 REFRESH=1 INTERVAL=2s`) → `/api/health` содержит
    секцию `retention` с `healthy:true`, `last_run_at` обновляется каждые 2с.
- **Коммиты:**
  - `6b6f694` feat(retention): 6.34 (B25.1) — retention status + расширение /api/health
  - `e43c5c5` feat(retention): 6.34 (B25.2) — UI-статус retention в admin_audit.html
- **Признано ненужным (обоснование):**
  - **Retention для `patients`:** НЕ делаем. `patients` — бизнес-данные клиники
    (медкарты), retention здесь опасен: молчаливое удаление через N дней
    несовместимо с требованиями хранения медкарт (152-ФЗ/приказы Минздрава).
    Если когда-нибудь понадобится «чистить архивных пациентов» — это должна
    быть отдельная явная операция (soft-delete + ручная админ-кнопка с
    подтверждением), а не фоновой runner.

<!-- B25_SNAPSHOT_APPLIED -->

### 6.35 (B26) — TODO (уточнить)
- **Статус:** запланирован
- **Цель:** TBD. Активных кандидатов в бэклоге нет: §6.28 (retention),
  §6.31 (impersonation hardening), §6.32 (retention UI), §6.33 (межтабовый
  sync), §6.34 (retention status + UI) — все закрыты. Возможные направления
  на будущее:
  1. `/api/metrics` в Prometheus-формате (для prod-мониторинга).
  2. Алерты/уведомления при `retention.healthy=false` (Slack/webhook).
  3. Retention для `refresh_tokens.revoked_at` — сейчас чистим только по
     `expires_at`, но revoked-записи живут до `expires_at`. Можно удалять
     revoked раньше.
  4. Экспорт audit-log в CSV/JSONL для SIEM.

<!-- B26_SNAPSHOT_PENDING -->

---

## Правила
- 1 подпатч = 1 коммит (или несколько — если подпатч намеренно разбит на части, как 6.10).
- Перед завершением подпатча — запись в этом файле: что сделано + следующий номер.
- Никогда не коммитить без зелёных `go vet ./...`, `go build ./backend`, `go test ./backend/...`.
- Никогда не пушить в prod без проверки на localhost.

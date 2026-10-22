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

### 6.35 (B26) — ЗАКРЫТ
- **Дата:** 2026-10-02
- **Статус:** закрыт (коммит 9f3e46d)
- **Цель:** `/api/metrics` в Prometheus-формате — prod-готовность
  для мониторинга.
- **Что сделано:**
  - **9f3e46d** `feat(metrics): 6.35 (B26.1) — Prometheus-совместимый /api/metrics`
    Новый файл `backend/metrics.go` — ручная генерация формата
    `text/plain; version=0.0.4` (без prometheus/client_golang — не тянем
    зависимость). Метрики:
    - `botmax_retention_enabled` (gauge 1|0);
    - `botmax_retention_healthy` (gauge 1|0);
    - `botmax_retention_last_run_timestamp_seconds` (gauge, unix; 0 если не было);
    - `botmax_retention_deleted_total{table="audit|refresh"}` (counter);
    - `botmax_retention_last_error` (gauge 1|0).
    Endpoint публичный (как `/api/health`): добавлен в `isAuthWhitelist`
    в `auth.go`. Регистрация — `registerMetricsRoutes()` в `main.go` рядом
    с `/api/health`. Тесты: `TestMetrics_{Disabled,EnabledNotRun,AfterRun,
    Error,MethodNotAllowed,ContentType}`.
- **Файлы:**
  - `backend/metrics.go` (новый) — handler + ручной exposition.
  - `backend/metrics_test.go` (новый) — 6 тестов.
  - `backend/main.go` — вызов `registerMetricsRoutes()`.
  - `backend/auth.go` — `/api/metrics` в whitelist.
- **Env-переменные:** новых нет.
- **UI-smoke (БД /tmp/b26.db, AUTH_ENFORCE=true):**
  - без retention → все метрики с нулями: `enabled 0`, `healthy 0`, `last_run 0`.
  - с retention (`AUDIT=30 REFRESH=1 INTERVAL=2s`) → `enabled 1`, `healthy 1`,
    `last_run ~now`, счётчики `0/0`, `last_error 0`.
  - `Content-Type: text/plain; version=0.0.4; charset=utf-8` (совместим с Prometheus).
- **Коммиты:**
  - `9f3e46d` feat(metrics): 6.35 (B26.1) — Prometheus-совместимый /api/metrics
- **Признано ненужным (обоснование):**
  - **Алерты при `retention.healthy=false` (Slack/webhook):** НЕ делаем. Правильный
    путь — Alertmanager поверх `/api/metrics`, а не собственный алертер в приложении.
    Изобретение своей системы алертов — антипаттерн; в prod обычно используют
    внешние (Prometheus + Alertmanager, Uptime Kuma и т.п.).
  - **Retention для `refresh_tokens.revoked_at`:** НЕ делаем сейчас. Выгода
    маргинальная: `IsRefreshRevoked("jti")` и так возвращает `true` для
    неизвестного jti — то есть удаление revoked-записи не меняет поведение.
    При текущих объёмах (десятки-сотни токенов) экономия места ничтожна.
    Вернуться, если появятся реальные объёмы.
  - **Экспорт audit-log в CSV/JSONL для SIEM:** НЕ делаем. Для BOT_MAX
    (маленькая клиника) неактуально — `/api/audit` с фильтрами покрывает
    ручной анализ. Экспорт в SIEM — задача интеграции на стороне клиента,
    а не самого приложения.

<!-- B26_SNAPSHOT_APPLIED -->

### 6.36 (B27) — ЗАКРЫТ
- **Дата:** 2026-10-03
- **Статус:** закрыт (коммиты d690449 + f6c75db + 944fc45 + b7b5834 + 6fadac0)
- **Цель:** password reset + must_change_password + brute-force lock +
  роль registrar (первая волна МИС-плана §6.37).
- **Что сделано:**
  - **d690449** `feat(auth): 6.36 (B27.1) — schema password_reset_tokens + поля doctors + роль registrar`
    Схема: таблица `password_reset_tokens` (id, token_hash, doctor_id,
    created_at, expires_at, used_at) + 2 индекса. Миграции через
    `ensureColumn(db, ...)` (PRAGMA table_info + ALTER TABLE — идемпотентно):
    doctors.must_change_password, password_changed_at, failed_login_count,
    locked_until. Роль `RoleRegistrar = "registrar"` в enum.
  - **f6c75db** `feat(auth): 6.36 (B27.2) — store password reset + brute-force lock`
    Store-функции (password_reset.go): CreatePasswordResetToken,
    GetPasswordResetToken, ConsumePasswordResetToken, DeleteExpired,
    UpdatePasswordHash, SetMustChangePassword, IncrementFailedLogin,
    LockDoctor, ResetFailedLogin, IsDoctorLocked.
    Константы: PasswordResetTokenTTL=1h, MaxFailedLogins=5, DoctorLockDuration=15m.
  - **944fc45** `feat(auth): 6.36 (B27.3a) — API password-reset request/confirm + fix: getDoctorByEmail case-insensitive`
    Endpoints: POST /api/auth/password-reset/request (всегда 200 —
    anti-enumeration; dev возвращает token в JSON), POST
    /api/auth/password-reset/confirm (token+new_password, min 8).
    При успехе — revoke всех refresh-токенов доктора.
    Хелперы: GenerateResetToken (32 байта hex), hashResetToken (sha256),
    RevokeAllRefreshForDoctor. **Fix:** getDoctorByEmail теперь
    `WHERE LOWER(email) = LOWER(?)`.
  - **b7b5834** `feat(auth): 6.36 (B27.4) — middleware must_change_password + change-password + admin reset`
    Middleware `requirePasswordChanged` — 403 при must_change_password=1,
    кроме whitelist (change-password, logout, password-reset/*, isAuthWhitelist).
    POST /api/auth/change-password (self) — old+new, снимает флаг,
    НЕ отзывает refresh. POST /api/doctors/{id}/reset-password
    (admin/superadmin) — GenerateTempPassword (base32, 16 символов),
    ставит must_change_password=1, revoke refresh, защита от сброса
    superadmin'а admin'ом. Middleware chain:
    `recover(auth(requirePasswordChanged(mux)))`.
  - **6fadac0** `feat(auth): 6.36 (B27.5) — UI password reset: forgot/reset/change + кнопка 🔑 в админке`
    Новые страницы: `forgot_password.html` (запрос reset-ссылки),
    `reset_password.html?token=...` (установка нового пароля),
    `change_password.html` (принудительная смена при must_change_password).
    `login.html` — ссылка «Забыли пароль?».
    `admin_doctors.html` — кнопка 🔑 + `resetDoctorPassword()`
    (показ temp с копированием в буфер).
    `doctor_cabinet.html` — перехват 403 must_change_password → редирект
    на change_password.html.
- **Файлы:**
  - `backend/db.go` — password_reset_tokens + ensureColumn + миграции.
  - `backend/db_test.go` (новый) — 3 теста схемы.
  - `backend/auth.go` — RoleRegistrar, handlePasswordResetRequest,
    handlePasswordResetConfirm, handleChangePassword,
    requirePasswordChanged, isPasswordChangeWhitelist.
  - `backend/auth_test.go` — TestRole_RegistrarValid.
  - `backend/password_reset.go` (новый) — store + helpers.
  - `backend/password_reset_test.go` (новый) — 18 тестов.
  - `backend/doctors.go` — handleDoctorResetPassword + GenerateTempPassword.
  - `backend/main.go` — middleware chain.
  - `forgot_password.html`, `reset_password.html`, `change_password.html` (новые).
  - `login.html`, `admin_doctors.html`, `doctor_cabinet.html` — правки.
- **Env-переменные:** новых нет.
- **UI-smoke (БД /tmp/b27.db, AUTH_ENFORCE=true):**
  - POST /api/auth/password-reset/request → {ok:true, token:"..."} (dev).
  - POST /api/auth/password-reset/confirm → {ok:true}.
  - login superadmin с новым паролем → JWT.
  - admin reset DOC-001 → {ok:true, temporary_password:"..."}.
  - login DOC-001 с temp → JWT → /api/patients = 403 must_change_password.
  - change-password → {ok:true} → /api/patients = 200.
  - Статика: forgot/reset/change_password.html → 200.
- **Коммиты:**
  - `d690449` feat(auth): 6.36 (B27.1) — schema password_reset_tokens + поля doctors + роль registrar
  - `f6c75db` feat(auth): 6.36 (B27.2) — store password reset + brute-force lock
  - `944fc45` feat(auth): 6.36 (B27.3a) — API password-reset request/confirm + fix: getDoctorByEmail case-insensitive
  - `b7b5834` feat(auth): 6.36 (B27.4) — middleware must_change_password + change-password + admin reset
  - `6fadac0` feat(auth): 6.36 (B27.5) — UI password reset: forgot/reset/change + кнопка 🔑 в админке
- **Известные ограничения:**
  - SMTP не подключён: в prod reset-ссылка не отправляется (только лог/audit).
    В dev токен возвращается в JSON для отладки.
  - Brute-force lock: счётчик `failed_login_count` пока НЕ увеличивается
    в `handleAuthLogin` — store-функции готовы, но интеграция в login
    запланирована отдельным подпатчем (B27.4.1).
  - Роль `registrar` объявлена, но пока не используется (ждёт модуля
    sick_leave, B40).

<!-- B27_SNAPSHOT_APPLIED -->

### 6.43 (B29.6) — Backlog: UI-feedback из смоука B29.5

**Статус:** открыто, не начато.
**Источник:** ручной UI-смоук B29.5 (2026-10-13, DOC-001, `/tmp/b295_smoke.db`).
**Приоритет:** низкий (UX, не блокирует закрытие B29.5).

**Коммиты:** —

**Пункты:**

1. **Локализация опций фильтров `kind`/`status` на русский.**
   - `kind`: consultation→Консультация, study→Исследование,
     procedure→Процедура, lab→Лаборатория, followup→Повторный приём.
   - `status`: draft→Черновик, finished→Завершён.
   - Значения в API и URL остаются английскими; перевод только в `<option>`
     (label) и в badge статуса.
   - Файлы: `patient_card.html` (разметка фильтров, ~3300–3320),
     `doctor_cabinet.html` (`renderEncountersTable`).
   - Реализация: словари `KIND_RU`/`STATUS_RU` в inline-скрипте страницы.

2. **Специальность → выпадающий список** вместо `<input type="text">`.
   - Сейчас: `<input type="text" id="encFilterSpecialty" placeholder="например, lor">`.
   - Нужно: `<select id="encFilterSpecialty">` с опциями специальностей.
   - Источник списка: `/api/specialties` (проверить наличие эндпоинта)
     или статический словарь на фронте.

3. **Календарь (`<input type="date">`) — контрастнее.**
   - Иконки выбора даты плохо видны на тёмном фоне.
   - Правка CSS: `color-scheme: dark` на `input[type="date"]` либо
     `filter: invert(1)` на `::-webkit-calendar-picker-indicator`.

4. **Сортировка приёмов ASC/DESC.**
   - Сейчас на UI жёсткий DESC по `started_at` (`renderEncountersTable`).
   - Нужен переключатель «от последних к первым / от первых к последним».
   - Backend: параметр `?order=asc|desc` в `parseEncounterFilter`
     (`encounters.go`), проброс в SQL `ORDER BY started_at {ASC|DESC}`.
   - Frontend: select `encFilterOrder` в панели фильтров, проброс через
     `getFilters()`, добавление `order` в `urlKeys` (`BotMaxPager.create`).
   - Оценка: ~2 подпатча (backend + frontend).

**Оценка волны:** ~1 сессия (4 подпатча).

<!-- B29_6_BACKLOG_APPLIED -->

### 6.58 (B30) — ПЛАН (не начат, checkpoint 2026-10-14)
- **Тема:** синхронизация patient → doctor через backend, с consent-механизмом.
- **Проблема (найдена при смоке B29.8):**
  - `patient_cabinet_full.html` не делает **ни одного** API-запроса
    к `/api/patients/*`. Всё, что вводит пациент (vitals, симптомы,
    аллергии, препараты, вакцинации, диагнозы, файлы, ФИО, рост/вес,
    дата рождения), живёт только в `localStorage` — см.
    `saveToLocalStorage()` (`patient_cabinet_full.html:827`),
    `loadFromLocalStorage()` (`:835`).
  - `patientId = patientData.patientId || 'PAT-001'` (`:15`) — литерал,
    не связан с БД; аутентификации пациента нет.
  - Врач в `patient_card.html` видит **только** то, что сам ввёл через
    `PUT /api/patients/{id}/vitals` (`:1474`); каналы не пересекаются.
  - На бэкенде: `vitals` / `vitals_history` есть (`backend/db.go:125,135`),
    обработчики `/{id}/vitals` есть (`backend/patients.go:122,245`);
    таблиц под symptoms/allergies/vaccinations/medications/diagnoses
    **нет**.
  - `clinics` **уже есть** (`backend/db.go:37`): `doctors.clinic_id` и
    `patients.clinic_id` — FK на `clinics(id)` (`:50,60,69,88`),
    `vitals_history.clinic_id NOT NULL` (`:158,164`). Отдельной
    many-to-many `doctor_clinics` нет — у врача **один** `clinic_id`
    (1:N). Для consent на клинику используем существующий FK.
- **Решения (зафиксированы):**
  - Логин пациента: **email + пароль** (`POST /api/patient-auth/*`,
    отдельная таблица `patient_accounts`, привязка к `patients.id`).
  - Consent: **два типа** — `grantee_type ∈ {'doctor','clinic'}`.
    `grantee_id` для `'doctor'` — `doctors.id`, для `'clinic'` —
    `clinics.id` (взять из `patients.clinic_id`). Пациент выбирает,
    кому выдаёт. Модель `consents(id, patient_id, grantee_type,
    grantee_id, scopes, granted, expires, revoked)` + audit log
    `consents_audit`.
  - Данные: **upsert по `id`** (не «только новое»). У всех сущностей
    в `patientData` уже есть `id` (`Date.now()`) — используем как
    якорь. Добавляем `updated_at` и `deleted_at` (soft delete),
    конфликты — server-wins + уведомление пациенту.
  - Миграция localStorage → backend: **одноразовая при первом логине**,
    флаг `migrated_at` в localStorage. Существующие данные **не теряются**.
  - Курсовые препараты: **врач видит и может редактировать** —
    полноценный CRUD у врача, без read-only.
  - Файлы: **BLOB в SQLite, лимит 5 MB/файл** (пересмотр на этапе 3,
    при превышении — FS или S3).
- **Объём:** ~18–25 подпатчей, 3 этапа:
  - **Этап 1 (MVP, ~5–6 подпатчей):** `patient_accounts` +
    `consents` + `consents_audit` (миграция); `POST /api/patient-auth/*`;
    запрос/одобрение/отзыв consent (эндпоинты + UI у пациента + UI
    у врача); запись vitals/birthdate/height/weight пациентом на сервер;
    индикатор sync в карте врача. Результат: пациент записал
    температуру → врач видит её.
  - **Этап 2 (~8–10 подпатчей):** symptoms, allergies, vaccinations,
    diagnoses + scopes в consent (разрешение по категориям) + UI
    списков в карте врача.
  - **Этап 3 (~5–7 подпатчей):** constantMedications, courseMedications
    (CRUD у врача), files (BLOB) + file storage.
- **Статус (2026-10-22):** этап 1 закрыт (B30.1–B30.8), этап 2 **закрыт
  полностью**: B30.9a–d (`1238b44`), B30.9e — UI records lists в
  `patient_card.html` (`3e1251d`), B30.9f — docs + smoke + пуш.
  Дальше — этап 3 §6.58: `constantMedications` / `courseMedications` /
  `files` (BLOB).


- **Этап 2 (§6.58) — B30.9a–e закрыто (2026-10-22, коммиты `1238b44` + feat(b30.9e)):**
  - **B30.9a:** таблицы `symptoms` / `allergies` / `vaccinations` /
    `diagnoses` — `id TEXT PK`, `patient_id`/`clinic_id` FK,
    `created_at`/`updated_at`, `deleted_at`, индексы
    `idx_<kind>_patient` + `idx_<kind>_alive`.
  - **B30.9b:** белый список `scope` (`vitals_read` + `symptoms_read` /
    `allergies_read` / `vaccinations_read` / `diagnoses_read`),
    `normalizeConsentScope`, защита в `upsertConsentRequest` +
    5 тестов (`consent_scope_b30_9b_test.go`).
  - **B30.9c:** `/api/patient/records/{kind}` — `GET` (живые),
    `POST` (upsert по `id`, server-wins, `created_at` не меняется),
    `DELETE` (soft delete), под `patientAuthMiddleware`;
    `clinic_id` = снимок `patients.clinic_id`. Тесты: 401/upsert/
    soft-delete/чужой → 404 (`patient_records_test.go`).
  - **B30.9d:** `/api/patients/{id}/{kind}` — врач;
    `findApprovedConsent(pid, doctorID, kind+"_read")`,
    401 без doctorID / 403 `consent_required` / 403 `consent_expired` /
    500 `consent_bad_expires`. Тесты: 401/403/чужой scope/OK/удалённая
    скрыта (`patient_records_doctor_test.go`).
  - **B30.9e:** UI records lists в `patient_card.html` — 4 секции
    «(БД)» после «Рекомендаций» внутри контейнера карты:
    `recordsSymptomsContent` / `recordsAllergiesContent` /
    `recordsVaccinationsContent` / `recordsDiagnosesContent`
    (заголовки «Симптомы/Аллергии/Вакцинации/Диагнозы (БД)»).
    `RECORD_KINDS = ['symptoms','allergies','vaccinations','diagnoses']`,
    `loadPatientRecords()` вызывается из `loadPatient()` (server-путь)
    после `maybeMigratePatientData()`. Чтение —
    `GET /api/patients/{id}/{kind}` через существующий `apiRequest`
    (Bearer, refresh 401 — внутри `BotMaxAuth.authFetch`). Sync-бейдж
    `#recordsSync_<kind>`: `загрузка…` / `обновлено HH:MM` (+ `title`
    с полным `toLocaleString()`). При 403 `consent_required` /
    `consent_expired` — плашка «Нужно согласие пациента…»; прочие
    ошибки — `.records-error`. Пусто → «Пока нет записей».
  - Проверки: `go build` / `go vet` / `gofmt -l` — чисто;
    UI-smoke: `grep -c 'class="records-list"' patient_card.html` → 4;
    `RECORD_KINDS` — все 4; `badge.title` в `loadRecordsOne`.
    `go test ./backend/` — `ok 7.961s`.
- **B30.9e (UI списков в `patient_card.html`) — следующий.**
  Крупный UI: 4 секции (symptoms/allergies/vaccinations/diagnoses),
  чтение через `GET /api/patients/{id}/{kind}` (B30.9d),
  индикатор sync. **Вынесен в новый чат** со свежим слепком.
- **B30.9f (docs + smoke + пуш)** — после B30.9e.
- **Конвенция (2026-10-22):** все изменения в файлах вносить **через
  bash** (`cat >`, `sed -i`, `python3`-патчи), не ручной вставкой
  в редакторе. Коммит/пуш — тоже командами, с `git diff --stat`
  после каждого шага.

### 6.57 (B29.8) — ЗАКРЫТО (2026-10-14)
- **Тема:** backlog §6.56 — infantGrowth null, temp regex, time sync fallback.
- **Коммиты:**
  - `ea28259` fix(ui): B29.8.1 — guard в updateInfantGrowthTable (backlog §6.56)
  - `05a2775` fix(ui): B29.8.2 — showTemperatureChart: regex [+-], запятые→точки, guard canvas
  - `1e1b7c0` fix(ui): B29.8.3 — syncTimeWithInternet: guard statusDiv, response.ok, fallback на локальные часы
- **Файлы:**
  - `patient_cabinet_full.html`
    - `updateInfantGrowthTable()` — ранний `return` при отсутствии
      `#growthTable tbody`; однократный `console.warn('[infantGrowth] …')`
      через `window.__infantGrowthWarned`. До фикса: `null.innerHTML` валил
      весь `updateAll()`, когда таблица роста не отрендерена.
    - `showTemperatureChart()` — regex `[+\-–]?` (опциональный знак),
      нормализация всех запятых → точки, отсев `NaN` через `isFinite`,
      guard `#temperatureChart` canvas перед `getContext`. До фикса:
      `+36.6` / `36,6°C` тихо давали `NaN` на оси Y, отсутствующий
      canvas — падение на `.getContext` от `null`.
    - `syncTimeWithInternet()` — guard `#syncStatus` во всех трёх
      обращениях, `response.ok`-проверка, флаг `__timeSource`
      (`"local"` / `"remote"`), фолбэк на `new Date()` в `catch`,
      однократный `console.warn('[time] worldtimeapi недоступен …')`.
      До фикса: `ERR_CONNECTION_RESET` от worldtimeapi.org приводил
      к отсутствию fallback и логу только в UI.
- **Закрывает backlog §6.56 (SNAPSHOTS.md):**
  - `SNAPSHOTS.md:1024` — `updateInfantGrowthTable` null.innerHTML;
  - `SNAPSHOTS.md:1027` — `syncTimeWithInternet` `ERR_CONNECTION_RESET`;
  - `SNAPSHOTS.md:1030` — `showTemperatureChart` regex свободного текста.
- **Проверки:**
  - `curl -s http://localhost:8082/api/health` → `{"status":"ok"}`;
    `patient_cabinet_full.html` — HTTP 200, 87604 bytes.
  - Баланс скобок `<script>`-блоков (curly/paren/bracket): все `depth=0`.
  - Ручной smoke:
    - `updateInfantGrowthTable` при отсутствии `#growthTable tbody` — ранний
      выход без исключения, `updateAll()` завершается, в консоли — ровно один
      `[infantGrowth]` warn.
    - `showTemperatureChart`: значения `36.6`, `+38,5`, мусор `н/д` — записи
      с `NaN` не пушатся, ось Y остаётся в `[35..42]`.
    - `syncTimeWithInternet`: `fetch` бросает `TypeError: Failed to fetch`
      (эмулирован) — `__timeSource === "local"`, `#syncStatus` не падает,
      UI показывает «❌ Ошибка синхронизации (локальное время)».
- **Замечания:**
  - `worldtimeapi.org` из контейнера devcontainer недоступен (нет egress) —
    фикс не «чинит» сеть, а делает недоступность безопасной. При появлении
    egress — поведение автоматически станет `__timeSource === "remote"`.
  - Perl- и sed-подходы при патчинге: `git apply` с heredoc-диффами
    падал на кириллице; рабочий метод — точечная замена диапазона строк
    через `sed -i 'N,Mc\ …'` с `LC_ALL=C.UTF-8`.

### 6.56 (B29.7.3) — ЗАКРЫТО (2026-10-14)
- **Тема:** Chart.js реактивно к теме — добивка (statusChart + temperatureChart).
- **Коммиты:**
  - `d121075` feat(charts): B29.7.3.1 — temperatureChart реагирует на смену темы
  - `3766f88` feat(charts): B29.7.3.2 — statusChart реагирует на смену темы
- **Файлы:**
  - `patient_cabinet_full.html` — `showTemperatureChart()`: `options.scales`
    через `BotMaxTheme.getChartColors(BotMaxTheme.current())`; guard-подписка
    `window.__tempChartThemeBound` + `BotMaxTheme.onChange`, коллбэк читает
    `temperatureChart` по замыканию.
  - `doctor_cabinet.html` — подключён `<script src="theme.js"></script>` **без**
    `BotMaxTheme.init` (своя `toggleTheme()` сохранена); после создания
    `statusChart` — `applyChartTheme(statusChart, current())`; в `toggleTheme()`
    после `setAttribute('data-theme', next)` — `applyChartTheme(statusChart, next)`.
- **Проверки:** `gofmt -l` чисто; `go vet`/`go build`/`go test` — зелёные.
  Ручной smoke:
  - `patient_cabinet_full.html`: модалка «📊 Динамика температуры» — оси/ticks/сетка
    в текущей теме; переключение темы при открытой модалке — мгновенно; закрыть →
    сменить → открыть — корректно; guard не дублирует подписку.
  - `doctor_cabinet.html`: донат «СТАТУСЫ» рисуется в текущей теме, легенда/сегменты
    без регресса; переключение темы при открытом донате — tooltip перекрашивается;
    F5 сохраняет тему; консоль чистая (ошибки `BotMaxTheme`/`getChartColors` — нет).
- **Замечания:**
  - `doctor_cabinet.html` использует `theme.js` пассивно (без `init`) — своя
    `toggleTheme()` пишет в тот же ключ `localStorage.theme`, конфликта нет.
- **Backlog (не наши тикеты, обнаружены при smoke):**
  - `updateInfantGrowthTable` падает на `null.innerHTML` (`patient_cabinet_full.html:995`);
    вызывается из `updateGender → updatePatientInfo → updateAll` — у младенцев
    может отсутствовать ожидаемый DOM-элемент.
  - `syncTimeWithInternet` → `GET https://worldtimeapi.org/api/timezone/Europe/Moscow`
    даёт `net::ERR_CONNECTION_RESET` (`patient_cabinet_full.html:869`) — нужен
    fallback или таймаут.
  - `showTemperatureChart` regex `/Температура:\s*([\d.,]+)/i` требует строгий
    формат `Температура: <число>`; свободный текст («08 июня температура») не
    парсится → пустой график.
  - На странице несколько модалок «История X» (SpO2/АД/рост-вес) — не Chart.js,
    просто список; к B29.7.3 отношения не имеют.

### 6.55 (B29.7) — ЗАКРЫТО (2026-10-14)
- **Тема:** Chart.js реактивно к теме (B29.7.1–2).
- **Проблема:** графики витальных показателей в `patient_card.html`
  при переключении темы не перерисовывались — canvas не понимает
  CSS `var(--…)`, цвета осей/легенды/сетки оставались от старой темы.
- **Коммиты:**
  - `7024c55` feat(charts): B29.7.1 — CHART_THEME + applyChartTheme в theme.js
  - `7cd5c3e` fix(ui): B29.7.2 — витальные графики реагируют на смену темы
- **Файлы:**
  - `theme.js` — `CHART_THEME` (палитры dark/light), `getChartColors(theme)`
    (fallback → dark), `applyChartTheme(chart, theme)` — обновляет
    scales/plugins + `chart.update('none')`. Экспорт через `window.BotMaxTheme`.
  - `patient_card.html` — `options` собираются через
    `BotMaxTheme.getChartColors(BotMaxTheme.current())`; одна подписка
    `BotMaxTheme.onChange` на страницу (guard `window.__vitalsThemeBound`),
    коллбэк читает `window.vitalsChartInstance` в момент вызова.
- **Проверки:** `gofmt -l` чисто; `go vet`/`go build`/`go test` — зелёные.
  Ручной smoke (4/4 ✅): 1) график сразу в текущей теме; 2) смена темы
  при открытом графике — мгновенно; 3) закрыть → сменить → открыть —
  корректно; 4) повторные открытия не ломают подписку.
- **Замечания:** при первом открытии после патча браузер отдавал
  закэшированный `theme.js` → `getChartColors is not a function`;
  лечится hard reload (Ctrl+Shift+R). На прод — рассмотреть
  `Cache-Control` для статики.
- **Backlog:** B29.7.3 — применить тот же паттерн в `doctor_cabinet.html`
  (`statusChart`, круговой) и `patient_cabinet_full.html`
  (`temperatureChart`, inline).

### 6.54 (B29.6-ui-fix8) — ЗАКРЫТО (2026-10-14)

**Статус:** рамки блоков в `patient_cabinet_full.html`. 1 подпатч.

**Проблема:** в `patient_card.html` все блоки (`.section`, `.header`,
`.card` и т.д.) получили серые рамки. В `patient_cabinet_full.html`
блоки остались **без рамок** — только фоны. Визуально несогласованно.

**Сделано:** `border: 1px solid var(--border-subtle);` в 8 классов:
- `.header`, `.current-date`, `.combined-section`
- `.card`, `.info-item`
- `.qr-box`, `.computer-box`
- `.modal-content`

Фокус полей ввода уже работал (B29.6.7).

**Токены:** dark `#718096`; light `#cbd5e0`.

**Автопроверки:**
- `gofmt -l` пусто, `go vet`, `go build`, `go test -count=1` — зелёные.
- Баланс скобок CSS 87/87.
- HTTP 200, 85392b.
- `border: 1px solid var(--border-subtle)` = 12 (было 4).

---

### 6.53 (B29.6-ui-fix7) — ЗАКРЫТО (2026-10-14)

**Статус:** добить хардкод-бордеры после B29.6.5a. 1 подпатч.

**Проблема:** при разборе «рамок нет у textarea в светлой теме»
(§6.51–6.52) выяснилось: в `patient_card.html` **два** правила
`.field-textarea`.
- стр. 108 — нормальное, `border: var(--bg-input-border)`.
- стр. 396 — с `background: transparent !important` (поверх
  `.spell-highlight-layer`) и **хардкод-бордером**
  `border: 1px solid rgba(255,255,255,0.1)`.

Второе правило **перебивает** первое. В светлой теме
`rgba(255,255,255,0.1)` = почти прозрачный белый на белом.

**Диагностика:** Console `getComputedStyle(#field_anamnesis).borderColor`
= `rgb(255,255,255,0.1)` — белый вместо ожидаемого `rgba(0,0,0,0.55)`.

**B29.6.15 — `.field-textarea`:**
- `border: 1px solid rgba(255,255,255,0.1)` → `var(--bg-input-border)`.

**B29.6.16 — `.chart-menu-content`:**
- `border: 1px solid rgba(255,255,255,0.1)` → `var(--bg-modal-border)`.

**Не тронуто:** `.spell-tooltip-actions` border-top — разделитель
внутри тёмного тултипа орфографии.

**Урок:** в файле могут быть **дублирующиеся правила** одного селектора.
Токенизация «по grep литералов» их не ловит. Console `getComputedStyle`
быстрее всего указывает источник.

---

### 6.52 (B29.6-ui-fix6) — ЗАКРЫТО (2026-10-13)

**Статус:** финальный контраст рамок в светлой теме. 1 подпатч.

**Коммит:** вошёл в общий коммит (B29.6.13-14).

**Проблема:** после §6.51 (`0.35` = `#d1d1d1`) рамки в светлой теме
всё ещё воспринимались как «едва заметные».

**Прогрессия alpha (light):**
- `0.18` (`#e0e0e0`) — не видно.
- `0.35` (`#d1d1d1`) — едва заметно.
- **`0.55` (`#737373`) — заметно и комфортно** (итоговое).

**Итоговые значения:**
- `--bg-card-border`:  `rgba(0,0,0,0.55)`.
- `--bg-modal-border`: `rgba(0,0,0,0.55)`.
- `--bg-input-border`: `rgba(0,0,0,0.55)`.

Dark не тронут (`0.18`/`0.32`) — на тёмном фоне серые рамки видны.

**Подтверждение:** Console `getComputedStyle(.section).borderColor`
= `rgba(0, 0, 0, 0.55)`. Визуально — рамки отчётливо видны.

**Урок:** для светлого фона «серые рамки» начинаются **от 0.5 alpha**.
Диапазон 0.1-0.35 визуально «исчезает» на белом.

---

### 6.51 (B29.6-ui-fix5) — ЗАКРЫТО (2026-10-13)

**Статус:** промежуточное усиление контраста (шаг к финальному).

**Проблема:** после §6.50 рамки `.section` в светлой теме сливались
с фоном. **Console** показала `border-color = rgba(0,0,0,0.18)`
= `#e0e0e0` — почти невидимый на белом.

**Сделано:** `0.18` → `0.35` (промежуточно).

**Урок:** **Console `getComputedStyle`** даёт точное `border-color` —
сразу видно, «рамка есть, но бледная» или «рамки нет вовсе».
Визуальная оценка ненадёжна — оба случая выглядят как «нет рамок».

---

### 6.50 (B29.6-ui-fix4) — ЗАКРЫТО (2026-10-13)

**Статус:** тема запоминается между сессиями. 1 подпатч.

**Коммит:** `ee1eae5` — fix(frontend): тема запоминается.

**Проблема:** если пользователь открыл страницу и НЕ кликал тумблер,
`localStorage.theme` оставался **пустым**. При следующем открытии
`init()` снова применял `opts.default`, но **не записывал** его.
Итог: тема не переживала закрытие браузера без явного переключения.

**Фикс:** в `init()` при выборе дефолта (когда `saved` пуст) — вызов
`_write(saved)`. Теперь тема запоминается с первого открытия.

**Автопроверки:**
- `gofmt -l`, `go vet`, `go build`, `go test -count=1` — зелёные.
- `theme.js` HTTP 200, 5000b.

---

### 6.49 (B29.6-ui-fix3) — ЗАКРЫТО (2026-10-13)

**Статус:** контрастные поля ввода. 1 подпатч (2 файла).

**Коммит:** вошёл в `8d79f63` (B29.6.10-11).

**Проблема:** после §6.48 (контрастные border-токены) в светлой теме
поля ввода (textarea, input РОСТ/ВЕС/АД, select-фильтры) были
**неотличимы от `.section`** — фон совпадал (оба `#ffffff`), рамка
бледная.

**Сделано:**

`patient_card.html`:
- light: `--bg-input: #ffffff` → `#f7fafc` (отличать от `.section`).
- light: `--bg-input-border: rgba(0,0,0,0.25)` → `rgba(0,0,0,0.35)`.
- dark:  `--bg-input-border: rgba(255,255,255,0.24)` → `rgba(255,255,255,0.32)`.

`patient_cabinet_full.html`:
- light: `--border-subtle: #e2e8f0` → `#cbd5e0`.
- dark:  `--border-subtle: #4a5568` → `#718096`.

**Урок:** «серый на белом» — недостаточно. Для полей ввода нужен
**отличный от карточки фон** + контрастная рамка.

---

### 6.48 (B29.6-ui-fix2) — ЗАКРЫТО (2026-10-13)

**Статус:** контрастные серые рамки в patient_card. 1 подпатч.

**Коммит:** вошёл в `8d79f63` (B29.6.10-11).

**Проблема:** после §6.47 (серые рамки) они были **слишком бледные**
— плохо видны на обоих фонах. Заказчик: «нужно чтобы границы рамок
было видно как на тёмном, так и на белом фоне».

**Сделано:** повышена alpha у **только border-токенов** (фоны не тронуты).

Dark:
- `--bg-card-border`: 0.08 → 0.18
- `--bg-input-border`: 0.12 → 0.32
- `--bg-modal-border`: 0.10 → 0.18

Light:
- `--bg-card-border`: 0.08 → 0.18
- `--bg-input-border`: 0.15 → 0.35
- `--bg-modal-border`: 0.15 → 0.22

**Не тронуто:** фоны (`--bg-card`, `--bg-input`, `--bg-modal`).

---

### 6.47 (B29.6-ui-fix) — ЗАКРЫТО (2026-10-13)

**Статус:** уточнение UI-feedback после §6.46. 1 подпатч.

**Коммит:** `c2c1a13` — fix(ui): серые рамки по умолчанию,
фиолетовые при фокусе.

**Контекст:** в §6.46 (B29.6.8) было сделано «все рамки — фиолетовые»
по умолчанию. Заказчик уточнил: правильно — **серые по умолчанию,
фиолетовые при фокусе**. Откатили, сохранив токен `--border-accent`
для фокусного состояния.

**Сделано:**
- **Откат по умолчанию (серые):**
  - `.header`, `.section` → `var(--bg-card-border)`.
  - `.field-textarea`, `.mkb-input-wrapper input`, `.vital-item input`,
    `.bp-inputs input` → `var(--bg-input-border)`.
  - `.edit-card-modal-content` → `var(--bg-modal-border)`.
- **Focus → `var(--border-accent)`** (вместо `#667eea` / `#9f7aea`):
  `.field-textarea:focus`, `.mkb-input-wrapper input:focus`,
  `.vital-item input:focus`, `.bp-inputs select:focus`,
  `.bp-inputs input:focus`.
- **Универсальное правило** (для inline-стилизованных полей фильтров):
  `input:focus, textarea:focus, select:focus {
   border-color: var(--border-accent) !important; }`.

**Автопроверки:**
- `gofmt -l` пусто, `go vet`, `go build`, `go test -count=1` — зелёные.
- `--border-accent`: 2 определения + 6 применений (`:focus`).
- Серые border'ы восстановлены: `--bg-card-border` × 2,
  `--bg-input-border` × 4, `--bg-modal-border` × 1.
- Баланс скобок CSS 128/128.
- HTTP 200, 144508b.

**Урок:** UI-фидбек уточняется итеративно. `--border-accent` (уже был
заведён в §6.46) переиспользован как **фокусный** цвет.
⚠️ `!` в `git commit -m "..."` в bash → history-expansion: сообщение
обрезается. Использовать `-m '...'` (одинарные кавычки) или `-F <файл>`.

---

### 6.46 (B29.6-ui) — ЗАКРЫТО (2026-10-13)

**Статус:** UI-доработка после визуального смоука темы.
1 подпатч.

**Коммит:** `33d3dcb` — style(ui): единый фиолетовый акцент для рамок.

**Запрос:** рамки полей ввода и карточек должны быть такими же
фиолетовыми, как у кнопки «Редактировать» (визуальный акцент бренда).

**Сделано:**
- Новый токен `--border-accent`
  (dark: `rgba(159,122,234,0.35)`; light: `rgba(159,122,234,0.5)`).
- 7 рамок переведены:
  `.header`, `.section`, `.field-textarea`, `.mkb-input-wrapper input`,
  `.vital-item input`, `.bp-inputs input`, `.edit-card-modal-content`.
- Семантические рамки (`.btn-*`, `dose-*`, badge) не тронуты.

**Автопроверки:**
- `gofmt -l` пусто, `go vet`, `go build`, `go test -count=1` — зелёные.
- `--border-accent` определения = 2 (dark + light),
  использования = 7, остатки старых border-токенов = 0.
- Баланс скобок CSS 127/127.
- HTTP 200, 144168b.

**Не сделано (опционально, backlog):**
- `patient_cabinet_full.html` — рамки остаются нейтрально-серыми
  (`--border-subtle`). Там исходно светлая тема, серая рамка корректна.
  При желании — отдельный подпатч B29.6.9.

---

### 6.45 (B29.6-fix) — ЗАКРЫТО (2026-10-13)

**Статус:** регресс B29.6.5 (неполная токенизация тем) — исправлен.
2 подпатча.

**Коммиты:**
- `<B29.6.6>` — fix(ui): доводка light-темы patient_card
- `<B29.6.7>` — fix(ui): токенизация input/textarea/select (cabinet)

**Регресс:**
- **B29.6.5a.2** (CSS-токенизация `patient_card.html`) заменил только
  `color: #e2e8f0` / `color: #a0aec0`, но **НЕ заменил** background-хардкоды
  (`rgba(26,32,44,0.98)`, `rgba(255,255,255,0.05)`, `rgba(255,255,255,0.03)`,
  `rgba(0,0,0,0.8)`, `box-shadow: ...0,0,0,0.5`). Результат — светлая
  тема полностью сломана: тёмная шапка, невидимые карточки `.section`,
  белый текст на белом фоне.
- **B29.6.5b.2/b.3** (токенизация `patient_cabinet_full.html`) не покрыли
  `input`/`textarea`/`select` в основной части. Результат — в тёмной
  теме textarea симптомов и dropdown'ы оставались белыми.

**Причина:** в B29.6.5 проверяли только **цвет текста** (grep `color:`),
но не **фон** (`grep background:`). Граничный случай: правила
`.section`, `.header`, `.vital-item input` содержали **background**
с теми же литералами (rgba(255,255,255,0.05)), которые не попали в
реестр замен.

**B29.6.6 — patient_card.html:**
- `.header`: background → `var(--bg-header)`; border → `var(--bg-card-border)`;
  shadow → `var(--shadow-lg)`.
- `.section`: background → `var(--bg-card)`; border → `var(--bg-card-border)`.
- `.vital-item input`, `.bp-inputs input`: bg → `var(--bg-input)`;
  border → `var(--bg-input-border)`.
- `.field-textarea`, `.mkb-input-wrapper input`: bg → `var(--bg-panel)`;
  border → `var(--bg-input-border)`.
- `.spell-tooltip-item`: bg → `var(--bg-row-hover)`.
- Модалки (chart-modal, chart-menu overlay): bg → `var(--modal-overlay)`.
- **Новый токен** `--modal-overlay` (dark: rgba(0,0,0,0.8);
  light: rgba(0,0,0,0.5)).

**B29.6.7 — patient_cabinet_full.html:**
- Универсальное правило для полей ввода:
  `input:not([type="checkbox"]):not([type="radio"]):not([type="file"]),
   textarea, select { background: var(--bg-card);
   color: var(--text-primary); border: 1px solid var(--border-subtle); }`.
- Checkbox/radio/file исключены — семантика (accent-color, display:none).

**Автопроверки:**
- `gofmt -l` пусто, `go vet`, `go build`, `go test -count=1` — зелёные.
- В `<style>` `patient_card` **0 остатков** background-хардкодов.
- Баланс скобок CSS: `127/127`, `87/87`.
- HTTP: 200/200, `--modal-overlay` = 4, `input:not(...)` = 1.

**Уроки (для будущих токенизаций):**
- После замены цветов **всегда** проверять и `color:`, и `background:`,
  и `border:`, и `box-shadow:` — одни и те же rgba могут быть в разных
  свойствах.
- **Визуальный смоук в браузере обязателен** для темы — curl не ловит
  «белый текст на белом фоне».

---

### 6.44 (B29.6) — ЗАКРЫТО (2026-10-13)

**Статус:** B29.6 (backlog UI-feedback из §6.43 + доп. тема) — закрыто.
12 подпатчей + автотесты + смоук.

**Контекст:** из UI-смоука B29.5 (§6.43) накопился backlog:
1) локализация kind/status, 2) specialty как select,
3) контрастный календарь, 4) сортировка ASC/DESC.
Плюс в процессе сессии добавлено требование:
5) **тёмная/светлая тема** для кабинета пациента и карты пациента.

**Коммиты (12):**
- `be76538` — B29.6.1 — локализация kind/status (label≠value)
- `a51e726` — B29.6.2 — specialty как `<select>` (`/api/specialties`)
- `7e6cab2` — B29.6.3 — контрастный календарь (`color-scheme`+`invert`)
- `84dc429` — B29.6.4a — `?order=asc|desc` в `listEncountersFiltered`
- `e84153d` — B29.6.4b — сортировка ASC/DESC в `patient_card`
- `d7563ac` — B29.6.5a.1 — `theme.js` + scaffolding (patient_card)
- `4b8a916` — B29.6.5a.2 — токенизация CSS (patient_card)
- `bee36cc` — B29.6.5a.3 — токенизация inline (patient_card)
- `6a6b2bc` — B29.6.5b.0 — `theme.js`: per-page default (`init.default`)
- `6f9e8bd` — B29.6.5b.1 — `theme.js` + scaffolding (cabinet)
- `452614d` — B29.6.5b.2 — токенизация CSS (cabinet)
- `5ab58cc` — B29.6.5b.3 — inline-стили (cabinet)

**B29.6.1 — локализация kind/status (label≠value):**
- `patient_card.html`, `doctor_cabinet.html`: словари
  `ENCOUNTER_KIND_RU` (consultation/study/procedure/lab/followup)
  и `ENCOUNTER_STATUS_RU` (draft/finished); функции
  `encounterKindRu()` / `encounterStatusRu()`.
- Локализованы `<option>` фильтров и badge статуса
  (`renderEncountersTable` + view-modal).
- Значения в API и URL остаются английскими (label ≠ value).

**B29.6.2 — specialty → `<select>`:**
- `patient_card.html`: `<input type=text id=encFilterSpecialty>`
  → `<select>`.
- `loadSpecialtiesIntoFilter()`: `GET /api/specialties` → `Object.keys`
  → `<option>`; fallback на ошибку/пусто — только «— все —» (UI не
  блокируется).
- `keydown`-Enter для specialty удалён (не нужен для `<select>`);
  `encFilterSpecialty` в `change`-ids (авто-применение).
- **fix:** `readFiltersFromURL()` вызывается внутри `.then()` загрузки
  опций — URL-значение `specialty=lor` находит свою опцию (async).

**B29.6.3 — контрастный календарь:**
- `patient_card.html`, `doctor_cabinet.html`: `input[type="date"]`
  `color-scheme: dark` + `::-webkit-calendar-picker-indicator`
  `filter: invert(1) brightness(1.4)`.
- Задел: `[data-theme="light"]` → `color-scheme: light` + `filter: none`.

**B29.6.4a — backend `?order=asc|desc`:**
- `EncounterFilter.Order` (`"asc"`|`"desc"`, default desc).
- `parseEncounterFilter`: валидация; `order=asc` + `cursor` → 400
  (keyset поддерживает только DESC).
- `listEncountersFiltered`: offset-ветка
  `ORDER BY started_at {ASC|DESC}, id {ASC|DESC}`;
  keyset-ветка без изменений.
- Тесты: `TestParseEncounterFilter_Order` (6 кейсов),
  `TestListEncounters_OrderAscDesc` (asc/desc/default + зеркальность).
- Смоук: `desc[0]==asc[-1]`, `desc[-1]==asc[0]`, `id desc monotonic`,
  `bogus=400`, `asc+cursor=400`.

**B29.6.4b — frontend сортировка:**
- `patient_card.html`: `<select id="encFilterOrder">` (Сначала новые /
  Сначала старые, default desc) в панели фильтров.
- `readEncounterFilters` + `writeFiltersToURL` (keys) +
  `readFiltersFromURL` + `resetEncountersFilters` (восстановление
  `'desc'`) + `initEncounterFilters` (change-ids).
- `renderEncountersTimeline`: **удалён локальный `items.sort`** —
  порядок задаёт backend. Устранена двойная сортировка.

**B29.6.5 — тёмная/светлая тема (доп. из UI-feedback):**
- **`theme.js`** (новый файл в корне, ~148 строк): `window.BotMaxTheme`
  (`init/toggle/set/current/mountButton/onChange`); ключ localStorage
  `theme`, default `'dark'`; `init({default})` — per-page default.
- **`patient_card.html`** (дефолт **dark**):
  - a.1: `theme.js` + scaffolding + `<span id="themeHost">` (в шапке).
  - a.2: токенизация CSS — 25 замен (`--bg-body-1/2`, `--bg-modal`,
    `--bg-modal-border`, `--text-primary/secondary/title`,
    `--shadow-md/lg`).
  - a.3: токенизация inline — 61 замена + новый токен `--bg-panel`.
  - **Итого ~86 замен**; все хардкоды (кроме Chart.js canvas и
    семантики) переведены на `var(--…)`.
- **`patient_cabinet_full.html`** (дефолт **light**):
  - b.0: `theme.js`: `init({default})`.
  - b.1: `theme.js` + scaffolding + `<span id="themeHost">`
    (рядом с logout).
  - b.2: токенизация CSS — 30 замен + новый токен `--border-subtle`.
  - b.3: токенизация inline — 3 замены.
  - **`@media print { body { background: white; } }` сохранён**;
    бренд-градиенты `#667eea→#764ba2` и семантика статусов
    (`#48bb78`/`#f56565`/`#4299e1`/`#ed8936`) не тронуты.

**Автопроверки:**
- Backend: `gofmt -l` пусто, `go vet`, `go build`, `go test -count=1` —
  зелёные на каждом шаге.
- HTTP: `theme.js` → 200 `text/javascript`; `patient_card.html` → 200;
  `patient_cabinet_full.html` → 200.
- Смоук: `/api/specialties` → `{"lor":["2024"]}`;
  `asc`/`desc` зеркальны (mirror desc[0]==asc[-1], desc[-1]==asc[0]);
  `bogus=400`; `asc+cursor=400`.

**Файлы, затронутые в волне:**
- `backend/encounters.go` (+29/−4) — поле `Order`, валидация, SQL.
- `backend/encounters_filter_test.go` (+52) — тесты парсера.
- `backend/encounters_list_test.go` (+42) — тест сортировки.
- `patient_card.html` (много правок: локализация, select specialty,
  order, CSS-токены, inline-токены, theme.js).
- `doctor_cabinet.html` (+11/−2 — только локализация + календарь).
- `patient_cabinet_full.html` (~+65/−62 за 4 подпатча).
- `theme.js` (**новый**, ~148 строк).

**Не тронуто осознанно:**
- `doctor_cabinet.html` — уже имел свою систему тем (`:root` /
  `[data-theme="light"]`), не рефакторили.
- Chart.js-цвета в `patient_card` (canvas не резолвит CSS-переменные).
- `@media print`, print-preview iframe, бренд-градиенты, семантика
  статусов кнопок/бэйджей.

**Закрывает backlog §6.43:** все 4 пункта + тема.

### 6.42 (B29.5) — ЗАКРЫТО (2026-10-13)

**Статус:** B29.5 (backlog-фикс: регресс `limit` 100→20 в `doctor_cabinet.html`
+ унификация пагинации) — закрыто, 3 подпатча + автотесты + ручной смоук.

**Контекст:** смена default `limit` в B29.1 (encounters.go:47,
`encounterListLimitDefault = 20`) сломала `doctor_cabinet.html` — голый
`GET /api/encounters?doctor_id=...` без `limit` получал 20 записей вместо 100,
без пагинации и без возможности дойти до остальных. Вместо точечного
`?limit=100` сделана **унификация**: единый модуль `pager.js` для обеих
страниц истории (`patient_card.html` и `doctor_cabinet.html`).

**Коммиты:**
- `54e3f17` — B29.5.1 — `pager.js`: общий модуль пагинации (`window.BotMaxPager`).
- `c0f6c5d` — B29.5.2 — `doctor_cabinet.html`: подключение pager'а, `#encountersPager`, lazy-init `ensureEncountersPager`. Фикс регресса.
- `4e89898` — B29.5.3 — `patient_card.html`: удалены дубли `fetchEncounters`/`renderPager`, добавлен `ensureEncountersPager`; **bugfix B29.2.4** — в server-пути не вызывался `initEncounterFilters()`.

**B29.5.1 — pager.js (новый файл в корне репо):**
- IIFE, `'use strict'`, экспорт `window.BotMaxPager` (паттерн `auth.js`).
- 343 строки / 14175 байт. Отдаётся `backend/main.go:508`
  (`http.FileServer(http.Dir(rootDir))`) с `Content-Type: text/javascript; charset=utf-8`.
- API:
  - `create(opts)` → `{ reload, reset, fetch, next, prev, setOffset, setLimit, getState, render }`
  - `buildQuery(params)` — детерминированная сериализация (sorted keys, skip `''`/`null`/`undefined`).
  - `computePagerMeta(state)` → `{ from, to, total, canPrev, canNext, label }`.
  - `readFiltersFromURL(keys)`, `syncFiltersToURL(filters, keys, extra)`.
- `create(opts)`:
  - `endpoint` (default `/api/encounters`), `fixedParams` (`{doctor_id}` | `{patient_id}`)
  - `getFilters()` — возвращает актуальный набор фильтров (kind/status/specialty/from/to/limit)
  - `onItems(items, meta)`, `onError(err)`, `onAfterFetch(meta)`
  - `pagerEl` (default `encountersPager`), `urlSync` (default true), `urlKeys`
- Рисует в `#encountersPager`: кнопки `#encPagePrev`/`#encPageNext` + label
  «N–M из TOTAL» (или «стр. K» при `total=null`).
- `urlSync` через `history.replaceState`; `offset=0` не пишется в URL.

**B29.5.2 — `doctor_cabinet.html`:**
- Подключён `<script src="pager.js">` после `auth.js` (строка 728).
- Добавлен `<div id="encountersPager">` рядом с `#encountersTableBody` (строка 511).
- `loadMyEncounters` → `ensureEncountersPager` (lazy) + `pager.reload()`.
- `renderEncountersTable(items, meta)` — сигнатура расширена (meta пока не используется).
- `fixedParams: { doctor_id: currentDoctorId }`, `urlSync: false` (без URL-фильтров в кабинете).
- **Регресс вылечен:** при 25 приёмах видны «1–20 из 25» + кнопки «←/→», переход даёт «21–25 из 25».
- Размер: 144587 → 145443 байт (+856).

**B29.5.3 — `patient_card.html`:**
- Подключён `<script src="pager.js">` после `auth.js` (строка 930).
- Удалены дубли: `fetchEncounters` (~49 строк), `renderPager` (~68 строк).
- Добавлен `ensureEncountersPager` (lazy, `fixedParams: { patient_id: currentPatientId() }`).
- Все три точки входа переписаны на `pager.reload()`:
  `loadPatientEncounters`, `applyEncountersFilters`, `resetEncountersFilters`.
- `getFilters` возвращает `encState.lastFilters` (UI-концепция «применённые фильтры» сохранена).
- `urlSync: false` — URL-синхронизация остаётся за `writeFiltersToURL` (B29.2.4), без двойного `replaceState`.
- **Bugfix B29.2.4:** в server-пути `loadPatient → then(...)` пропущен вызов
  `initEncounterFilters()` — фильтры из URL не подхватывались, обработчики
  Apply/Reset/Enter/change не навешивались. Исправлено в обоих путях (server и fallback).
- Размер: 140850 → 137465 байт (−3385); строки: 3390 → 3307 (−83).

**Автопроверки:**
- Backend: `gofmt -l backend/` пусто, `go vet ./...`, `go build -o /tmp/botmax_server ./backend`, `go test -count=1 ./backend/...` — зелёные.
- HTTP: `pager.js` → `200 OK`, `text/javascript; charset=utf-8`, `Content-Length: 14175`, `Accept-Ranges: bytes`.
- Post-patch sanity: `doctor_cabinet` — `pager.js` (1), `#encountersPager` (1), `BotMaxPager.create` (1), старый `fetchEncounters` = 0;
  `patient_card` — `fetchEncounters`/`renderPager` = 0, `BotMaxPager.create` = 1, `ensureEncountersPager` = 4, `initEncounterFilters` = 2.
- API-контракт (curl, DOC-001):
  - `?doctor_id=DOC-001&limit=20&offset=0` → `count: 20, total: 25`
  - `?doctor_id=DOC-001&limit=20&offset=20` → `count: 5, total: 25`
  - `?patient_id=PAT-...&status=finished&limit=5` → `count: 5, total: 8`

**Ручной UI-смоук (2026-10-13, БД `/tmp/b295_smoke.db`, DOC-001):**
- `doctor_cabinet.html`: 25 приёмов, «1–20 из 25», «Вперёд →» → «21–25 из 25», ←/→ активны/disabled корректно.
- `patient_card.html`: 25 приёмов, timeline, панель фильтров, «показано 1–20 из 25», счётчик «Найдено: 25».
- `patient_card.html?status=finished`: селект `status` = `finished` (bugfix B29.2.4 подтверждён), timeline = 8 finished, «показано 1–8 из 8», «Найдено: 8».
- `patient_card.html` с `limit=50` через UI: «показано 1–25 из 25».

**Итоги волны:**
- Единый источник правды для пагинации: `pager.js` используется обеими страницами.
- `patient_card.html` −83 строки.
- Устранён скрытый баг B29.2.4 (server-путь не инициализировал фильтры).
- Регресс B29.1 закрыт для UI.

**Backlog из смоука → см. §6.43:**
- Локализация опций `kind`/`status` на русский.
- Специальность → выпадающий список.
- Календарь (from/to) контрастнее.
- Сортировка приёмов ASC/DESC.

<!-- B29_5_SNAPSHOT_APPLIED -->

### 6.41 (B29.2) — ЗАКРЫТО (2026-10-13)

**Статус:** B29.2 (frontend: панель фильтров + пагинация в `patient_card.html`) — закрыто, 4 подпатча + смоук.

**Коммиты:**
- `e134215` — B29.2.1 — HTML: `<details>` панель фильтров + `#encountersPager`.
- `cfd668c` — B29.2.2 — JS: `encState` + `fetchEncounters` + meta (limit/offset/total) в timeline.
- `8483575` — B29.2.3 — `renderPager`: Назад/Вперёд + «N–M из TOTAL».
- `313b95b` — B29.2.4 — Применить/Сбросить + URL-синхронизация (`replaceState`) + Enter/change.

**Что появилось на странице `patient_card.html`:**
- Свёрнутая панель «🔎 Фильтры»: `kind` (select), `status` (select), `specialty` (text),
  `from`/`to` (`<input type="date">`), `limit` (10/20/50/100, default 20).
  Кнопки «Применить»/«Сбросить».
- Счётчик `#encFiltersTotal` («Найдено: N») и `#encountersCountLabel` («показано X–Y из N»).
- Пагинация `#encountersPager` (offset-режим): «← Назад» / «N–M из TOTAL» / «Вперёд →».
  Кнопки дизейблятся на границах.
- Синхронизация фильтров с URL через `history.replaceState` — ссылка шарится.
- Enter в поле `specialty` / изменение `select`'ов = «Применить».
- «Сбросить» возвращает `limit=20` и пустые поля.

**Смоук (curl, `/tmp/botmax_b29_2.db`):**
- `/api/health` → `{"status":"ok"}`.
- login (`doctor@local` / `doctor@local`) → `accessToken`.
- `?doctor_id=DOC-001&limit=5` → `{count, items, limit, offset, total}`.
- `?...&no_total=1` → без ключа `total`.
- `?kind=bogus` → HTTP 400 `{"error":"invalid kind: \"bogus\""}`.

**Осталось за кадром (backlog):**
- `doctor_cabinet.html:2134` — `GET /api/encounters?doctor_id=...` всё ещё без фильтров
  и пагинации. Отдельный подпатч B29.5 или перенести в B30+.
- Ключ `total` в счётчике при `no_total` не используется на UI; если понадобится
  «Ещё»-кнопка на keyset — API готово.
- Экспорт фильтров в CSV — не в этой волне.

**Следующий шаг:** B29.3 — frontend: keyset-пагинация (опционально) или перейти к B30
(экспорт/печать). Решение — в начале следующей сессии.

### 6.40 (B29.1) — ЗАКРЫТО (2026-10-13)

**Статус:** B29.1 (backend: фильтры + пагинация + масштабирование) — закрыто, 6 подпатчей + правки тестов.

**Коммиты:**
- `631093a` — B29.1.1 — индекс `idx_enc_patient_kind(patient_id, kind, started_at DESC)`.
- `a1de28a` — B29.1.2 — `EncounterFilter` + `parseEncounterFilter` + cursor helpers.
- `9047d3c` — B29.1.3 — SQL-сборщик WHERE + keyset + `listEncountersFiltered`/`countEncounters`.
- `7694a2e` — B29.1.4 — 10 repository-тестов list (фильтры, offset, keyset, count).
- `6a1513f` — B29.1.5 — 3 EXPLAIN QUERY PLAN теста (`idx_enc_patient_kind` в offset и keyset).
- `d523198` — B29.1.6 — 13 HTTP-тестов list (400/200/RBAC/no_total/next_cursor).

**Итого:** 32 теста B29.1 PASS, полный `go test ./backend/...` зелёный.

**Контракт `GET /api/encounters` (актуальный):**
- Скоуп (обязателен): `patient_id | doctor_id | clinic_id`.
- Фильтры (все опц.): `kind` ∈ {consultation, study, procedure, lab, followup};
  `status` ∈ {draft, finished}; `specialty` (точное совпадение);
  `from`, `to` — RFC3339, включительно, `from ≤ to`.
- Пагинация: `limit` ∈ [1..200] (default **20**, было 100); `offset` ∈ [0..10000] (default 0);
  `cursor` — opaque keyset, `base64url(startedAt|id)`; при заданном `cursor` offset игнорируется.
- `no_total=1` (или `true`) — не считать COUNT(*).
- Ответ: `{items, limit, offset, count, total?, next_cursor?}`.
  - `total` — отсутствует при `no_total=1`.
  - `next_cursor` — присутствует только если страница не последняя (`LIMIT limit+1`).

**Реализация:**
- `EncounterFilter` + `parseEncounterFilter(q url.Values)` — вся валидация, 400 с человекочитаемым `error`.
- `buildEncounterWhere(f)` — динамический WHERE с `?`-плейсхолдерами.
- `listEncountersFiltered(db, f)` — offset- или keyset-режим, `LIMIT limit+1`, `next_cursor`.
- `countEncounters(db, f)` — COUNT(*) без cursor/offset/limit.
- `scanEncounter` — единая точка scan (`encounterSelectCols`).
- Удалены `listEncountersByPatient` / `listEncountersByDoctor` / `listEncounters`.
- `handleEncountersList` переписан поверх `listEncountersFiltered`; RBAC сохранён
  (doctor → только `doctor_id=me`; admin → своя клиника; superadmin → любые).

**Тесты:**
- `encounters_filter_test.go` — 6 тестов (`parseEncounterFilter` OK/Errors/Defaults,
  cursor round-trip/empty/bad).
- `encounters_list_test.go` — 10 repository-тестов (kind/status/specialty/from-to/combined,
  offset, keyset, next="", count, keyset stable under insert).
- `encounters_plan_test.go` — 3 EXPLAIN-теста (idx используется для offset и keyset,
  без `SCAN encounters`; для запроса без kind — `idx_enc_patient`).
- `encounters_http_test.go` — 13 HTTP-тестов (bad kind/status, from>to, limit>200,
  offset>10000, no scope, doctor patient list + total, no_total, next_cursor,
  doctor foreign doctor_id → 403, admin foreign clinic → 403, admin own clinic).

**Изменение поведения (важно для B29.2 UI):**
- Default `limit` уменьшен 100 → 20. Фронт должен явно передавать `limit`,
  если нужны большие страницы.
- В ответе появились `total` (по умолчанию) и `next_cursor` (на неполных страницах).
- 400-ответы теперь `{"error":"<detail>"}` от `json.NewEncoder`, с HTML-экранированием
  (`>` → `\u003e` в сыром теле; после `Unmarshal` — нормально).

**Открытые вопросы для B29.2 (UI):**
1. Фронт передаёт `limit` явно? Дефолт 20 может «обрезать» старые сценарии.
2. Использовать ли `next_cursor` на UI или жить на offset?
3. Отображать ли `total` в интерфейсе (счётчик «N приёмов»)?

**Следующий шаг:** B29.2 — Frontend: панель фильтров в `patient_card.html`.

### 6.39 (B29) — ПЛАН (в работе, checkpoint 2026-10-09)

**Статус:** запланировано, к реализации.

**Цель B29:** История пациента (timeline + фильтры + права).

**Разбивка:**
- **B29.1** — Backend: фильтры + пагинация + масштабирование.
- **B29.2** — Frontend: панель фильтров в `patient_card.html`.
- **B29.3** — Frontend: пагинация (offset + keyset).
- **B29.4** — Снапшот §6.39.

**Решения по B29.1 (согласовано 2026-10-09):**
- Фильтры: `kind`, `status`, `specialty`, `from`, `to` (все опциональные).
- Пагинация: **offset** (default) + **keyset cursor** (для больших данных).
- `limit`: 1..200, default 20 (было 100 — уменьшено для производительности).
- `offset`: 0..10000 (защита от глубокой пагинации).
- `total` — через COUNT(*) с возможностью отключить `?no_total=1`.
- Keyset — **сейчас** (оба режима: offset + cursor).
- Новый индекс: `idx_enc_patient_kind ON encounters(patient_id, kind, started_at DESC)`.
- Валидация: `kind` ∈ {consultation, study, procedure, lab, followup},
  `status` ∈ {draft, finished}, `from`/`to` — ISO-8601, `from ≤ to`.
- **EXPLAIN QUERY PLAN** тест — проверка использования индекса.

**Следующий шаг:** B29.1 (backend patch).

**Модель доступа (из B28.4.4a):**
1. Врачи одной клиники — видят всё.
2. Сторонние врачи — по consent (B34/B42).
3. `patient_portal` (B42) — ЛКП с magic link.
4. Дневник + adherence — B43.6+, B46, B50.

**Волны после B29:** B30 (экспорт/печать), B31 (PIN+QR гостевой доступ),
B32 (клиники), B33 (модули), B34 (consents), ... B42 (patient_portal).



### 6.38 (B28) — ЗАКРЫТО (2026-10-09)

**Статус:** закрыто полностью, 12 подпатчей.

**Закрыто:**
- **B28.1** (`7d8de0e`) — schema `encounters` (20 колонок + 3 индекса) + `newID` helper.
- **B28.2** (`57d3069`) — store-слой (CRUD + list + finish), 17 тестов.
- **B28.3** (`d53c49d`) — HTTP API `/api/encounters/*` + json-теги + права + аудит.
- **B28.4.1** (`0a4a016`) — timeline приёмов в `patient_card.html`.
- **B28.4.2** (`aba510d`) — раздел «Приёмы» в `doctor_cabinet.html`.
- **B28.4.3** (`0fc75bb`) — редактирование / завершение / удаление.
- **B28.4.4a** (`fd96a18`) — фикс 403: врач видит timeline пациента (в рамках одной клиники).
- **B28.4.4b** (`f3b4671`) — русификация типов, специальности select, кнопка 🔗 «в новой вкладке».
- **B28.4.5** (`ea137d9`) — `encounter.html` — отдельная страница просмотра приёма.
- **B28.5.1** (`00007ab`) — `POST /api/encounters/prefill` (backend).
- **B28.5.1b** (`a33f90a`) — 4 теста prefill + helper `getLatestEncounterForPrefill`.
- **B28.5.2** (`e524d97`) — prefill UI (кнопка 🪄 + 6 клинических полей в модалке).

**Что реализовано:**
- Полный CRUD по приёмам (encounters).
- Три UI-точки: раздел «Приёмы» в кабинете врача, timeline в карточке пациента, отдельная страница `encounter.html`.
- Prefill — автозаполнение формы нового приёма из последнего приёма пациента.
- Русификация типов приёмов, справочник специальностей (15).
- **21 тест** (17 store + 4 prefill).

**API:**
- `GET    /api/encounters?patient_id=|doctor_id=|clinic_id=` — список.
- `POST   /api/encounters` — создать.
- `GET    /api/encounters/{id}` — получить.
- `PATCH  /api/encounters/{id}` — обновить.
- `POST   /api/encounters/{id}/finish` — завершить.
- `DELETE /api/encounters/{id}` — удалить (только draft).
- `POST   /api/encounters/prefill` — prefill из последнего приёма.

**Модель доступа к истории пациента (решение B28.4.4a):**
1. **Врачи одной клиники** видят историю пациента без доп. разрешений.
2. **Сторонние врачи** (из других клиник) — только по consent от пациента.
3. **Личный кабинет пациента** хранит все приёмы от всех врачей.
4. **Пациент** управляет доступом (выдаёт/отзывает consent).
5. **Scope'ы**: encounters, adherence, diary (отдельные разрешения).
6. В B28.4.4a — временное послабление (403 убран): в системе одна клиника.
7. Полная consent-модель — в **B30**.

**Открытые TODO (перенесено в план):**
- **B29** — auth: PIN-код + вход по ссылке на email (magic link).
- **B30** — кабинет пациента: рекомендации + дневник + adherence + consent + доступ сторонним врачам.
- **Синхронизация пациентов**: `STORAGE_MODE` (`server` vs `cache`) — пациенты в UI есть, в БД нет (обнаружено в smoke B28.4).

**Следующая волна:** B29 или B30 (на выбор).


---

## План развития: МИС-функциональность (B27–B42)

**Добавлено:** 2026-10-02 (перед B27).
**Статус:** зафиксирован, реализация — по волнам B27…B42.
**B27 (password reset) — ЗАКРЫТ 2026-10-03** (см. §6.36).
Следующая волна — B28 (encounters + clinics).
**Контекст:** расширение BOT_MAX от «кабинета одного врача» до
амбулаторной МИС: несколько специальностей, общая история пациента,
внешние врачи по PIN/QR, лаборатории, справки, рецепты, направления на ЭЛН.
**Принципы:**
- **Модульность** — крупные блоки (referrals, labs, certificates, ...)
  подключаются через env-флаг `BOTMAX_MODULES`, клиника берёт только нужное.
- **152-ФЗ** — раздельные согласия (ПД / медвмешательство / манипуляция),
  read-логи медданных, локализация, шифрование at rest.
- **Версионирование шаблонов** — документ хранит снимок текста шаблона;
  редактирование — через админку, без деплоя.
- **Опциональность prefill** — врач сам решает, что подтянуть из истории.

### Обзор новых сущностей

| Сущность | Назначение |
|---|---|
| `encounters` | Приёмы (эпизоды) — центральная сущность, timeline пациента |
| `specialty_fields` | Редактируемая схема полей по специальностям |
| `referrals` | Направления к врачам / на исследования / анализы |
| `labs`, `lab_tests`, `lab_results`, `lab_result_values` | Лаборатории и результаты |
| `templates` (encounter/prescription/certificate) | Шаблоны осмотров, назначений, справок |
| `medications` | Справочник препаратов |
| `icd10_codes` | Справочник МКБ-10 |
| `prescriptions`, `prescription_items` | Выписанные рецепты (форма 107-1/у) |
| `certificates` | Справки (садик / секция / студент / бассейн) |
| `sick_leave_requests` | Направления на открытие листа нетрудоспособности |
| `consents` | Согласия (ПД, медвмешательство, манипуляция) |
| `reports` | Сгенерированные PDF/HTML/CSV с QR-верификацией |
| `clinics`, `clinic_doctors` | Клиники как организации |
| `password_reset_tokens` | Сброс пароля врача |

### Encounters — эпизоды с prefill

**Ключевая модель:**

    encounters
      id, patient_id, doctor_id, clinic_id
      specialty          -- снимок специальности приёма
      title              -- «Первичный осмотр ЛОР», «Повторный приём»
      kind               -- consultation | study | procedure | lab | followup
      started_at, finished_at, status
      complaint, anamnesis, examination, diagnosis_text, diagnosis_icd10, recommendations
      common_json        -- общие поля (аллергии, хронические, рост/вес/ИМТ, ...)
      specialty_json     -- специфичные поля специальности

**Разделение полей:**
- **Общие поля** (common_json) — видны всем специальностям, копируются
  между любыми врачами: allergies, chronic_conditions, blood_type,
  disability, height, weight, bmi, smoking, alcohol, family_history.
- **Специфичные поля** (specialty_json) — зависят от специальности,
  схема в specialty_fields (редактируется через админку).

**Prefill (предзаполнение нового приёма):**
- POST /api/encounters/prefill — возвращает кандидатов:
  - common_fields: значения из последнего завершённого encounter
    любой специальности + источник (дата, врач);
  - specialty_previous: предыдущий протокол той же специальности +
    полный текст для предпросмотра.
- POST /api/encounters принимает блок prefill:
  {common_from_encounter_id, specialty_from_encounter_id, fields: [...]}.
  Сервер сам копирует значения (фронт не «подсовывает» данные).
- UI: при открытии нового приёма — кнопка «Заполнить из предыдущего»;
  модалка с чек-боксами и предпросмотром. По умолчанию выключено.
- Не копируются: подписи, согласия, status — у каждого эпизода свои.
- Аудит: encounter.prefilled с detail={from_encounter_ids, fields}.

### Роли

- superadmin — всё;
- admin — своя клиника (врачи, пациенты, отчёты);
- doctor — свои приёмы + read-доступ к истории пациентов, с которыми
  есть encounter;
- registrar (новая, включается при активном модуле sick_leave) —
  оформляет направления на ЭЛН, работает с СФР-системой клиники.

### Модульная архитектура

**Env-флаг BOTMAX_MODULES:**
- пусто — базовый набор (auth, doctors, patients, audit, encounters);
- all — все доступные модули;
- referrals,labs,certificates — выборочно.

**Доступные имена модулей:**
referrals, labs, templates, prescriptions, certificates,
sick_leave, consents.

**Зависимости (fail-fast при старте):**
- labs требует referrals;
- prescriptions требует templates;
- certificates требует templates.

**Структура кода:**

    backend/modules/
      registry.go           -- интерфейс Module + allModules
      referrals/
      labs/
      certificates/
      sick_leave/
      templates/
      prescriptions/
      consents/

**Публичный endpoint:** GET /api/modules → {"enabled": [...]}.
**UI-хелпер:** BotMaxModules.isEnabled("labs") — рендерить условно.
Серверная проверка всё равно главная (роуты модуля не регистрируются,
если модуль выключен).

### Безопасность

**Password reset (B27):**
- POST /api/auth/password-reset/request {email} — токен 32 байта,
  хранится hashed, TTL 1 час;
- POST /api/auth/password-reset/confirm {token, new_password};
- POST /api/doctors/{id}/reset-password (admin) — временный пароль,
  флаг must_change_password;
- POST /api/auth/change-password (self);
- brute-force lock: 5 неудач → locked_until = now + 15m;
- middleware: при must_change_password=true все API кроме
  /change-password и /logout → 403.

**Гостевой доступ (PIN + QR):**
- PIN — 6 цифр (не 4!), crypto/rand, хранится hashed;
- TTL PIN ≤ 1 час, одноразовый, rate-limit по IP;
- после ввода PIN выдаётся HttpOnly cookie-сессия
  (botmax_guest_session), сам PIN в URL/логах не участвует;
- QR-сценарий — тот же invite, но token в URL (32 байта hex);
- никогда не показывать PIN/токен повторно — только при создании;
- «Потерял PIN» → кнопка «Создать новый invite», старая инвалидируется.

**Согласия (ПЭП + 152-ФЗ):**
- простая ЭП: подпись пальцем на canvas, сохранение текста
  согласия + версии + sha256 + IP/UA + PDF;
- раздельные согласия: ПД, медицинское вмешательство, конкретная
  манипуляция — отдельно;
- отзыв: revoked_at, документы не удаляются (доказуемость);
- read-логи медданных (patient.read, encounter.read) — отдельно
  от audit-log операций;
- шифрование at rest (SQLCipher) — отложено в B41.

**QR-верификация отчётов:**
- каждая выданная справка/рецепт/PDF содержит QR на
  /verify/report/<id>?sig=<hmac>;
- HMAC-SHA256 по report_id|content_sha256|issued_at секретом
  BOTMAX_REPORT_SIGNING_KEY;
- reports хранит content_sha256 — правки не сойдутся.

### Версионирование шаблонов

    <entity>_templates
      id, version, active, parent_id, body_template, created_by, created_at

- Редактирование через admin_templates.html (admin/superadmin);
- врач — только личные шаблоны, admin — клинические;
- при использовании документа сохраняется снимок body_template
  в самом документе + ссылка template_id + template.version_used;
- базовые шаблоны засеиваются один раз (seed_templates.go) только
  если таблица пуста — админ может их изменить/удалить.

### Реестр модулей (план волн B27–B42)

| Волна | Что | Модуль |
|---|---|---|
| B27 | Password reset + must_change_password + brute-force lock + роль registrar | (базовое) |
| B28 | Encounters + clinics (минимум): title, kind, common_json, specialty_json | (базовое) |
| B28.5 | Prefill: POST /encounters/prefill + UI-модалка + аудит | (базовое) |
| B29 | История пациента (timeline + фильтры + права) | (базовое) |
| B30 | Экспорт/печать: PDF + HTML + QR-верификация | (shared) |
| B31 | Гостевой доступ: PIN + QR + cookie-сессия | (базовое) |
| B32 | Клиники как организации (many-to-many) | (базовое) |
| B33 | Реестр модулей + BOTMAX_MODULES + /api/modules | (инфраструктура) |
| B34 | Модуль consents (ПЭП + 152-ФЗ) | consents |
| B35 | Модуль referrals (направления к врачам/исследованиям) | referrals |
| B36 | Модуль labs (лаборатории + результаты + справочник тестов) | labs |
| B37 | Модуль templates + specialty_fields + справочники | templates |
| B38 | Модуль prescriptions (рецепты + PDF 107-1/у) | prescriptions |
| B39 | Модуль certificates (справки) | certificates |
| B40 | Модуль sick_leave (направления на ЭЛН) | sick_leave |
| B41 | 152-ФЗ: read-логи медданных, SQLCipher, 2FA админов | (базовое) |
| B42+ | Интеграции (опционально): СФР (ЭЛН), лаборатории (API), аптеки, ЕГИСЗ | — |

MVP: B27 → B28 → B29 → B30 (password reset, приёмы, история, печать).

### Отложенные интеграции (B42+)

- СФР / ЭЛН: интеграция через СМЭВ — отдельный проект с внешними
  сертификациями. В BOT_MAX — только «направление на открытие листа»;
- Лаборатории через API (Инвитро/Гемотест) — если дадут доступ;
- ЕГИСЗ / ЕМИАС — региональные МИС;
- Аптеки, маркировка ЛС — отдельная область.

### Принятые решения (согласованы 2026-10-02)

1. Справочник лабораторных тестов — свой, минимальный (~50 популярных),
   пополняется через админку.
2. Справочник препаратов — ручной, ~100 популярных, пополняется
   через админку.
3. Рецептурные формы: 107-1/у (обязательно), 107-1/у-04(л)
   (опционально). 148-1/у-88 не делать без лицензии.
4. СФР/ЭЛН — только «направление на открытие», без интеграции.
5. Справки MVP: садик / секция / студент / бассейн. Остальные — позже.
6. Шаблоны: личные (врач) + клинические (admin).
7. Шаблоны редактируются через админку, без деплоя. Версионирование
   с сохранением снимка текста в документе.
8. Prefill: по умолчанию выключен (кнопка «Заполнить из
   предыдущего»). Предпросмотр — разворачивающийся блок в модалке.
9. Общие поля: allergies, chronic_conditions, blood_type, disability,
   height, weight, bmi, smoking, alcohol, family_history.
10. kind: consultation, study, procedure, lab, followup.
11. Копирование подписей/согласий — НЕ делаем. У каждого эпизода свои.
12. Если нет предыдущего приёма — prefill-модалка не показывается,

---

## Дополнение §6.37: модули коммуникаций, ЛКП, монетизация

**Добавлено:** 2026-10-03.
**Контекст:** расширение плана — внутренние коммуникации (почта,
звонки), личный кабинет пациента, оповещения, монетизация.

### Модуль messages — внутренняя почта (врач↔врач)

**Назначение:** асинхронные сообщения между врачами одной клиники +
системные уведомления от других модулей.

**Модель данных:**

    messages
      id, thread_id, sender_id, sender_kind, subject, body, kind,
      priority, created_at, edited_at, deleted_at,
      ref_entity, ref_entity_id

    message_threads
      id, subject, created_at, updated_at, last_message_id,
      is_system, clinic_id

    message_recipients
      id, message_id, recipient_id, read_at, archived_at, folder

    message_templates
      id, doctor_id, clinic_id, name, subject_template,
      body_template, created_at, updated_at

**Правила доступа:**
- врач — любому врачу своей клиники;
- admin — любому врачу своей клиники;
- superadmin — любому врачу;
- гостевой врач (PIN) — НЕ может писать;
- admin/superadmin НЕ видят содержимое личных сообщений
  (только метаданные в audit: кто, кому, когда).

**Системные уведомления:** единый API

    messages.SendSystem(toDoctorIDs, subject, body,
                        refKind, refID, priority)

- если модуль messages выключен — no-op (без падения других модулей);
- источник: `botmax/referrals`, `botmax/labs`, `botmax/guest`, ...

**API:**

    GET    /api/messages                     inbox, пагинация, фильтры
    GET    /api/messages/threads
    GET    /api/messages/threads/{id}
    POST   /api/messages                     создать
    PATCH  /api/messages/{id}                редактировать (TTL 15м)
    DELETE /api/messages/{id}                soft delete
    POST   /api/messages/{id}/read
    POST   /api/messages/{id}/archive
    GET    /api/messages/unread-count        {total, urgent}
    GET    /api/message-templates
    POST   /api/message-templates

**UI:**
- `inbox.html` — список тредов, фильтры, поиск;
- badge ✉️ N в шапке `doctor_cabinet.html`;
- композер с autocomplete врачей клиники + шаблоны;
- привязка к entity (encounter, referral, lab_result) — «Открыть» открывает контекст.

**Realtime (эволюция):**
- MVP: polling `/api/messages/unread-count` каждые 60с;
- далее: SSE `GET /api/messages/stream`.

**Env:**
- `BOTMAX_MESSAGE_RETENTION_DAYS` (default 365).

**Волна:** B33.5.

### Модуль patient_portal — личный кабинет пациента

**Назначение:** защищённый доступ пациента к своим данным (read-only),
к переписке с врачом, к онлайн-записи и уведомлениям.

**Ключевой принцип:** `patients` (медкарта) ≠ `patient_accounts` (аккаунт).
Не у каждого пациента есть аккаунт (дети, пожилые, недееспособные).
Аккаунт — 1:1 к пациенту, опционально.

**Модель данных:**

    patient_accounts
      patient_id PK, login_email NULL, login_phone NULL,
      password_hash NULL, must_change_password,
      failed_login_count, locked_until,
      created_at, last_login, email_verified, phone_verified, active

    patient_magic_links
      id, token_hash UNIQUE, patient_id, created_at, expires_at,
      used_at, purpose

    patient_consents
      id, patient_id, kind, version, text_sha256, signed_at,
      signature_blob, context_ip, context_ua, revoked_at

**Вход (гибрид):**
- magic-link на email/SMS — основной способ;
- опциональный пароль — для частого использования;
- 2FA (SMS/TOTP) — опционально;
- сессия в HttpOnly cookie, TTL 30 мин бездействия;
- уведомление о новом входе с устройства.

**Права пациента:**
- read-only: приёмы (timeline), анализы, документы (PDF);
- write: своё сообщение врачу, запись на приём, отзывы;
- свои согласия (даёт/отзывает);
- свой журнал доступа («кто смотрел мою карту»).

**Волна:** B42.

### Модуль messages_patient — переписка врач↔пациент

**Назначение:** асинхронная переписка между пациентом и врачом
через защищённый ЛКП.

**Ключевой принцип:** один тред на пару (врач, пациент) — приватность.
Врач B не видит тред врача A с тем же пациентом.

**Модель данных:**

    patient_threads
      id, patient_id, doctor_id, created_at, updated_at,
      last_message_id, archived_by_patient, archived_by_doctor

    patient_messages
      id, thread_id, sender_kind, sender_patient_id, sender_doctor_id,
      body, kind, priority, created_at, read_at, deleted_at

    patient_message_attachments
      id, message_id, filename, mime_type, size, file_path, sha256

**Правила:**
- врач видит тред только если есть encounter с пациентом;
- admin клиники НЕ видит содержимое (только метаданные);
- аудит: `patient_message.sent`, `.read` — без содержимого;
- медсодержание — только в защищённом канале (не в SMS/email).

**API (врач):**

    GET    /api/patient-messages/threads
    GET    /api/patient-messages/threads/{id}
    POST   /api/patient-messages/threads/{id}/messages
    POST   /api/patient-messages/messages/{id}/read

**API (пациент):**

    GET    /api/patient/threads
    GET    /api/patient/threads/{id}
    POST   /api/patient/threads/{id}/messages

**Волна:** B43.

### Модуль notifications — оповещения пациентов

**Назначение:** единый слой событий → каналов → получателей.
НЕ отдельные интеграции SMS/email/push, а один orchestrator.

**Ключевой принцип:** если `medical_content=true` — только in-app
(пациент сам заходит в защищённый кабинет). В SMS/email/push —
только нейтральные («есть новое сообщение», «результат готов» + ссылка).

**Каналы:**

| Канал | Провайдеры | Default |
|---|---|---|
| in-app | `patient_messages` | включён |
| Web Push | VAPID (браузер) | включён |
| Email | SMTP / SendGrid / Yandex | опционально |
| SMS | SMS.ru / SMSC / Mango | опционально |
| MAX | MAX Bot API (max.ru) | опционально, РФ |
| VK | VK Bot API | опционально, РФ |

**Мессенджеры WhatsApp/Telegram** — не используем (152-ФЗ: юрисдикция
вне РФ, данные уходят третьим лицам). MAX / VK — допустимы (юрисдикция
РФ, но требуется договор поручения на обработку ПД).

**Модель данных:**

    notification_events
      id, code UNIQUE, title_template, body_template, sms_template,
      default_priority, default_channels_csv, medical_content,
      active, editable_by_admin, created_at, updated_at

    patient_notification_prefs
      patient_id PK, sms_enabled, email_enabled, push_enabled,
      preferred_channel, quiet_hours_from, quiet_hours_to, updated_at

    notification_queue
      id, event_code, recipient_kind, recipient_id, channel,
      payload_json, scheduled_at, attempt_count, max_attempts,
      status, last_error, created_at, updated_at, sent_at,
      provider_msg_id

    notification_log
      id, queue_id, event_code, recipient_kind, recipient_id,
      channel, provider, status, provider_msg_id, error, occurred_at

    webpush_subscriptions
      id, patient_id, endpoint, p256dh, auth, user_agent,
      created_at, last_used_at, revoked_at

**Приоритеты и каналы (примеры):**

| Код события | Приоритет | Каналы | Мед-контент |
|---|---|---|---|
| appointment_reminder_24h | high | sms, in-app | нет |
| appointment_reminder_2h | critical | sms | нет |
| new_message_from_doctor | normal | push, in-app | нет |
| lab_result_ready | high | sms, in-app | нет |
| document_issued | normal | email, in-app | нет |
| portal_invite | critical | sms, email | нет |
| password_reset_patient | critical | sms, email | нет |
| diagnosis_discussion | high | in-app only | да |

**Провайдер-интерфейс:**

    type Provider interface {
        Name() string
        Channel() string
        Send(ctx, recipient, msg) (DeliveryResult, error)
        Health(ctx) error
    }

Реализации: providers/sms/smsru.go, smsc.go, mango.go;
providers/email/smtp.go, sendgrid.go, yandex.go;
providers/webpush/webpush.go; providers/inapp/inapp.go;
providers/max/max.go; providers/vk/vk.go.

**Очередь (worker):**
- отдельная goroutine, batch 50;
- backoff: 1м, 5м, 30м (3 попытки);
- scheduled_at — для отложенных (reminders);
- при отмене приёма — задачи → cancelled;
- дедупликация по hash(recipient, event_code, entity_id, day);
- rate-limit: 3 SMS/сутки на пациента + 60 SMS/мин глобально.

**API модуля (внутреннее, для других модулей):**

    notifications.Send(ctx, SendRequest{
        EventCode, RecipientKind, RecipientID,
        Payload, EntityKind, EntityID,
        ScheduledAt, Priority, Channels,
    })

Если notifications выключен — no-op, логирует.

**Retention:**
- `notification_log` — 365 дней;
- `notification_queue` — sent/failed старше 30 дней удаляются.

**Env:**
- BOTMAX_SMS_PROVIDER=none|smsru|smsc|mango
- BOTMAX_EMAIL_PROVIDER=none|smtp|sendgrid|yandex
- BOTMAX_MAX_BOT_TOKEN=...
- BOTMAX_VK_BOT_TOKEN=...
- BOTMAX_WEBPUSH_VAPID_PUBLIC/PRIVATE/SUBJECT
- BOTMAX_NOTIFICATION_RATE_LIMIT_SMS_PER_DAY=3
- BOTMAX_NOTIFICATION_LOG_RETENTION_DAYS=365

**Волна:** B43.5.

### Модуль calls — внутренние звонки

**Назначение:** аудио-звонки 1-на-1 между врачами, врач↔пациент.

**Технология:** WebRTC (DTLS-SRTP шифрование), signaling — на нашем
Go-бэкенде через WebSocket, TURN — self-hosted coturn.

**Модель данных:**

    calls
      id, initiator_doctor_id, initiator_patient_id,
      callee_doctor_id, callee_patient_id,
      kind, status, started_at, accepted_at, ended_at,
      duration_seconds, end_reason,
      recording_url, recording_consent_at

**Signaling-протокол (WebSocket /api/calls/signal):**

Клиент → сервер:
- call_initiate, call_accept, call_decline, call_hangup
- sdp_offer, sdp_answer, ice_candidate

Сервер → клиент:
- incoming_call, call_accepted, call_ended
- sdp_offer, ice_candidate

**Логика сервера:**
- map[doctorID]*Conn — активные WS-соединения;
- call_initiate → проверка онлайн → incoming_call;
- таймаут 30 сек → status=missed.

**Окружение:**
- BOTMAX_STUN_URLS (default: stun:stun.l.google.com:19302 — ТОЛЬКО dev);
- BOTMAX_TURN_URLS + BOTMAX_TURN_SECRET — обязательны для prod;
- BOTMAX_CALL_RING_TIMEOUT_SECONDS=30.

**Опциональные подмодули:**

| Модуль | Что | Default |
|---|---|---|
| calls | WebRTC audio 1-на-1 | off |
| calls_video | + видео | off |
| calls_recording | + запись (с согласия обеих сторон) | off |
| telephony | SIP/Asterisk — звонки на телефоны | off |

**UI:**
- иконка 📞 в inbox и в списке врачей;
- модалка входящего звонка;
- панель активного звонка в правом нижнем углу;
- вкладка «Звонки» в inbox (история).

**Волна:** B44.

### Монетизация и лицензирование

**Модель: Open Core + Enterprise-модули по лицензии.**

**Community Edition (бесплатно):**
- базовый набор: auth, doctors, patients, encounters, audit,
  health, metrics;
- 1 клиника, ограниченное число врачей;
- без SMS/email-провайдеров (только in-app);
- community support.

**Enterprise Edition (платно):**
- все модули по подписке;
- multi-clinic;
- SLA + приоритетная поддержка;
- приоритет обновлений;
- интеграции (СФР, ЛИС, ЕГИСЗ) — опционально.

**Пакеты модулей:**

| Пакет | Модули |
|---|---|
| Клинический | referrals, labs, prescriptions, certificates, sick_leave |
| Коммуникации | messages, notifications (+ SMS/email/MAX/VK) |
| Пациентский портал | patient_portal, messages_patient |
| Звонки | calls, calls_video, calls_recording |
| Телефония | telephony (SIP/Asterisk) |
| Аналитика | reports_analytics |
| Коммерция | scheduling, billing, inventory, pharmacy |

**Лицензирование:**
- **JWT-ключ** с подписью (offline, без online-проверки);
- содержит: список модулей, лимиты (врачи, клиники), срок;
- env: `BOTMAX_LICENSE_FILE=/etc/botmax/license.jwt`;
- endpoint `GET /api/license` — что доступно (для UI);
- при старте: пересечение `BOTMAX_MODULES` и `license.Modules`;
- fail-fast при истёкшей/неполной лицензии.

**Прайсинг-гипотеза (Enterprise, self-hosted):**
- до 5 врачей: 60 тыс ₽/год;
- до 20 врачей: 180 тыс ₽/год;
- до 50 врачей: 400 тыс ₽/год;
- 50+: индивидуально.

**Дополнительные источники:**
- внедрение под ключ: 50–200 тыс ₽;
- интеграции под заказ: 100–500 тыс ₽/проект;
- SLA 99.9% + реакция ≤4ч: +20% к лицензии/год;
- white-label для сетей: от 1 млн ₽/год;
- обучение: 5–15 тыс ₽/чел.

**Техзакладки (сделать в B33):**
- `BOTMAX_LICENSE_FILE`, `/api/license`;
- `modules.LicensedModules()` — пересечение;
- телеметрия (opt-in, анонимная) — что используется, что продавать.

### Расширенный ЛКП — волны B43.6–B55

Помимо базового ЛКП (B42) и переписки (B43):

| Волна | Что |
|---|---|
| B43.6 | Медицинский профиль пациента (аллергии, хронические, лекарства) + верификация врачом |
| B43.7 | Timeline здоровья — единая хронология всех событий |
| B43.8 | Семейный доступ (родитель→дети, опекун с документом) |
| B43.9 | Журнал доступа пациента к своей карте (read-аудит, «кто смотрел») |
| B43.10 | Экспорт всех данных (PDF + JSON + CSV) по 152-ФЗ |
| B45 | scheduling — расписание и онлайн-запись из ЛКП |
| B46 | patient_vitals — домашние измерения (АД, вес, глюкоза) |
| B47 | feedback_patient — отзывы и оценки |
| B48 | billing_patient — онлайн-оплата, чеки (54-ФЗ) |
| B49 | pre_visit — анкеты и чек-листы до приёма |
| B50 | patient_reminders — расширенные напоминания (лекарства, скрининги) |
| B51 | vaccination_certificates — прививочный сертификат с QR |
| B52 | pharmacy — заказ лекарств в аптеке-партнёре |
| B53 | ai_patient_assist — AI-объяснение анализов (с модерацией врачом) |
| B54 | PWA (offline + install) |
| B55 | mobile_app — мобильное приложение (RN/Flutter) |

**Принципы UX ЛКП:**
- mobile-first, крупный шрифт (16px+), WCAG AA;
- главный экран — «Сейчас»: ближайший приём, непрочитанные, напоминания;
- явный выход, сессия 30 мин, 2FA опционально;
- «Позвонить в клинику» всегда в шапке;
- журнал входов (устройство, IP, время).

**Что НЕ делать:**
- соцсеть, лайки, комментарии;
- wellness-дневник;
- AI-диагнозы (юридическая катастрофа);
- Google Analytics / Yandex Metrica (данные пациентов);
- реклама.

### Полный список модулей BOTMAX_MODULES

**Коммуникации:**
messages, notifications, calls, calls_video, calls_recording, telephony

**Клинические:**
referrals, labs, templates, prescriptions, certificates, sick_leave,
vaccinations, inventory

**Пациентские:**
patient_portal, messages_patient, scheduling, patient_vitals,
feedback_patient, billing_patient, pre_visit, patient_reminders,
vaccination_certificates, pharmacy, ai_patient_assist

**Административные:**
reports_analytics, billing, multi_clinic, queue_management

**Интеграции (опционально, отдельные проекты):**
integrations_lis, integrations_ehr, integrations_sfr, integrations_egisz

**Базовые (всегда включены, не в BOTMAX_MODULES):**
auth, doctors, patients, encounters, audit, health, metrics, settings

### Дополнительные модули (фаза 2/3)

| Код | Цель | Приоритет |
|---|---|---|
| scheduling | расписание, онлайн-запись | высокий |
| billing | касса, услуги, чеки, эквайринг | высокий |
| reports_analytics | отчёты, дашборды для владельца | высокий |
| inventory | склад медикаментов, срок годности | высокий |
| vaccinations | прививочный календарь РФ | высокий |
| integrations_lis | Инвитро/Гемотест/Хеликс API | средний |
| integrations_ehr | HL7 FHIR, ЕГИСЗ | средний |
| ai_assist | ассистент врача (диагнозы, суммаризация) | средний |
| pharmacy | связь с аптеками, МДЛП | средний |
| dms | ДМС-интеграция, счета, реестры | средний |
| queue_management | электронная очередь, табло | средний |
| feedback | отзывы, NPS | средний |
| referral_network | сеть клиник-партнёров | низкий |
| school_medical | медкабинеты в школах/садах | низкий |
| insurance_reports | отчёты для страховых | низкий |
| regulatory_reports | формы 12, 30, 14 (Минздрав) | низкий |
| mobile_app | мобильное приложение | средний |
| i18n | международный рынок (KZ, BY, AM) | низкий |

### Обновлённые волны (сводка)

**Фаза 1 (MVP, B27–B44):** password reset, encounters, история,
экспорт, guest, clinics, modules, messages, consents, referrals,
labs, templates, prescriptions, certificates, sick_leave,
patient_portal, notifications, calls.

**Фаза 2 (Commercial, B45–B55):** scheduling, billing,
reports_analytics, inventory, vaccinations, ЛКП-расширения,
лицензирование, multi-clinic.

**Фаза 3 (Advanced, B56+):** ai_assist, integrations_lis,
integrations_ehr, telephony, pharmacy, dms, mobile_app, i18n,
regulatory_reports.
    encounter создаётся пустым.

---

## Правила
- 1 подпатч = 1 коммит (или несколько — если подпатч намеренно разбит на части, как 6.10).
- Перед завершением подпатча — запись в этом файле: что сделано + следующий номер.
- Никогда не коммитить без зелёных `go vet ./...`, `go build ./backend`, `go test ./backend/...`.
- Никогда не пушить в prod без проверки на localhost.


## Уроки сессий (рабочая память)

Раздел накапливается. Сверяться перед началом каждой сессии.

### bash / git / python

- Heredoc в интерактиве: мелкие куски по 3–8 строк, делимитер в кавычках
  (<<'EOF'). Крупный heredoc — риск залипания, отменять нельзя.
- `cat >` пишет пробелы, `gofmt -w` возвращает табы. Прогонять после
  каждого нового .go-файла.
- Python-правки: `assert s.count(old) == N` перед replace — ловит
  «нашёл не то» до записи. Спасал дважды (пробелы vs табы, неверный якорь).
- После правок номера строк сдвигаются. Ориентир — содержимое (grep -n),
  не номера.
- `git diff --stat` пустой = правок нет. Молчаливый go build —
  не доказательство. Смотреть diff глазами.
- `git diff` открывается в пейджере (двоеточие внизу). `q` — выход,
  или `git --no-pager diff` для коротких.
- Отступы в Go — табы (gofmt). Python-паттерн должен использовать \t,
  не пробелы, иначе count == 0.

### Тесты

- Хендлеры читают пакетную db (main.go:382). В тестах подменять:
  saved := db; db = newTestDB(t); t.Cleanup(func(){ db = saved }).
  Паттерн — doctors_test.go:TestHandleDoctorsList_RoleFilter.
- Хелпер подмены — без возврата: setupConsentDB(t). Работаем через
  пакетную db (симметрия с doctors_test.go).
- Стиль HTTP-тестов: httptest.NewRequest(...).WithContext(ctx) +
  httptest.NewRecorder() + h.ServeHTTP.
- Роль в контекст: withDoctorID(ctx, id, RoleDoctor) — 3 аргумента,
  withPatientID(ctx, id) — 2 аргумента.
- gofmt -l backend/ и go vet ./backend/ — молчание = чисто.

### Аудит данных

- Nullable в Scan: смотреть тип Go-поля, не греп. sql.Null* — ок;
  string/int/float64 + nullable-колонка — дефект; COALESCE в SQL — ок.

- **B30.4 (sync):** формат timestamp'ов в проекте — `"2006-01-02 15:04:05"` (`tsLayout`), НЕ RFC3339. Для TEXT-колонок `sql.NullTime` не подходит — брать `sql.NullString` + `time.Parse(tsLayout, ...)`. Ошибка парсинга → 500, не 403. Тесты — прямой вызов handler'а через `httptest.NewRequest(...).WithContext(withDoctorID(...))`, без `NewServer` (образец `consent_test.go`).

## §6.59 — B30.1 + B30.2: инфраструктура auth пациента (14.10.2026)

### Что сделано

**B30.1 — схема (коммит `909843a`):**
- `patient_accounts` — аккаунт пациента (login, password_hash, FK на patients)
- `consents` — согласие patient ↔ doctor (scope, status pending/approved/revoked)
- `consents_audit` — append-only журнал согласий, без FK (переживает удаление consent)
- 6 индексов, все FK с CASCADE на DELETE

**B30.2 — auth-контур пациента (коммит `b629ef2`):**
- `patient_refresh_tokens` — отдельная таблица (FK на `patient_accounts`, не на `patients`)
- `RolePatient Role = "patient"` — добавлен в `auth.go` + `Valid()`
- `cookies.go`: `botmax_pat_access` / `botmax_pat_refresh` + `setPatientCookies` / `clearPatientCookies`
- `backend/patient_auth.go` (новый файл, ~450 строк):
  - `handlePatientLogin` / `handlePatientRefresh` / `handlePatientLogout`
  - `patientAuthMiddleware` — hard-mode всегда (soft-режим запрещён)
  - `patientIDFromCtx`, `withPatientID`, `ctxKeyPatientID`
  - Модель `PatientAccount`, CRUD на `patient_refresh_tokens`
- `main.go`: `registerPatientAuthRoutes()` (3 роута)
- `auth.go`: `isAuthWhitelist` расширен на `/api/patient-auth/*`

### Зафиксированные архитектурные решения

- **Consent — отдельная таблица, не audit_log.** `consents_audit` append-only, без FK.
- **`Claims.DoctorID` = subjectID.** Поле исторически называется `doctorID`, для пациента = `patientAccountID`. Не переименовываем — сломает `authMiddleware`, `impersonation`, `requireRole`.
- **Auth-events пациента — только slog, не в audit_log.** Audit_log — врачебный журнал. Если понадобится аудит пациента — отдельная таблица `patient_audit`.
- **Patient-middleware — hard-mode всегда.** Soft-режим недопустим (пациент иначе прочитает чужие данные).
- **`patient_refresh_tokens` — отдельная таблица.** Не переиспользуем `refresh_tokens` (там FK на `doctors.id`).
- **Cookies пациента отделены от врачебных.** Врач и пациент могут быть залогинены в одном браузере.

### Что осталось в B30

- **B30.2b — регистрация пациента:** миграция `patients.doctor_id → NULL` + `POST /api/patient-auth/register` (пациент вводит `patient_id` от врача + свой login/password).
- **B30.3 — consent-flow:** миграция `consents` (+ `requested_by`, `denied_at`, `expires_at`) + 5 роутов (`request`, `approve`, `deny`, `revoke`, `list`) + врачебный UI для запроса consent.
- **B30.4 — sync:** `GET /api/patients/{id}/sync` — врач получает vitals_history при approved consent, с учётом scope.
- **B30.5 — UI пациента:** личный кабинет (ведёт свои vitals) + индикатор sync у врача.

### Точка входа для следующей сессии

```bash
git log --oneline -3
# ожидаем:
#   <hash> docs(snapshots): 6.59 — B30.1 + B30.2 закрыты
#   06eac41 chore(gitignore): ...
#   b629ef2 feat(auth): B30.2 — auth-контур пациента ...

git status -sb
# ожидаем: ## main...origin/main

# напомни: стоим на старте B30.2b (регистрация) или B30.3 (consent),
# оба — большие волны с миграцией существующих таблиц.
# Перед началом — БЭКАП bot_max.db (make backup).


## 6.60 — fix FK via DSN, подготовка B30.2b

### Сделано

- **fix(db):** PRAGMA `foreign_keys` передаётся через DSN, а не через `db.Exec`.
  `db.Exec("PRAGMA ...")` действовало только на одно соединение пула,
  остальные открывались с FK=OFF. Скрытый баг: каскады не работали в проде.
- **Тесты:**
  - `TestInitDB_ForeignKeysOnAllConns` — FK=1 на 5 подряд соединениях.
  - `TestInitDB_CascadeDeleteOnDoctor` — каскадное удаление пациентов.

### Побочно вскрыто

- Старые тесты полагались на FK=OFF (висячие ссылки вставлялись без ошибок).
- Старые тесты полагались на наличие `DOC-001` после `InitDB` — а сид
  вызывается отдельно, из `seed.go` / `main`.

### Заметки на будущее

- `newTestDB` использует `:memory:` + `database/sql` пул. Де-факто
  работает с одним соединением. Может флакать при параллельных тестах.
  Отдельная задача: `SetMaxOpenConns(1)` или переезд на `t.TempDir()`.
- `make backup` показывает ложное `⚠️ Сервер запущен`. Причина
  подтверждена: `pgrep -f /tmp/botmax_server` (Makefile:101) матчит
  родительский `sh -c`, в чьей командной строке тот же паттерн.
  Фикс: заменить проверку на `fuser 8082/tcp` — функциональная
  проверка (занят ли порт), не зависит от имён процессов.

### Точка старта B30.2b

- FK работает end-to-end, миграция будет валидной.
- Схема `patients` зафиксирована (`/tmp/schema_patients_before.sql`).
- SQL-скрипт миграции проверен на копии БД: данные сохраняются,
  FK дочерних таблиц перепривязываются корректно.
- `bot_max.db` не менялась с 14.10 11:57 — «холодная» БД для миграции.
- Бэкап: `backups/bot_max_20261015_061824` (252K).

## 6.61 — B30.2b: patients.doctor_id → NULLABLE + ON DELETE SET NULL

### Сделано

- **feat(db):** `migratePatientsDoctorIdNullable` — идемпотентная миграция
  пересоздания таблицы `patients`:
  - `doctor_id TEXT NOT NULL` → `doctor_id TEXT` (nullable)
  - FK `ON DELETE CASCADE` → `ON DELETE SET NULL`
  - Порядок колонок 1:1 → `INSERT INTO patients_new SELECT *` безопасен.
  - Через `db.Conn(ctx)` — **единое** соединение. `PRAGMA foreign_keys = OFF`
    per-connection, на `db.Exec` применилось бы к случайному соединению пула
    (§6.60).
  - `PRAGMA foreign_key_check` **внутри** транзакции → Rollback при нарушениях.
  - Идемпотентность: `PRAGMA table_info(patients).doctor_id.notnull == 0` → no-op.

- **Семантика:** удаление врача больше не сносит пациентов каскадом.
  Раньше `CASCADE` → удалялись patients + вся история через 6 дочерних таблиц
  (`cards`, `encounters`, `vitals`, `vitals_history`, `consents`,
  `patient_accounts`). Теперь `SET NULL` → `patients.doctor_id = NULL`,
  история сохраняется.

- **Тесты (backend/db_test.go):**
  - `TestInitDB_CascadeDeleteOnDoctor` → `TestInitDB_SetNullOnDoctorDelete`
    (ассерция обратной семантики).
  - +5 новых `TestMigrate_DoctorIdNullable_*` — Applied / Idempotent /
    DataPreserved / FKOnDeleteSetNull / IndexesRecreated.
  - Через новый хелпер `newFileTestDB` (файловая БД в `t.TempDir()`),
    т.к. `:memory:` несовместим с `db.Conn(ctx)` — каждое соединение пула
    видит свою независимую БД.
  - `TestInitDB_ForeignKeysOnAllConns` — регрессия §6.60 не сломана.

- **Smoke-тест** `backend/smoke_migration_test.go` (`//go:build smoke`) —
  прогон миграции на **копии** реальной `bot_max.db`.
  Запуск: `SMOKE_DB=/tmp/bot_max.smoke.db go test -tags smoke -run TestSmoke -v`.
  Обычный `go test ./...` его не видит.

### Проверки

- `go test ./...` — ok (7.6s).
- Целевой прогон 7 тестов — все PASS.
- Smoke на копии `bot_max.db`:
  - `doctor_id` → nullable ✅
  - FK `ON DELETE SET NULL` в `.schema patients` ✅
  - 7/7 пациентов сохранены ✅
  - `foreign_key_check` — пусто ✅
  - Индексы `idx_patients_doctor`, `idx_patients_clinic` пересозданы ✅
  - Удаление доктора при FK=ON → все 7 пациентов `doctor_id = NULL`,
    дочерние таблицы не тронуты ✅

### Побочно вскрыто

- **`sqlite3` CLI по умолчанию `PRAGMA foreign_keys = OFF`.**
  Первая проверка семантики на копии показала висячие ссылки
  (пациенты с `doctor_id='DOC-001'` после `DELETE FROM doctors`).
  Причина: новое соединение CLI, FK не активирован. **Правильно:**
  всегда `PRAGMA foreign_keys = ON;` первой строкой в `sqlite3`-сеансе.
  Это не баг миграции, но важно для будущих CLI-смоуков.

- **`encounters.doctor_id` имеет `ON DELETE RESTRICT`** (db.go:111).
  Из-за этого `DELETE FROM doctors` падает с
  `FOREIGN KEY constraint failed (19)`, пока есть encounters.
  У нас в копии: 1 encounters → удаление доктора **блокируется**.
  `patients` освобождаются только если encounters не мешают.

### Следующая задача — B30.2c

- **`encounters.doctor_id → NULLABLE + ON DELETE SET NULL`** — по образцу
  B30.2b. Сейчас `RESTRICT` — эпизоды приёмов **блокируют** удаление врача.
  Логично: врач уволился → encounters остаётся историей, просто без врача.
  Паттерн отлажен (§6.61), ждём только применения.

### Технические заметки

- `patients_new` DDL — копия оригинального DDL с двумя изменениями
  (`doctor_id TEXT` без `NOT NULL`, FK `SET NULL`). Остальные 19 колонок
  **байт-в-байт** — иначе `SELECT *` не скомпилируется семантически.
- FK-ссылки **дочерних** таблиц (`cards`, `encounters`, `vitals`,
  `vitals_history`, `consents`, `patient_accounts`) на `patients.id`
  **не пересоздаются** — `ALTER TABLE patients_new RENAME TO patients`
  сохраняет их валидность по имени.
- `sqlite_autoindex_patients_1` (от PRIMARY KEY) пересоздаётся автоматически
  вместе с таблицей.
- `patients` не имеет `AUTOINCREMENT` → `sqlite_sequence` не затрагивается.

### Точка входа на следующую сессию

    cd /workspaces/BOT_MAX-main
    git log --oneline -3                    # должен быть merge B30.2b в main
    git status -sb
    grep -n "6.61" SNAPSHOTS.md             # этот раздел
    make backup                             # фикс из 6.60 — уже работает
    # далее — B30.2c: encounters.doctor_id → NULLABLE + ON DELETE SET NULL

- `bot_max.db` **не тронута** миграцией (работали на копиях).
- Smoke-инфраструктура готова: `smoke_migration_test.go`, `//go:build smoke`.
- В `/tmp/botmax_rollback_point/bot_max.db` — эталон отката, `integrity_check=ok`.

## 6.62 — B30.2c: encounters.doctor_id → NULLABLE + ON DELETE SET NULL

**Дата:** 2026-10-15
**Коммит:** b1de072
**Контекст:** прямое продолжение §6.61 (B30.2b). Тот же класс миграции
для таблицы encounters — эпизоды приёмов больше не блокируют удаление врача.

### Что сделано

- `migrateEncountersDoctorIdNullable` — rebuild таблицы encounters:
  `doctor_id TEXT` (было NOT NULL), FK `ON DELETE SET NULL` (было RESTRICT).
  Паттерн 1:1 с B30.2b: PRAGMA foreign_keys OUTSIDE транзакции на выделенном
  conn; rebuild в tx; foreign_key_check до Commit; идемпотентность через
  PRAGMA table_info.
- `encountersDoctorIdIsNullable` — проверка notnull == 0.
- Вызов в `Open()` сразу после `migratePatientsDoctorIdNullable` (`db.go:315`).
- `encounters.go`: 3 read-запроса переведены на `COALESCE(doctor_id, '')`:
  - `getEncounterByID` (стр. 343)
  - `encounterSelectCols` (стр. 371)
  - `getLatestEncounterForPrefill` (стр. 702)
- Индексы idx_enc_patient, idx_enc_doctor, idx_enc_clinic,
  idx_enc_patient_kind пересозданы в шаге rebuild.

### Проверено на боевой БД (bot_max.db)

- `.schema encounters`: `doctor_id TEXT,` (без NOT NULL);
  FK `doctor_id → doctors(id) ON DELETE SET NULL`.
- `PRAGMA foreign_key_check` до и после — пусто.
- `PRAGMA integrity_check` — ok.
- `PRAGMA index_list(encounters)` — 4 индекса idx_enc_* + autoindex PK.
- Внешних FK на encounters нет (§6.61 прогноз подтверждён).
- Триггеров на encounters нет.
- Идемпотентность: повторный `make run` — миграция no-op, схема не меняется.

### Известные долги (не блокеры)

**B30.2d — NullString для Patient/Encounter.**
B30.2b и B30.2c применили миграцию на уровне SQL, но Go-модели
(`Patient.DoctorID`, `Encounter.DoctorID`) остались `string`. Реальный NULL
из БД читается через `COALESCE(doctor_id, '')` в SQL. Правильное решение —
`sql.NullString` + кастомный `MarshalJSON` с сохранением JSON-контракта
(`"doctor_id": ""` при NULL). Триггер долга: первое реальное удаление врача
с привязанными пациентами или encounters. Приоритет — средний (данных с
NULL сейчас нет).

**B30.2e — синхронизация DDL в `db.go`.**
В `db.go:64, 93, 112` DDL CREATE TABLE всё ещё объявляет
`doctor_id TEXT NOT NULL` (и `ON DELETE RESTRICT` для encounters), хотя
миграции пересоздают таблицы в NULLABLE+SET NULL. Для свежей БД схема
корректна после миграции, но «источник истины» (DDL) рассинхронизирован с
фактическим состоянием. Нужно обновить DDL до NULLABLE+SET NULL, чтобы
миграции стали no-op на новых инсталляциях. Приоритет — низкий.

### Откат

`backups/bot_max_20261015_135947` — снимок до миграции B30.2c.
`PRAGMA integrity_check` — ok. Восстановление: `cp backups/bot_max_20261015_135947 bot_max.db`
при остановленном сервере.

### Точка входа следующей сессии

    cd /workspaces/BOT_MAX-main
    git log --oneline -3
    git status -sb
    grep -n "6.62" SNAPSHOTS.md
    make backup
    # далее — B30.2d (NullString для Patient/Encounter)
    #      или B30.2e (синхронизация DDL в db.go)
    # обе — в разделе «Известные долги» §6.62
```

---

## 6.63 — B30.2d: COALESCE(doctor_id, '') в SELECT patients

**Дата:** 2026-10-15
**Коммит:** 805a10d
**Контекст:** закрытие мины, оставшейся после §6.61 (B30.2b). Прямое
продолжение §6.62 (B30.2c) — тем же паттерном, но для таблицы patients.

### Что сделано

- `getPatientsByDoctor` (`db.go:383`): `SELECT id, doctor_id, COALESCE(clinic_id, ...)`
  → `SELECT id, COALESCE(doctor_id, ''), COALESCE(clinic_id, ...)`.
- `getPatientByID` (`db.go:461`): та же замена.
- `Patient.DoctorID` остаётся `string` (`patients.go:16`). Кастомный
  `MarshalJSON` не вводится — симметрия с B30.2c важнее семантически
  «честного» null в JSON.
- Правка — 2 строки, 1 файл. `git diff` — 2 вставки, 2 удаления.

### Почему не NullString + MarshalJSON

Исходный брифинг предлагал `sql.NullString` + кастомный `MarshalJSON`.
Отказались в пользу симметрии с B30.2c (`COALESCE(doctor_id, '')`):
в проекте уже принят этот паттерн для encounters, вводить второй способ
решения того же класса проблем — плодить зоопарк. Единый паттерн > «честный»
null в JSON. Если когда-нибудь понадобится null в API — это отдельная задача
(B30.3: унификация nullable-полей), сразу для Patient + Encounter + всех
остальных.

### Проверено на боевой БД (bot_max.db)

- `SELECT COUNT(*) FROM patients WHERE doctor_id IS NULL` = **0** на момент
  правки. Мина была заряжена, но не выстрелила.
- `go build ./...` — чисто.
- `go test ./...` — `ok BOT_MAX/backend 7.587s`.
- Воспроизведение мины на копии БД (`/tmp/bot_max_test.db`):
  `UPDATE patients SET doctor_id = NULL WHERE id = 'PAT-001';`
  `SELECT id, COALESCE(doctor_id, ''), name FROM patients WHERE doctor_id IS NULL;`
  → `PAT-001||Иванов Петр Александрович`.
  До правки Go-драйвер на этой строке вернул бы
  `converting NULL to string is unsupported` → 500 на `GET /api/patients`.
  После — 200 с `"doctorId": ""`.

### Закрытие B30.2d из §6.62

Блок B30.2d в «Известных долгах» §6.62 помечен как ЗАКРЫТО — см. эту секцию.
Приоритет «средний» снят: правка выполнена до первого реального удаления
врача с пациентами.

### Остаётся долг B30.2e

Синхронизация DDL в `db.go:64, 93, 112` — всё ещё `doctor_id TEXT NOT NULL`
и `ON DELETE RESTRICT`, хотя миграции пересоздают таблицы в NULLABLE+SET NULL.
Приоритет низкий, косметика. Не блокер.

### Откат

`backups/bot_max_20261015_154007` — снимок перед этой сессией. Восстановление:
`cp <backup> bot_max.db` при остановленном сервере. Правка — уровня SQL-строки,
для отката достаточно `git revert 805a10d` — модель и JSON-контракт не затронуты.

### Точка входа следующей сессии

    cd /workspaces/BOT_MAX-main
    git log --oneline -3
    git status -sb
    grep -n "6.63" SNAPSHOTS.md
    make backup
    # далее — B30.2e (синхронизация DDL в db.go:64, 93, 112)
    #      или B30.3 (аудит остальных nullable-полей: Scan без COALESCE)
    # B30.2d закрыт, см. §6.63

---

## 6.64 — B30.2e: синхронизация DDL patients/encounters с миграциями

**Дата:** 2026-10-15
**Коммит:** c31b0e6
**Контекст:** последний долг из §6.62. DDL CREATE TABLE в `db.go`
рассинхронизировался с фактическим состоянием после B30.2b/c.

### Что сделано

- `db.go:64`  patients.doctor_id:   `TEXT NOT NULL` → `TEXT`
- `db.go:83`  patients FK:          `ON DELETE CASCADE` → `ON DELETE SET NULL`
- `db.go:93`  encounters.doctor_id: `TEXT NOT NULL` → `TEXT`
- `db.go:112` encounters FK:        `ON DELETE RESTRICT` → `ON DELETE SET NULL`

4 строки в `backend/db.go`. Миграции оставлены как есть — на свежих БД
они станут no-op благодаря PRAGMA table_info-идемпотентности.

### Сюрприз: строка 83 (patients FK)

Брифинг §6.62 упоминал 64/93/112, но пропустил строку 83 —
`patients` FK был `ON DELETE CASCADE`, хотя в боевой БД после B30.2b стоит
`ON DELETE SET NULL`. Без правки свежая инсталляция получила бы
**каскадное удаление пациентов** при удалении врача. Обнаружено при
сверке `.schema patients` с боевой БД. Урок: DDL-аудит нужно делать по
фактической схеме, а не по списку из брифинга.

### Проверено

- `go build ./...` + `go test ./...` — `ok BOT_MAX/backend 7.341s`.
- Свежая БД: `BOTMAX_DB_PATH=/tmp/bot_max_fresh.db /tmp/botmax_server`.
  Лог: `msg="SQLite готова" dbPath=/tmp/bot_max_fresh.db`,
  seed DOC-001/SUPER-001/ADMIN-001, graceful shutdown.
- `diff` .schema с боевой:
  - `patients` — расхождение только `IF NOT EXISTS` (теряется при rebuild);
  - `encounters` — то же + двойные пробелы в тексте индексов.
  - **doctor_id и FK — идентичны** в обеих.
- Итог: DDL на свежей БД даёт схему, эквивалентную миграциям B30.2b/c.
  Миграции станут no-op на новых инсталляциях.

### B30.2e закрыт

После B30.2e все долги из §6.62 закрыты. Остаётся не-долг из обсуждения:
B30.3 — аудит остальных nullable-полей (`Scan` без `COALESCE`) в других
таблицах. Страховка, не мина. Приоритет — низкий.

### Откат

`git revert c31b0e6` — правка изолирована, модель/JSON не затронуты.

### Точка входа следующей сессии

    cd /workspaces/BOT_MAX-main
    git log --oneline -3
    git status -sb
    grep -n "6.64" SNAPSHOTS.md
    make backup
    # далее — B30.3 (аудит остальных nullable-полей: Scan без COALESCE)
    #      или новая задача
    # все долги §6.62 закрыты

## 6.65 — B30.3: аудит nullable-полей в Scan без COALESCE

**Статус:** ✅ закрыт (2026-10-16)

**Коммиты:**
- `cac3a1b` — fix(db): COALESCE для nullable-полей в vitals/vitals_history/cards
- `acf1213` — test(patient_auth): B30.3 — регрессия на nullable-поля в Scan

**Что сделано:**
Проведён аудит всех 16 мест `Scan(` в `backend/` на предмет сканирования
nullable-колонок в примитивные Go-типы (`string`/`int`/`float64`) без
защиты. Найдено 3 дефекта, все исправлены:

| Место | Проблема | Фикс |
|---|---|---|
| `db.go:625` getVitals | `vitals.bp/temp/spo2/updated` — все nullable | `COALESCE(...,'')` / `,0` |
| `db.go:673` getVitalsHistory | `vitals_history.bp/temp/spo2/recorded` — все nullable | то же |
| `db.go:704` getCard | `cards.data/updated` — оба nullable | `COALESCE(...,'')` |

**Уже безопасны (не трогали):**
- `patient_auth.go:89, 109` — `LastLogin sql.NullString` (штатный паттерн)
- `password_reset.go:88` — `usedAt sql.NullString`
- `password_reset.go:268` — `lockedUntil sql.NullString`
- `doctors.go:263` — `phone/clinicID/created/lastLogin sql.Null*`
- `audit.go:185` — `entity/entityID/ip/detail sql.NullString`
- `refresh.go:72` — `revoked_at sql.NullString`
- `encounters.go` — `COALESCE(doctor_id,'')`, `COALESCE(clinic_id,'')`, `COALESCE(finished_at,'')` в SQL
- `db.go:467` — `patients` — все nullable через `COALESCE` (B30.2e)
- `db.go:753, 770` — `doctors` узкий SELECT, колонки NOT NULL
- `auth.go:959` — `must_change_password INTEGER NOT NULL DEFAULT 0` (ALTER из `db.go:292`)

**Тесты:**
`backend/patient_auth_test.go` — 7/7 PASS:
- `TestGetVitals_NullFields` / `_NoRows`
- `TestGetVitalsHistory_NullFields` / `_Empty`
- `TestGetCard_NullFields` / `_NoRows`
- `TestGetPatientAccountByLogin_NullLastLogin`

**Проверка:**
    go test ./backend/ → ok BOT_MAX/backend 7.557s
    gofmt -l backend/ → пусто
    go build -o /dev/null ./backend → ok

**Методологическая заметка:**
При аудите nullable недостаточно грепа `Scan(` — нужно смотреть тип Go-поля:
- `string` / `int` / `float64` + nullable-колонка → 🔴 дефект
- `sql.NullString` / `sql.NullInt*` / `sql.NullFloat64` → ✅ безопасно (независимо от SQL)
- `COALESCE` в SQL → ✅ безопасно даже для примитива

Ложные срабатывания: `patient_auth.go` изначально был помечен 🔴
(`LastLogin` — по грепу без тела), но по факту `sql.NullString`.

Урок сессии: `git diff --stat` пустой → правок нет. Не полагаться на
молчаливый `go build` как признак применённых изменений.

**Откат:**
- `git revert cac3a1b` — фикс (3 SQL-запроса).
- `git revert acf1213` — тесты (отдельный файл, безопасно).

**Точка входа следующей сессии:**

    cd /workspaces/BOT_MAX-main
    git log --oneline -3
    git status -sb
    grep -n "6.65" SNAPSHOTS.md
    make backup
    # далее — новая задача

**Долги:** пусто. B30.3 закрыт. Аудит nullable завершён полностью.

## 6.66 — B30.3: consent-flow (вариант B)

**Статус:** в работе (2026-10-16)
**Предшественник:** §6.65 (B30.3-audit закрыт, долги пусто)
**Основание:** §6.59, строки 2614–2617 — план серии B30

**Цель:** consent-контур patient ↔ doctor. Пациент даёт/отзывает
согласие на чтение vitals, врач запрашивает. 5 роутов, append-only
аудит в consents_audit, enum расширен под deny.

**Решения (вариант B):**
- 1a: повторный request после denied → UPSERT в pending,
  approved_at/revoked_at обнуляются (SET NULL).
- 2a: revoke — обе стороны (пациент всегда, врач — отказ от доступа).
  revoke возможен только из approved (иначе 409).
- 3a: expires_at TEXT nullable — колонка есть, логики TTL нет (YAGNI).

**Обоснование выбора B (а не A):**
По скорости работы A ≡ B: тот же индекс B-tree, те же запросы,
тот же размер строки. Разница только в скорости разработки.
B выбран потому, что A даёт скрытый pending (не отличить отказ от
молчания) и дыру в аудите (deny не фиксируется). Схема B30.1 уже
предполагала actor_type patient|doctor|admin — то есть двусторонность.

**Декомпозиция:**

    #   Шаг              Файл                 Артефакт
    1   Схема-дельта     backend/db.go        status +denied, action +deny, expires_at
    2   Миграция         backend/db.go        ALTER-safe, паттерн B30.2b/c
    3   Функция аудита   backend/consent.go   consentAudit() по образцу auditLog
    4   Модель           backend/consent.go   Consent struct, JSON-теги
    5   Роуты (5)        backend/consent.go   request/approve/deny/revoke/list
    6   Регистратор      consent.go + main.go registerConsentRoutes()
    7   Тесты            backend/consent_test.go  по образцу patient_auth_test.go
    8   UI врача         doctor_cabinet.html  запрос consent (возможно → B30.5)

**Роуты:**

    POST /api/consents/request        врач     → pending    audit=request
    POST /api/consents/{id}/approve   пациент  pending→approved  audit=approve
    POST /api/consents/{id}/deny      пациент  pending→denied    audit=deny
    POST /api/consents/{id}/revoke    обе      approved→revoked  audit=revoke
    GET  /api/consents                обе      список (свои)

**Роль — через существующие хелперы (новых обёрток не плодим):**
    patientIDFromCtx(r)  — patient_auth.go:57
    doctorIDFromCtx(r)   — auth.go:358

**Установленные факты (разведка 2026-10-16):**
- Файла consent.go нет. Consent-роутов/хендлеров нет вообще.
- Схема consents + consents_audit готова (db.go:222–259, B30.1).
- auditLog() переиспользовать нельзя — пишет в audit_log с doctor_id,
  без actor_type. Нужна своя consentAudit() по образцу.
- registerConsentRoutes() встанет после registerPatientAuthRoutes()
  в main.go (стиль: registerXxxRoutes по группам).
- requirePatient/requireDoctor в проекте отсутствуют — это осознанный
  стиль; роль определяется через *IDFromCtx.

**Открытый вопрос (решён):** approved_at/revoked_at обнулять при
повторном request (SET NULL), чтобы не было артефактов старого цикла.

**Точка входа следующей сессии:**

    cd /workspaces/BOT_MAX-main
    git log --oneline -3
    git status -sb
    grep -n "6.66" SNAPSHOTS.md
    make backup
    # далее — подшаг 1: схема-дельта в db.go

**Долги:** пусто.

**Итог (закрыто 2026-10-16):**

| # | Подшаг | Артефакт |
|---|---|---|
| 1 | Схема-дельта | e83eba7 |
| 2 | Миграция | e83eba7 |
| 3 | consentAudit | 1334120 |
| 4 | Модель Consent | 1334120 |
| 5 | 5 роутов | 1334120 |
| 6 | registerConsentRoutes + вызов | 1334120 |
| 7 | Тесты (13/13 PASS) | 80967e1 |
| 8 | UI врача | отложено (B30.5) |

**Коммиты:**
- 7a86943 docs(snapshots): §6.66 — план и решения
- e83eba7 feat(db): consents.expires_at + enum denied/deny
- 1334120 feat(consent): 5 роутов
- 80967e1 test(consent): 13 тестов (httptest)

**Урок сессии (подмена глобальной db в тестах):**
Хендлеры consent.go читают пакетную db (main.go:382), которая
инициализируется только в main(). В тестах её надо подменять:
паттерн doctors_test.go:TestHandleDoctorsList_RoleFilter —
saved := db; db = newTestDB(t); t.Cleanup(func(){ db = saved }).
Оформлено как хелпер setupConsentDB(t) без возврата — работаем
через пакетную db (симметрия с doctors_test.go).

## 6.67 — B30.4: sync (approved consent + scope)

**Дата:** 2026-10-16
**Коммит:** `8e80da9` feat(sync): B30.4 — GET /api/patients/{id}/sync
**Предшественник:** §6.66 (B30.3 consent-flow закрыт)

### Что сделано

`GET /api/patients/{id}/sync` — врач получает `vitals_history` пациента
при наличии `approved` consent со `scope='vitals_read'`. Consent-контур
B30.3 переиспользуется без изменений.

**Файлы:**
- `backend/sync.go` (new, ~145 строк) — `handlePatientSync`,
  `parseSyncLimit`, `findApprovedConsent`, `checkConsentExpired`.
- `backend/sync_test.go` (new, ~200 строк) — 10 тестов, httptest, прямой
  вызов handler'а (стиль 1:1 с `consent_test.go`).
- `backend/patients.go` (+5 строк) — `case "sync":` в `handlePatientByID`
  рядом с `vitals`/`cards`.

### Контракт

    GET /api/patients/{id}/sync?limit=N   (doctor-only)

    200 {"patientId":...,"consentId":...,"limit":N,"records":[...]}
    403 {"error":"doctor only"}       — нет doctor-контекста
    403 {"error":"consent_required"}  — нет approved consent / чужой scope
    403 {"error":"consent_expired"}   — expires_at < now()
    400 {"error":"invalid limit"}     — limit не число / <1 / >500
    405 {"error":"method not allowed"}

### Решения сессии

1. **Scope — строго `vitals_read`.** Не префикс. UNIQUE на
   `(patient_id, doctor_id, scope)` в B30.3 фиксирует модель «один scope =
   одна связка»; префикс — преждевременно.
2. **Нет consent → 403 `consent_required`** (не 404). Не палим
   существование пациента. Существование `patients` специально не
   проверяем — экономия SELECT, FK не нужен.
3. **`expires_at` — TEXT, layout `"2006-01-02 15:04:05"`** (`tsLayout`,
   как в `consent.go:73,325,353`). НЕ RFC3339. NULL → бессрочный.
   Не парсится → 500 `internal` (коррапт, не пропускаем).
4. **TTL не выставляется.** `upsertConsentRequest` (B30.3) не трогает
   `expires_at` — все текущие consents имеют NULL. Проверка в sync —
   заготовка. Тест `TestSync_ConsentExpired` — через ручной UPDATE.
5. **Лимиты:** дефолт `50` (совместим с `getVitalsHistory`), max `500`.
   `?limit=abc / 0 / -5 / 9999` → 400 `invalid limit`.
6. **Аудит:**
   - 403 (нет consent или expired) → `consents_audit`, action `read_denied`,
     actor `doctor` — это событие consent-контура.
   - 200 → общий `audit`, action `vitals.read`, entity `patient`.

### Отложено

- `?offset=`, `?from=&to=` — пагинация и диапазон. `getVitalsHistory`
  умеет только `limit`. Добавим, когда появится реальный потребитель
  (UI врача или B30.5).
- Префиксный scope (`vitals_read%`) — если появится `vitals_read_write`.

### Тесты (10/10 PASS)

| Тест | Что проверяет |
|---|---|
| `TestSync_HappyPath` | doctor + approved + 3 vitals → 200, records=3, DESC |
| `TestSync_NoConsent` | нет consent → 403 `consent_required` |
| `TestSync_ConsentPending` | status='pending' → 403 `consent_required` |
| `TestSync_ConsentRevoked` | approved→revoked → 403 `consent_required` |
| `TestSync_ConsentExpired` | expires_at в прошлом → 403 `consent_expired` |
| `TestSync_WrongScope` | scope='other_scope' → 403 `consent_required` |
| `TestSync_NonDoctor` | нет doctor-ctx → 403 `doctor only` |
| `TestSync_BadLimit` | limit=abc/0/-5/9999 → 400 `invalid limit` |
| `TestSync_LimitApplied` | ?limit=2 при 5 записях → 200, records=2 |
| `TestSync_MethodNotAllowed` | POST → 405 |

### Проверка

- `go test ./backend/ -run TestSync -v` → 10/10 PASS
- `go test ./backend/` → ok, 7.434s
- `go build -o /tmp/botmax ./backend/` → OK
- `go vet ./...` → OK
- `gofmt -l backend/` → пусто

**Урок сессии:** timestamp-формат проекта — `"2006-01-02 15:04:05"`,
не RFC3339. Проверять через `grep -n "Format(" backend/*.go` до
написания парсера.

## 6.68 — B30.5: UI пациента (self-vitals) + sync-индикатор у врача

**Дата:** 2026-10-16
**Коммиты:** `6560603` feat(patient-vitals), `a2621ec` feat(patient-cabinet),
`c14defe` feat(doctor-cabinet)
**Предшественник:** §6.67 (B30.4 sync закрыт)

### Что сделано

Пациент ведёт свои vitals через backend (self-endpoints). Врач видит
индикатор sync на карте пациента.

**Backend — self-vitals (`6560603`):**
- `backend/patient_vitals.go` (new, ~140 строк) — `handlePatientVitals`
  (GET/PUT), `handlePatientVitalsHistory` (GET), `resolvePatientIDForSelf`,
  `registerPatientVitalsRoutes`.
- `backend/patient_vitals_test.go` (new, ~230 строк) — 9 тестов, httptest,
  прямой вызов handler'а (стиль 1:1 с `consent_test.go`).
- `backend/main.go` (+1 строка) — `registerPatientVitalsRoutes()` рядом
  с `registerPatientAuthRoutes()`.

**UI пациента (`a2621ec`):**
- `patient_cabinet_full.html` (+69 строк) — `syncVitalsToServer()`,
  `bootstrapVitalsFromServer()`, вызовы в `updateBP`, `updateSpO2`,
  `addTemperatureFromSelect`, `DOMContentLoaded`.

**UI врача (`c14defe`):**
- `doctor_cabinet.html` (+38 строк) — `<span id="syncBadge">`,
  `updateSyncBadge(patientId)`, вызов в `selectPatient`, сброс в
  `deletePatient`.

### Контракт

    GET  /api/patient/vitals                      (patient-only)
    PUT  /api/patient/vitals                      (patient-only)
    GET  /api/patient/vitals/history?limit=N      (patient-only)

    GET 200 {"bp":"...","temp":...,"spo2":...,"updated":"..."}
    PUT 200 {"ok":true}
    GET history 200 [{id,patientId,bp,temp,spo2,recorded}, ...] DESC
    404 {"error":"patient account not found"}  — нет аккаунта / нет ctx
    400 "invalid json: ..."                    — PUT с битым телом
    405 "method not allowed"

### Решения сессии

1. **Резолв patientID — двухшаговый.** `patientIDFromCtx(r)` возвращает
   `patient_accounts.id` (subjectID). `getPatientAccountByID` даёт
   `acc.PatientID` (`patients.id`). Vitals-таблицы работают с `patients.id`.
   Три уровня контекста: `Claims.DoctorID` (историческое имя, = subjectID),
   `patient_accounts.id`, `patients.id` — не путать.
2. **Формат времени — RFC3339, не `tsLayout`.** `db.go:upsertVitals` /
   `appendVitalsHistory` пишут `time.RFC3339`. Урок B30.4 про
   `"2006-01-02 15:04:05"` относится к `consents.expires_at`, не к vitals.
   Ошибки парсинга тут не бывает — строку прокидываем как есть.
3. **GET без записи → 200 + пустой объект,** не 404. Фронт сам решает,
   показывать ли fallback. `getVitals` использует `COALESCE(...)`, поэтому
   `sql.ErrNoRows` — только при полном отсутствии строки.
4. **`limit` в history: default 50, cap 500.** Симметрия с B30.4.
   (`getVitalsHistory` тоже default 50.)
5. **Middleware — `patientAuthMiddleware` (hard-mode),** как в B30.2.
   Whitelist не нужен: `registerPatientVitalsRoutes` оборачивает хендлеры
   сразу, `patient-auth/*` остаётся отдельно.
6. **UI пациента — fallback на localStorage.** `patient_cabinet_full.html`
   пока без экрана логина, поэтому backend-синк «мягкий»: 401/сеть → тихо
   остаёмся на localStorage. Экран логина — отдельная задача (B30.5b).
7. **Temp через DOM, не через `patientData.tempHistory`.** В модели
   пациента нет числового temp. `addTemperatureFromSelect` дописан
   `syncVitalsToServer()` — значение `tempSelect.value` уходит в backend,
   текстовая запись в `patientData.symptoms` остаётся (для графика).
8. **Индикатор sync — через `BotMaxAuth.authFetch`,** не голый `fetch`.
   Bearer + refresh 401, как в остальных вызовах `doctor_cabinet.html`.
9. **`limit=500` в бейдже + суффикс `+`.** `sync.go:parseSyncLimit`
   возвращает 400 при `limit > 500` (не cap). Показываем полный набор до
   500 и `500+` при упоре.

### Бейдж sync — состояния

| Источник | Бейдж |
|---|---|
| `200`, `records.length < 500` | зелёный «sync: N записей» |
| `200`, `records.length === 500`, `limit === 500` | зелёный «sync: 500+ записей» |
| `403 consent_required` | красный «нет согласия» |
| `403 consent_expired` | жёлтый «согласие истекло» |
| `401 / 404 / 5xx / сеть` | пусто (тихо) |

### Отложено

- **Коммит 2b:** `showBPHistory` / `showSpO2History` / `showTemperatureChart`
  всё ещё на localStorage. Перевод на `GET /api/patient/vitals/history`.
- **B30.5b:** экран логина пациента (если `patient_cabinet_full.html`
  должен работать только авторизованным).
- **Кнопка «запросить consent»** в `doctor_cabinet.html` при бейдже
  «нет согласия» — backlog.
- **Smoke E2E в браузере** — отложен (backend-тесты зелёные, 9/9).
- **`offset` / `from&to`** для self-history — наследуется из отложенного
  B30.4.

### Тесты (9/9 PASS, backend)

| Тест | Что проверяет |
|---|---|
| `TestPatientVitals_GetEmpty` | нет записи → 200 пустой объект |
| `TestPatientVitals_PutGet` | PUT → GET: значения сохранились |
| `TestPatientVitals_History` | PUT×2 → history 2 записи, DESC |
| `TestPatientVitals_HistoryLimit` | ?limit=1 → одна запись |
| `TestPatientVitals_AccountNotFound` | чужой accID → 404 |
| `TestPatientVitals_NoCtx` | нет accID в ctx → 404 |
| `TestPatientVitals_PutBadJSON` | битый JSON → 400 |
| `TestPatientVitals_MethodNotAllowed` | DELETE → 405 |
| `TestPatientVitalsMiddleware_NoToken` | нет cookie → 401 |

### Проверка

- `go vet ./backend/` — чисто.
- `go test ./backend/` — PASS, 7.4s.
- `gofmt -l backend/` — пусто.

### Точка входа следующей сессии

```bash
cd /workspaces/BOT_MAX-main
git log --oneline -5
git status -sb
grep -n "6.68\|B30.5\|B30.6" SNAPSHOTS.md | head
make backup
# далее — smoke B30.5 в браузере, затем B30.5b (экран логина пациента)
# или B30.6 (см. §6.58 — план B30)
```

### Уроки сессии

- **Три уровня контекста пациента.** `patient_accounts.id` (subjectID,
  кладётся в ctx как `patientIDFromCtx`), `patients.id` (`acc.PatientID`,
  используется в vitals-таблицах), `Claims.DoctorID` (историческое имя,
  для пациента = `patientAccountID`). Путаница здесь ловится тестом
  `TestPatientVitals_AccountNotFound`.
- **`sync.go:parseSyncLimit` не делает cap.** `limit > 500` → 400, не «тихо
  режем до 500». Поэтому в бейдже `limit=500` — единственный способ
  получить максимум. Если когда-нибудь поменяем на cap — упростить бейдж.
- **HTML-файлы — пробелы, не табы.** Python-скрипт с таб-символом в якорях
  даёт `count == 0`. Для `.html` использовать 4 пробела. Урок из сессий
  подтвердился.
- **`grep -c` считает строки, не вхождения.** `grep -c "updateSyncBadge"`
  дало 3, а не 4 — потому что `function updateSyncBadge(patientId)` — одна
  строка. При проверке якорей ориентироваться на число строк.
- **`cat <<'EOF'` с блоком ` ``` ` внутри — не использовать.**
  Закрывающий `EOF` легко «съедается» в интерактиве (залипание на `>`).
  Markdown-блоки с код-фенсами дописывать в файл **построчно через python
  `open(p, "a").write(line + "\n")`** или через `printf '%s\n' …`,
  короткими кусками по 3–8 строк. В §6.68 это стоило двух залипаний.

## 6.69 — B30.5 smoke: находки и пересмотр порядка

Backend-smoke через curl (smoke_b30_5.sh) прошёл end-to-end:
login -> cookie -> PUT/GET/history — все 6 шагов HTTP 200.
Контракт /api/patient/vitals рабочий. По ходу smoke выявлено:

### H1. UI пациента не авторизуется -> sync-индикатор мёртв
`patient_cabinet_full.html` (a2621ec) ходит на /api/patient/vitals
без cookie. `patientAuthMiddleware` (patient_auth.go:441) требует
cookie `botmax_pat_access` — иначе 401. UI ловит 401 и уходит в
localStorage-fallback. Следствие: sync-индикатор врача (c14defe)
в проде **всегда** в состоянии «нет данных».

Причина: B30.5b (экран логина пациента) на момент B30.5 не сделан.

### H2. patient_accounts пуста — логиниться некому
`SELECT COUNT(*) FROM patient_accounts` -> 0. Backend login/refresh/
logout готовы (B30.2, /api/patient-auth/*), но ни одного аккаунта
не заведено. B30.5b = UI-логин **плюс** заведение аккаунта.

### H3. Cookie Secure — dev-safe (не блокер)
`setPatientCookies` (cookies.go:96) берёт Secure из
`currentEnv.CookieSecure`; в dev (BOTMAX_ENV=dev) — false.
На http://localhost:8082 cookie работает корректно.

### H4. Контракт vitals: bp_sys/bp_dia не принимаются (SMOKE-находка)
PUT с `{"bp_sys":120,"bp_dia":80,...}` даёт `{"ok":true}`, но GET
возвращает `bp:""`. API ожидает **строку** `bp` (вида `"120/80"`),
а не два числовых поля. `bp_sys`/`bp_dia` молча игнорируются.
Нужно либо (а) расширить API, либо (б) привести UI к формату `bp`.
Не блокер для B30.5b, но **фиксируем до коммита 2b** — история bp
без этого будет пустая.

### H5. patientLoginResp.patient.id = account id, а не patient id
В ответе login: `{"patient":{"id":"PACC-...","name":"...","login":"..."}}`.
`history.patientId` при этом = `PAT-001`. Поле `patient.id` в ответе
login семантически = account id. Если UI использует `patient.id`
как id пациента для запросов — будет рассинхрон. Проверить при B30.5b.

### H6. dev-hash endpoint (infra)
Добавлен `POST /api/patient-auth/dev-hash` (dev-only, prod -> 404):
`{"password":"..."} -> {"hash":"$2a$10$..."}`. Нужен для seed
patient_accounts в smoke-скриптах и локальной разработки.
Реализация — patient_auth.go, build-безопасно (проверка через
currentEnv.IsProd).

## Правило работы

**ВСЕ ИЗМЕНЕНИЯ ФАЙЛОВ — ТОЛЬКО ЧЕРЕЗ BASH.** Запрещено:
- открывать редактор (`code`, `nano`, `vim`) для правок;
- вставлять markdown/HTML/Go-текст прямо в промпт;
- heredoc с бэктик-фенсом и исполняемыми бэктиками внутри (урок §6.68).

Разрешено:
- `python3 - <<'PY'` с внешним литералом через тройные кавычки
  (одинарными или двойными), но с проверкой на конфликт;
- `cat > f <<'EOF'` — только если внутри нет бэктик-фенсов и бэктиков;
- `cat a >> b`, `printf`, `sed -i`, `tee`, `cp`, `mv`, `git`;
- `base64 -d` — крайний fallback.

Каждое изменение — проверяемо: `grep -n`, `wc -c`, `head`, `tail`,
`bash -n`, `gofmt -l`, `go build`.

### Уроки quoting (v2, 2026-10-20)

- **history-expansion**: символ bang в python -c съедается bash до python; assert не ловит — проверять вывод глазами. Лечение: set +H, либо \x21 в литерале, либо не писать bang в тексте.
- **вложенные кавычки**: sed c\ и python -c теряли escape (\" -> "). Надёжно: printf %s\n ... > /tmp/x.py, тело в одинарных кавычках, запуск python3 /tmp/x.py.
- **go build ./... из корня**: коллизия имени бинарника backend с каталогом backend/; использовать go build -o /tmp/<name> ./...

## Решение: пересмотр порядка

- **B30.5b** (UI логина пациента + seed/аккаунт) — вперёд 2b.
- **2b** (история vitals) — после B30.5b; и **сначала** решить H4
  (формат bp), иначе showBPHistory получит пустоту.
- **H5** (patient.id = account id) — проверить и исправить вместе
  с B30.5b, если UI начнёт использовать `patient.id`.

## Артефакты

- `smoke_b30_5.sh` — backend smoke (login -> cookie -> PUT/GET/history),
  с rollback через trap EXIT, использует dev-hash.
- `backend/patient_auth.go` — dev-only `POST /api/patient-auth/dev-hash`
  (IsProd -> 404).
- `patient_login.html` — TODO (B30.5b).
- `patient_cabinet_full.html` — TODO: `apiFetch()` + редирект на 401.

## B30.5b — patient_cabinet_full.html (2026-10-20)
- **H1 закрыт:** vitals PUT/GET переведены на `apiFetch` (credentials:'include';
  access-cookie botmax_pat_access, Path=/api). При 401 — один retry через
  `POST /api/patient-auth/refresh` (refresh-cookie botmax_pat_refresh,
  Path=/api/patient-auth, SameSite=Strict, HttpOnly), при неудаче — редирект
  на `/patient_login.html`. PUT — строгий (401→refresh→login); GET —
  `allow401:true` (тихий fallback на localStorage, как и было задумано).
  Из PUT убран явный `credentials:'same-origin'` (перекрывал include).
- **H4 закрыт:** backend принимает `bp string` (`vitalsBody.BP string json:"bp"`,
  backend/patients.go:251). UI собирает `bp` как `"sys/dia"`; при пустых
  селектах отправляется `""` (а не `"/"`). Backend не менялся.
- **H5 закрыт как неприменимый:** UI кабинета использует `patientData.patientId`
  (PAT-xxx); account id в patient_cabinet_full.html не участвует.
- **Сессия пациента:** cookie-based (botmax_pat_access / botmax_pat_refresh,
  HttpOnly, SameSite=Strict, Secure=prod). Bearer не поддерживается
  (patient_auth.go:456). Токены из ответа login в localStorage НЕ кладём.
- **patient_login.html:** новая страница входа; POST /api/patient-auth/login
  ({login,password}); авто-переход в кабинет при валидной cookie.
- **TODO:** `sessionToken` в localStorage (строка 645) оставлен как fallback
  на переходный период; удалить после подтверждения стабильной cookie-сессии.
- **Регламент бэкапа:** перед `make backup` останавливать сервер (`make stop`),
  чтобы bot_max.db не попадал в WAL-состояние (-wal/-shm не в бэкапе).

## B30.5b — smoke (2026-10-20)
- backend smoke `smoke_b30_5.sh`: 6/6 HTTP 200; PUT `bp` строкой,
  GET/history round-trip `"bp":"120/80"` — OK (`OK: bp round-trip 120/80`).
- Фикс smoke: было `bp_sys`/`bp_dia` (мёртвые поля, `vitalsBody` их не читает,
  `bp` оставался пустым) → заменено на `"bp":"120/80"` + assert round-trip.
  H4 теперь проверяется автоматически, а не только чтением кода.
- Связка account→patient: login возвращает `PACC-xxx`, vitals/history хранятся
  по `PAT-xxx` (`resolvePatientIDForSelf`). В UI не всплывает (UI на
  `patientData.patientId`). Зафиксировано, чтобы не читалось как баг.
- Токены `accessToken`/`refreshToken` в теле login — для API-клиентов
  (smoke/curl); браузер ходит только cookie (botmax_pat_access/refresh).

## B30.5b — диагностика и smoke-нюансы (2026-10-20)
- **smoke_b30_5.sh самоочищающийся:** trap EXIT делает
  `DELETE FROM patient_accounts WHERE id=$ACC_ID` + `rm -f $JAR`.
  Поэтому после прогона `SELECT ... WHERE login='smoke_pat'` = 0 — это норма,
  а не «INSERT не сработал». Для ручных проверок нужен свой аккаунт
  (refresh_test), который trap не трогает.
- **WAL и CLI:** при живом сервере `sqlite3 bot_max.db "SELECT ..."` может
  не видеть свежие записи из `-wal` (`journal_mode=wal`). Для проверки:
  `PRAGMA wal_checkpoint(PASSIVE);` перед SELECT (безопасно при живом
  сервере), либо `make stop` (checkpoint при закрытии). Иначе диагностика
  врёт (`COUNT=0` при реально существующих строках).
- **refresh-retry подтверждён контрактно:** login 200 → PUT 401 (удалён
  access-cookie) → POST /api/patient-auth/refresh 200 → PUT 200.
  Это ровно поведение apiFetch при истёкшем access.

## B30.5b — ИТОГ (2026-10-20)
B30.5b закрыт: H1 (apiFetch+refresh-retry+401->login), H4 (bp string,
round-trip в smoke), H5 (неприменим к patient_cabinet_full.html).
patient_login.html добавлен. Smoke: backend 6/6 + bp round-trip,
refresh-retry контрактно OK (login 200 → PUT 401 → refresh 200 → PUT 200).
Бэкап снят по новому регламенту (make stop перед make backup) — без
WAL-предупреждения, bot_max.db консистентен. Первый бэкап без ⚠️:
backups/bot_max_20261020_065516 (260K), ротация 10/10.

## B30.5b — followup: refresh 400→401 (2026-10-20, a446b88)
Бэклог-пункт 1 закрыт. `handlePatientRefresh` (backend/patient_auth.go:339):
отсутствие refresh-токена теперь `401 Unauthorized` вместо `400 Bad Request`.
Семантика: "нет аутентификации" ≠ "кривой запрос". Остальные ветки функции
(ParseToken, wrong role, revoked) уже отдавали 401 — цепочка консистентна.
apiFetch-поведение не меняется (`!rr.ok → redirect`), но 401 чище.

Проверки:
- `curl -i -X POST /api/patient-auth/refresh` без cookie → **401** (было 400);
- `-d '{"refreshToken":"garbage"}'` → **401** (ветка ParseToken);
- позитив (валидный refresh → 200) — в smoke, регресс `smoke_b30_5.sh` → **6/6**;
- `gofmt -l` пусто, `go build -o /tmp/botmax-build-check ./...` OK, `go vet ./...` OK.

Не тронуто (корректные 400): 223 (invalid json login), 228 (пустые login/password),
314 (dev-hash empty password). Бэкап: patient_auth.go.bak.b30_5b_refresh.
Коммит: a446b88 (1 file, 1 insertion, 1 deletion).

## B30.5b — followup 2: smoke KEEP_ACCOUNT=1 (2026-10-20, 1f55ba4)
Бэклог-пункт 2 закрыт. `smoke_b30_5.sh`: `cleanup()` больше не удаляет
временный `patient_accounts` при `KEEP_ACCOUNT=1` — удобно для ручной
отладки (аккаунт остаётся, id печатается в stderr). `rm -f $JAR` — всегда.

Проверки обоих режимов:
- по умолчанию: smoke **6/6**, аккаунт удалён (`SELECT count … PACC-SMOKE%` → **0**);
- `KEEP_ACCOUNT=1`: smoke **6/6**, аккаунт `PACC-SMOKE-6477` остался,
  ручной `DELETE` → **0**. `bash -n` OK.

Урок quoting: правка строки с вложенными кавычками через `sed c\` / python
`-c` дважды съедала экранирование (`\"` → `"`). Прагматичное решение —
не дублировать SQL-пример в `echo`, он уже есть в комментарии над `cleanup()`.
Коммит: 1f55ba4 (1 file, 8 insertions, 1 deletion).

## B30.6 — consent UI + dual-auth + patient login (2026-10-21)
Этап 1 B30 (см. §6.58): закрыт подпатчами 6a-s. Точка входа: 4b3b2af.

### Подпатчи
- **6a-c:** `/api/consents*` принимает и patient-, и doctor-токен (dual-auth в
  `authMiddleware`), ставит ОБА контекста; в `consent.go` резолв account→patients.id
  через `resolvePatientIDForSelf` (было: сравнение account id с patients.id → 401/403).
- **6d/e/f:** `index.html` — карточка пациента → `patient_login.html` (было: сразу кабинет),
  врач → `login.html`; `patient_login.html` приведён к стилю `login.html` (тёмная карточка,
  🧠 BOT_MAX, поля Логин/Пароль, подсказка `patient@local`); seed `PACC-DEMO`/`PAT-DEMO`
  (`patient@local`/`patient@local`) в `db.go`+`seed.go`.
- **6g-m:** кабинет пациента: убран doctor-guard (`auth.js` + inline `requireAuth`),
  добавлена consent-карточка (list/approve/deny/revoke через `apiFetch`), `logout()` →
  `POST /api/patient-auth/logout` + confirm, тема/контраст, баланс `<script>` (chart.js был
  незакрыт → весь JS «съеден»), глаз пароля в обеих формах.
- **6n-s:** единый UI с врачом: `--bg-header`/`--bg-card-border`, `.header`/`.card` с blur;
  переключатель темы (light/dark) через `theme.js` + `[data-theme="dark"]` с полным набором
  переменных; `.constant-med-item` → `var(--bg-subtle)` (было `#e6fffa`).

### Проверки
- consent end-to-end (curl): pending → approve → approved → revoke → revoked, всё 200;
- login `patient@local` при doctor-cookie → `Set-Cookie: botmax_access/refresh Max-Age=0` +
  patient-cookie; кабинет открывается без прыжка на врача;
- smoke 6/6 (bp round-trip), `go build`/`go vet`/`gofmt -l` зелёные;
- UI: белая/тёмная тема, consent-карточка рендерится, глаз пароля, «Выйти» разлогинивает.

### Коммиты
838aaf8 (6a-c) → 13aed81 (6d/e/f) → 4b3b2af (6g-s).

## B30.7 — серверный профиль пациента (2026-10-22)

Закрывает разрыв: поля `birth_date/gender/height/weight` в схеме и в структуре
`Patient` были, врачебные CRUD (`getPatientsByDoctor/createPatient/updatePatient`)
их читали/писали, но у пациента не было своего эндпоинта — кабинет правил
профиль только в `localStorage`. Теперь пациент читает и правит свой профиль
через `GET/PUT /api/patient/profile`, а сервер — источник истины.

### Что сделано

- `backend/patient_profile.go`: `PatientProfile`, `profileBody` (указатели —
  «не прислали» ≠ «прислали пусто»), `getPatientProfile/updatePatientProfile`,
  `calcAgeFromBirthDate` (+`formatAgeDisplay`, `profileItoa`),
  `handlePatientProfile`, `registerPatientProfileRoutes`.
- Роут за `patientAuthMiddleware`; резолв `patientAccountID → patients.id`
  через `resolvePatientIDForSelf` (как vitals/consents).
- Валидация: `gender ∈ {male,female,other}`, `birthDate` — `YYYY-MM-DD` и не
  в будущем, `height 0..250 см`, `weight 0..500 кг`, `name` non-empty (если пришло).
- Возраст пересчитывается на сервере из `birth_date` при PUT
  (`age/age_months/age_display`) — иначе врач видел бы расхождение birthDate↔age.
- `UPDATE` трогает только «свои» поля (`name/birth_date/gender/height/weight/
  age/age_months/age_display`). Врачебные (`doctor_id/status/diagnosis/ai_*/
  last_sync/clinic_id/phone/email`) через этот эндпоинт не меняются.
- audit: `patient.profile.update` при PUT.

### Подпатчи

- **7a (backend):** `patient_profile.go` + `patient_profile_test.go`
  (GET empty, PUT happy+persist, partial PUT, 8 invalid-кейсов → 400,
  no-account → 404, таблица `calcAgeFromBirthDate`),
  регистрация в `main.go` рядом с `registerPatientVitalsRoutes`.
- **7b (UI):** `patient_cabinet_full.html` — не новая карточка, а подключение
  существующих редакторов к серверу: `loadProfile()` (GET → patientData + DOM,
  рост см ↔ м+см, вес кг ↔ кг+г на границе, `ageDisplay` с fallback на
  локальный `getAgeDisplay`) и `saveProfile(patch)` (частичный PUT через
  `apiFetch`, `Content-Type: application/json`, текст 400 показывается через
  `showNotification`). Вызовы из `editPatientName` / `saveBirthDate` /
  `updateGender` / `updateHeightWeight`; `loadProfile()` в `updateAll()`.
  Без новых CSS — переиспользованы `.card`/`.info-*` и `showNotification`.
- **7c (smoke):** `smoke_b30_7.sh` — patient login (dev-hash + seed
  `patient_accounts`), GET/PUT/GET профиля, 5 invalid PUT → 400, bonus:
  doctor login (`doctor@local`) → `GET /api/patients/PAT-001` видит обновлённые
  `birthDate/gender`. Стиль `smoke_b30_5.sh`, `bash -n` чист.

### Проверки

- `go build`/`go vet`/`gofmt -l` зелёные, `go test ./backend/` ok;
- smoke `smoke_b30_7.sh` против живого сервера: шаги 1–7 + bonus зелёные, rc=0:
  - GET profile → 200 (`name`);
  - PUT `{birthDate:"1990-01-01",gender:"male",height:180,weight:75}` → 200,
    `age:36, ageMonths:441, ageDisplay:"36 г 9 мес"` (пересчитано);
  - повторный GET — значения сохранились;
  - invalid PUT (`gender:"xxx"`, `birthDate:"2999-01-01"`, `height:999`,
    `weight:999`, `name:"   "`) → 400;
  - doctor `GET /api/patients/PAT-001` — видит `birthDate:"1990-01-01"`,
    `gender:"male"`, `age:36` (консистентно для врача).

### Уроки сессии

- **Поля были, эндпоинта не было.** B30.7 — не про схему (всё уже было в
  `patients` и `Patient`), а про серверный CRUD пациента поверх
  `patientAccountID`. Перед «добавить модель» стоит грепнуть колонки: они уже
  могут быть и читаться врачебным путём.
- **`updatePatient` перезаписывает `age*` «как пришло».** Поэтому `PUT profile`
  обязан пересчитывать возраст из `birth_date` на сервере — иначе рождается
  расхождение, которое первым заметит врач в карточке.
- **Единицы на границе.** В UI `patientData.weight` — граммы, `patientData.height`
  — см; на сервере weight — кг, height — см. Конвертация ровно в `saveProfile`
  (`/1000`) и в `loadProfile` (`*1000`); смешивать нельзя.
- **`itoa` уже занят.** `encounters_plan_test.go` объявляет `itoa` — второй в
  пакете не собрать (`vet: itoa redeclared`). Локальный хелпер назван
  `profileItoa`. Впредь имена хелперов — с префиксом файла.
- **`go build ./...` из корня падает** на `build output "backend" already exists
  and is a directory`. Для проверки — `go build -o /dev/null ./...`.
- **Формат `ageDisplay` у старых записей другой** («35 лет» vs наш «36 г 9 мес»).
  Унификация формата — отдельная задача, если понадобится; PUT пишет наш формат.
- **soft-mode логирует «auth: soft-mode, нет токена»** на patient-роутах, хотя
  patient-cookie работает (200, корректный `patientIDFromCtx`). В hard-mode
  (`BOTMAX_AUTH_ENFORCE=1`) проверить, что patient-middleware не конфликтует
  с doctor-middleware.

### Коммиты

10a7fcb (7a) → 7b70ba1 (7b) → 9f45e47 (7c) → docs.

## B30.8a — тесты patientAuthMiddleware на /api/patient/profile (2026-10-22)

Закрывает пункт бэклога, оставленный в уроках B30.7 («в hard-mode проверить,
что patient-middleware не конфликтует с doctor-middleware»). Реальная дыра
оказалась шире: существующие `patient_profile_test.go` вызывают
`handlePatientProfile` напрямую с `withPatientID` в контексте — то есть
`patientAuthMiddleware` в тестах **байпасится**, и проверки роли/токена не
покрыты вообще.

### Что сделано

- Новый `backend/patient_profile_auth_test.go`: 4 теста прогоняют запрос через
  `patientAuthMiddleware(http.HandlerFunc(handlePatientProfile))`, cookie
  `botmax_pat_access`:
  - нет cookie → 401 `authorization required`;
  - doctor access-токен (`IssueAccessToken("DOC-MW2", RoleDoctor)`) → 401
    `invalid token` (ключевое: doctor-токен не принимается patient-эндпоинтом);
  - валидный patient access-токен (`IssueAccessToken(accID, RolePatient)`) →
    200, профиль своего пациента;
  - patient-токен несуществующего аккаунта → 404 (нет утечки чужого профиля).
- Helper `withProfileAuthEnv(t)` — по образцу `withMetricsEnv`: ставит
  `currentEnv = &Env{Name:"dev", JWTSecret:...}` с `t.Cleanup`, иначе
  `IssueAccessToken` паникует `currentEnv == nil`.

### Проверки

- `go build -o /dev/null ./...`, `go vet ./...`, `gofmt -l` — зелёные;
- `go test ./backend/ -run 'TestPatientProfile' -v` — 4 новых + старые
  `TestPatientProfile_*` — PASS;
- полный `go test ./backend/` — `ok 7.818s`.

### Уроки сессии

- **Middleware, не покрытый тестом, — не «работает», а «пока не падал».**
  `patientAuthMiddleware` уже был hard-mode и корректен по коду, но ни один
  тест его не прогонял: `patient_profile_test.go` шёл мимо. Регрессия в CI
  дешевле ручного прогона с `BOTMAX_AUTH_ENFORCE=1` и остаётся навсегда.
- **`BOTMAX_AUTH_ENFORCE` — только про doctor-middleware.** `patient_auth.go`
  не читает env и всегда hard-mode (см. комментарий в шапке файла). Пункт
  бэклога «patient-middleware vs doctor-middleware в hard-mode» сводится ровно
  к «doctor-токен не должен приниматься patient-роутом» — это и проверяем.
- **`IssueAccessToken` в тестах требует `currentEnv`.** Без него —
  `panic: jwtSecret: currentEnv == nil`. Паттерн `withXxxEnv(t)` с `t.Cleanup`
  уже есть (`withMetricsEnv`, `retention_test.go`) — переиспользовать, не
  изобретать.
- **Ассерты в python-патче считают и объявления, и вызовы.** `src.count("withProfileAuthEnv(t)")`
  не равен числу вызовов, потому что объявление — `withProfileAuthEnv(t *testing.T)`.
  Проверять объявление и вызовы отдельно, иначе ложный AssertionError и
  патч не применяется (что и случилось с первого раза — файл не записан,
  это хорошо: ассерт сработал до `write`).

### Коммиты

979bdf3 (8a) → docs.

## B30.8b — smoke vitals на временном пациенте (2026-10-22)

Закрывает пункт бэклога «smoke: vitals во временного пациента» (сейчас
писал в `PAT-001`, history копилась). `smoke_b30_8.sh` создаёт собственного
пациента `PAT-SMOKE8-$$` и аккаунт на него, пишет vitals, проверяет GET и
history, затем удаляет всё через `trap cleanup EXIT`.

### Что сделано

- `smoke_b30_8.sh` (по образцу `smoke_b30_7.sh`), 6 шагов:
  1. `POST /api/patient-auth/dev-hash` → bcrypt hash;
  2. seed: `INSERT INTO patients (id,name,created)` + `patient_accounts`
     на временного `PAT-SMOKE8-$$`;
  3. `POST /api/patient-auth/login` → cookie-jar;
  4. `PUT /api/patient/vitals` `{bp:"120/80",temp:36.6,spo2:98}` → 200;
  5. `GET /api/patient/vitals` → `bp:"120/80"`, `spo2:98`;
  6. `GET /api/patient/vitals/history?limit=5` → запись с
     `patientId:"PAT-SMOKE8-$$"` (не `PAT-001`).
- `cleanup`: удаляет `patient_accounts`, `patients`, `vitals`,
  `vitals_history` для временного `PATIENT_ID`; `KEEP_ACCOUNT=1` оставляет.

### Проверки

- `bash -n smoke_b30_8.sh` — чисто;
- прогон против живого backend (:8082) — все 6 шагов зелёные, rc=0:
  `OK: B30.8b smoke complete (patient=PAT-SMOKE8-35255)`;
- после прогона `SELECT count(*)` по `patients/patient_accounts/vitals/
  vitals_history` для `%SMOKE8%` — везде 0: cleanup отработал, `PAT-001` не
  затронут.

### Уроки сессии

- **Smoke без cleanup — это не smoke, а seed.** Старый подход «писать в
  PAT-001» оставлял растущую `vitals_history` на общем пациенте: следующий
  прогон видел чужие записи, а данные копились. Временный `PATIENT_ID` +
  `trap cleanup EXIT` — дешёвая гигиена, которую стоит повторять в
  последующих smoke (consents и т.п.).
- **Проверять надо `patientId` в history, а не только наличие `bp`.**
  Иначе smoke пройдёт и при записи «не туда» — ровно тот баг, который
  закрываем. В шаге 6 ассерт на `"patientId":"PAT-SMOKE8-$$"`.
- **GET `/api/patient-auth/login` без метода — 405**, не 404: probe «жив ли
  сервер» через curl на `/login` даёт 405, это норма, не ошибка.

### Коммиты

4e15b19 (8b) → docs.

## B30.8c — нормализация ageDisplay при чтении пациента (2026-10-22)

Закрывает пункт бэклога «унификация `ageDisplay` у старых записей»: в БД
`patients.age_display` у записей, созданных до B30.7, лежит «35 лет», а
`patient_profile.go` пишет «36 г 9 мес». Врач в карточке видел разнобой.

### Что сделано

- `backend/db.go`: helper `normalizeAgeDisplay(p *Patient)` — если
  `p.BirthDate != ""`, пересчитывает `Age/AgeMonths/AgeDisplay` через
  `calcAgeFromBirthDate(birth_date, time.Now().UTC())`. Ошибки парсинга
  `birth_date` игнорируются (остаётся то, что в БД).
- Вызов в обеих точках чтения: `getPatientsByDoctor` (перед `append`) и
  `getPatientByID` (перед `return`) — покрывает всех читателей, включая
  `encounters.go`/`consent.go`.
- Миграция БД **не делалась** намеренно: нормализация на чтении безопаснее,
  обратима и не трогает старые записи; `PUT /api/patient/profile` и так
  пишет новый формат в БД.

### Проверки

- `backend/db_patient_age_test.go`: 3 теста —
  - `getPatientByID` со старым `age_display="35 лет"` + `birth_date="1990-01-01"`
    → `AgeDisplay` = `formatAgeDisplay(calcAgeFromBirthDate(...))`, не «35 лет»;
  - без `birth_date` → `AgeDisplay` остаётся «35 лет» (пересчитывать не из чего);
  - `getPatientsByDoctor` нормализует так же.
- `gofmt -l` пуст, `go build -o /dev/null ./...`, `go vet ./...` — зелёные;
  `go test ./backend/` — `ok 7.970s`.

### Уроки сессии

- **Разнобой формата — это про чтение, а не про миграцию.** Достаточно
  пересчитывать на чтении: старые записи остаются как есть, врач видит
  единый формат, миграция не нужна. Если бы потребовался единый формат и
  в БД (для SQL-отчётов) — тогда отдельная миграция.
- **`patients.doctor_id` — FK на `doctors.id`.** В тесте
  `getPatientsByDoctor` нельзя вставить пациента с `doctor_id='DOC-AGE'`,
  не засидив доктора: `FOREIGN KEY constraint failed`. Паттерн —
  `INSERT INTO doctors (...) VALUES (?, NULL, ?, 'x', 'Test', 'lor',
  'doctor', datetime('now'), 1)` (см. `seedDoctorForRefresh`).
- **`calcAgeFromBirthDate` уже в том же пакете.** `db.go` может её звать
  без нового пакета и без цикла — helper рядом с вызовами, не отдельный
  модуль.

### Коммиты

2786a6f (8c) → docs.

## B30.8d — approve/deny без аутентификации → 401 (не 403) (2026-10-22)

Закрывает пункт бэклога «approve без токена → 403 vs 401 (семантика
soft-mode)». Семантика HTTP: 401 — «не аутентифицирован», 403 —
«аутентифицирован, но нет прав». В `handleConsentTransition` для
`approve`/`deny` (`requiredRole="patient"`) отсутствие аутентификации
отдавало 403 `patient only`, а ветка `revoke` (`default`) в том же файле —
корректный 401 `unauthorized`.

### Что сделано

- `backend/consent.go`, `case "patient"`: `pid == ""` → **401**
  `{"error":"unauthorized"}` вместо 403 `{"error":"patient only"}`.
  Комментарий у ветки: 403 ниже зарезервирован за «свой/чужой consent»
  (`c.PatientID != actorID`) — там он и остаётся.
- `backend/consent_test.go`: `TestConsentApprove_NoAuth_401` — запрос без
  `withPatientID`/`withDoctorID` → 401. Парный
  `TestConsentApprove_Forbidden_OtherPatient` (уже был) → 403: вместе
  закрепляют границу 401/403 с обеих сторон.

### Проверки

- `go test ./backend/ -run 'TestConsentApprove' -v` — Patient / Forbidden /
  NoAuth — PASS;
- `gofmt -l` пуст, `go build -o /dev/null ./...`, `go vet ./...` — зелёные;
  `go test ./backend/` — `ok 8.063s`.

### Уроки сессии

- **401 vs 403 — это «не аутентифицирован» vs «аутентифицирован, но не
  твой».** В одном файле уже была правильная ветка (`revoke` → 401), а
  `approve`/`deny` — неправильная (403). Асимметрию видно при чтении рядом:
  перед фиксом стоит сравнить все ветки с одинаковым смыслом «нет актора».
- **Проверка аутентификации — до `getConsent`.** В `handleConsentTransition`
  ветка `case "patient"` отрабатывает раньше похода в БД, поэтому тест на
  401 может передать произвольный `consentID` (до БД дело не дойдёт). Это же
  гарантирует, что «нет токена» не утекает в 404 «not found».
- **Пара тестов на границу лучше одного.** `NoAuth_401` +
  `Forbidden_OtherPatient` фиксируют оба края; без второго фикс мог бы
  случайно сделать 401 и на «чужой consent».

### Коммиты

ce93b5e (8d) → docs.

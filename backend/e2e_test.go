package main

// e2e_test.go — подпатч 6.21 (B12.1): E2E HTTP-тесты.
//
// Поднимает httptest.NewServer поверх http.DefaultServeMux
// с :memory: SQLite и всеми register*Routes. Проверяет HTTP-контракт:
// auth (login/refresh/logout), роли (401/403/200), CRUD, audit.
//
// Регистрация роутов — один раз (sync.Once), т.к. http.DefaultServeMux
// паникует при повторной регистрации одного пути.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------
// Одноразовая настройка окружения
// ---------------------------------------------------------------

var (
	e2eOnce   sync.Once
	e2eEnvSet bool
)

// e2eSetup выполняет одноразовые действия:
//   - InitDB(":memory:") → глобальная db
//   - currentEnv = тестовый (dev)
//   - register*Routes() на http.DefaultServeMux
//   - /api/health — вручную (его регистрирует main(), не register*)
//
// Возвращает *httptest.Server, обёрнутый в recoverMiddleware(authMiddleware).
func e2eSetup(t *testing.T) *httptest.Server {
	t.Helper()

	e2eOnce.Do(func() {
		var err error
		db, err = InitDB(":memory:")
		if err != nil {
			t.Fatalf("InitDB(:memory:): %v", err)
		}

		currentEnv = &Env{
			Name:         "dev",
			IsProd:       false,
			JWTSecret:    "e2e-test-secret-32-bytes-min-abcdef",
			AuthEnforce:  true,
			CookieSecure: false,
			DBPath:       "",
		}

		// Dev-seed: admin, superadmin, doctor (dev-аккаунты)
		if err := seedDefaultDoctor(db); err != nil {
			t.Fatalf("seedDefaultDoctor: %v", err)
		}
		if err := seedAdminRoles(db); err != nil {
			t.Fatalf("seedAdminRoles: %v", err)
		}

		http.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		})

		registerAuthRoutes()
		registerDoctorRoutes()
		registerPatientRoutes()
		registerAuditRoutes()

		e2eEnvSet = true
	})

	if !e2eEnvSet {
		t.Fatalf("e2eSetup: env не установлен")
	}

	handler := recoverMiddleware(authMiddleware(http.DefaultServeMux))
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

// ---------------------------------------------------------------
// HTTP-хелперы
// ---------------------------------------------------------------

// e2eReq — POST/GET с cookies и/или Bearer.
type e2eReq struct {
	method string
	url    string
	body   string
	bearer string
	cookie string // "name=value" — одна cookie
}

// e2eDo выполняет HTTP-запрос к тестовому серверу и возвращает (status, headers, body).
func e2eDo(t *testing.T, srv *httptest.Server, req e2eReq) (int, http.Header, string) {
	t.Helper()

	var bodyReader io.Reader
	if req.body != "" {
		bodyReader = strings.NewReader(req.body)
	}

	httpReq, err := http.NewRequest(req.method, srv.URL+req.url, bodyReader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if req.body != "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	if req.bearer != "" {
		httpReq.Header.Set("Authorization", "Bearer "+req.bearer)
	}
	if req.cookie != "" {
		httpReq.Header.Set("Cookie", req.cookie)
	}

	resp, err := srv.Client().Do(httpReq)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

// e2eLogin — логинится и возвращает (accessCookie, refreshCookie, role).
// Формат cookie: "botmax_access=...".
func e2eLogin(t *testing.T, srv *httptest.Server, email, password string) (string, string, string) {
	t.Helper()

	body := `{"email":"` + email + `","password":"` + password + `"}`
	status, hdr, respBody := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/auth/login", body: body,
	})
	if status != http.StatusOK {
		t.Fatalf("login %s: status %d, body=%s", email, status, respBody)
	}

	var access, refresh string
	for _, c := range hdr.Values("Set-Cookie") {
		if strings.HasPrefix(c, cookieNameAccess+"=") {
			access = strings.SplitN(c, ";", 2)[0]
		}
		if strings.HasPrefix(c, cookieNameRefresh+"=") {
			refresh = strings.SplitN(c, ";", 2)[0]
		}
	}
	if access == "" {
		t.Fatalf("login %s: нет cookie %s", email, cookieNameAccess)
	}

	role := ""
	if strings.Contains(respBody, `"role":"admin"`) {
		role = "admin"
	} else if strings.Contains(respBody, `"role":"superadmin"`) {
		role = "superadmin"
	} else if strings.Contains(respBody, `"role":"doctor"`) {
		role = "doctor"
	}
	return access, refresh, role
}

// ---------------------------------------------------------------
// Smoke: сервер поднимается, /api/health отвечает
// ---------------------------------------------------------------

func TestE2E_HealthSmoke(t *testing.T) {
	srv := e2eSetup(t)
	status, _, body := e2eDo(t, srv, e2eReq{method: http.MethodGet, url: "/api/health"})
	if status != http.StatusOK {
		t.Fatalf("health: status %d, body=%s", status, body)
	}
	if !strings.Contains(body, `"status":"ok"`) {
		t.Fatalf("health: unexpected body %s", body)
	}
}

// ---------------------------------------------------------------
// B12.2: auth-сценарии (login / refresh / logout / access)
// ---------------------------------------------------------------

func TestE2E_Login_Success(t *testing.T) {
	srv := e2eSetup(t)

	access, refresh, role := e2eLogin(t, srv, "admin@botmax.local", "admin")
	if access == "" || refresh == "" {
		t.Fatalf("login: пустые cookie (access=%q refresh=%q)", access, refresh)
	}
	if role != "admin" {
		t.Fatalf("login: role=%q, ожидали admin", role)
	}
}

func TestE2E_Login_Fail(t *testing.T) {
	srv := e2eSetup(t)

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost,
		url:    "/api/auth/login",
		body:   `{"email":"admin@botmax.local","password":"WRONG"}`,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("login WRONG: status %d, ожидали 401, body=%s", status, body)
	}
}

func TestE2E_Protected_NoAuth(t *testing.T) {
	srv := e2eSetup(t)

	status, _, body := e2eDo(t, srv, e2eReq{method: http.MethodGet, url: "/api/doctors"})
	if status != http.StatusUnauthorized {
		t.Fatalf("GET /api/doctors без auth: status %d, ожидали 401, body=%s", status, body)
	}
}

func TestE2E_Protected_WithCookie(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/doctors", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("GET /api/doctors с cookie: status %d, ожидали 200, body=%s", status, body)
	}
}

func TestE2E_Refresh_CookieFlow(t *testing.T) {
	srv := e2eSetup(t)
	_, refresh, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, hdr, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/auth/refresh", cookie: refresh,
	})
	if status != http.StatusOK {
		t.Fatalf("refresh без тела: status %d, ожидали 200, body=%s", status, body)
	}

	var gotAccess, gotRefresh bool
	for _, c := range hdr.Values("Set-Cookie") {
		if strings.HasPrefix(c, cookieNameAccess+"=") {
			gotAccess = true
		}
		if strings.HasPrefix(c, cookieNameRefresh+"=") {
			gotRefresh = true
		}
	}
	if !gotAccess || !gotRefresh {
		t.Fatalf("refresh: Set-Cookie не содержит access/refresh (access=%v refresh=%v)", gotAccess, gotRefresh)
	}
}

func TestE2E_Logout_CookiesCleared(t *testing.T) {
	srv := e2eSetup(t)
	access, refresh, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, hdr, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/auth/logout",
		cookie: access + "; " + refresh,
	})
	if status != http.StatusOK {
		t.Fatalf("logout: status %d, ожидали 200, body=%s", status, body)
	}

	var accessCleared, refreshCleared bool
	for _, c := range hdr.Values("Set-Cookie") {
		if strings.HasPrefix(c, cookieNameAccess+"=;") && strings.Contains(c, "Max-Age=0") {
			accessCleared = true
		}
		if strings.HasPrefix(c, cookieNameRefresh+"=;") && strings.Contains(c, "Max-Age=0") {
			refreshCleared = true
		}
	}
	if !accessCleared || !refreshCleared {
		t.Fatalf("logout: cookie не сняты (access=%v refresh=%v)", accessCleared, refreshCleared)
	}
}

func TestE2E_Logout_ThenAccess(t *testing.T) {
	srv := e2eSetup(t)
	access, refresh, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, _, _ := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/auth/logout",
		cookie: access + "; " + refresh,
	})
	if status != http.StatusOK {
		t.Fatalf("logout: status %d, ожидали 200", status)
	}

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/auth/refresh", cookie: refresh,
	})
	if status != http.StatusUnauthorized {
		t.Fatalf("refresh после logout: status %d, ожидали 401, body=%s", status, body)
	}
}

// ---------------------------------------------------------------
// B12.3: роли — 401/403/200
// ---------------------------------------------------------------

func TestE2E_Role_Doctor_Forbidden_Doctors(t *testing.T) {
	srv := e2eSetup(t)
	access, _, role := e2eLogin(t, srv, "doctor@local", "doctor@local")
	if role != "doctor" {
		t.Fatalf("login doctor: role=%q", role)
	}

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/doctors", cookie: access,
	})
	if status != http.StatusForbidden {
		t.Fatalf("doctor → /api/doctors: status %d, ожидали 403, body=%s", status, body)
	}
}

func TestE2E_Role_Doctor_Forbidden_Audit(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit", cookie: access,
	})
	if status != http.StatusForbidden {
		t.Fatalf("doctor → /api/audit: status %d, ожидали 403, body=%s", status, body)
	}
}

func TestE2E_Role_Admin_Allowed_Doctors(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/doctors", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("admin → /api/doctors: status %d, ожидали 200, body=%s", status, body)
	}
}

func TestE2E_Role_Admin_Allowed_Audit(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("admin → /api/audit: status %d, ожидали 200, body=%s", status, body)
	}
}

func TestE2E_Role_Superadmin_Allowed_Audit(t *testing.T) {
	srv := e2eSetup(t)
	access, _, role := e2eLogin(t, srv, "superadmin@botmax.local", "superadmin")
	if role != "superadmin" {
		t.Fatalf("login superadmin: role=%q", role)
	}

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("superadmin → /api/audit: status %d, ожидали 200, body=%s", status, body)
	}
}

func TestE2E_Role_Doctor_Allowed_Patients(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/patients", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("doctor → /api/patients: status %d, ожидали 200, body=%s", status, body)
	}
}

// ---------------------------------------------------------------
// B12.4: CRUD + audit
// ---------------------------------------------------------------

// extractJSONString — грубое извлечение значения "key":"..." из JSON-строки.
func extractJSONString(t *testing.T, body, key string) string {
	t.Helper()
	needle := `"` + key + `":"`
	i := strings.Index(body, needle)
	if i < 0 {
		t.Fatalf("extractJSONString: не нашли %q в %s", needle, body)
	}
	rest := body[i+len(needle):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatalf("extractJSONString: нет закрывающей кавычки для %q", key)
	}
	return rest[:j]
}

func TestE2E_Patient_CRUD(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Create
	createBody := `{"name":"E2E Patient","gender":"male","age":30}`
	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/patients", cookie: access, body: createBody,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d, ожидали 201, body=%s", status, body)
	}
	id := extractJSONString(t, body, "id")
	if id == "" {
		t.Fatalf("create: пустой id")
	}

	// List (содержит созданного)
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/patients", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("list: status %d, ожидали 200", status)
	}
	if !strings.Contains(body, id) {
		t.Fatalf("list: не содержит id=%s, body=%s", id, body)
	}

	// Update (PUT)
	updateBody := `{"id":"` + id + `","name":"E2E Patient 2","gender":"male","age":31}`
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodPut, url: "/api/patients/" + id, cookie: access, body: updateBody,
	})
	if status != http.StatusOK {
		t.Fatalf("update: status %d, ожидали 200, body=%s", status, body)
	}
	if !strings.Contains(body, "E2E Patient 2") {
		t.Fatalf("update: тело не содержит новое имя, body=%s", body)
	}

	// Delete (admin)
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodDelete, url: "/api/patients/" + id, cookie: access,
	})
	if status != http.StatusNoContent {
		t.Fatalf("delete: status %d, ожидали 204, body=%s", status, body)
	}
}

func TestE2E_Doctor_CRUD(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Create
	createBody := `{"email":"e2e-doc@botmax.local","password":"secret123","name":"E2E Doc","specialty":"lor","role":"doctor"}`
	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/doctors", cookie: access, body: createBody,
	})
	if status != http.StatusCreated {
		t.Fatalf("create doctor: status %d, ожидали 201, body=%s", status, body)
	}
	id := extractJSONString(t, body, "id")
	if id == "" {
		t.Fatalf("create doctor: пустой id")
	}

	// Patch
	patchBody := `{"name":"E2E Doc 2"}`
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodPatch, url: "/api/doctors/" + id, cookie: access, body: patchBody,
	})
	if status != http.StatusOK {
		t.Fatalf("patch doctor: status %d, ожидали 200, body=%s", status, body)
	}

	// Delete (soft)
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodDelete, url: "/api/doctors/" + id, cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("delete doctor: status %d, ожидали 200, body=%s", status, body)
	}
}

func TestE2E_Doctor_Create_Forbidden_For_Doctor(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	body := `{"email":"evil@e2e.local","password":"secret123","name":"Evil","role":"doctor"}`
	status, _, resp := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/doctors", cookie: access, body: body,
	})
	if status != http.StatusForbidden {
		t.Fatalf("doctor → POST /api/doctors: status %d, ожидали 403, body=%s", status, resp)
	}
}

func TestE2E_Audit_Records_Patient(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Create patient
	createBody := `{"name":"Audit Patient","gender":"male"}`
	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/patients", cookie: access, body: createBody,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d", status)
	}
	id := extractJSONString(t, body, "id")

	// Delete (чтобы гарантированно была запись delete — и create тоже)
	e2eDo(t, srv, e2eReq{
		method: http.MethodDelete, url: "/api/patients/" + id, cookie: access,
	})

	// Audit
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit?limit=100", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("audit: status %d", status)
	}
	if !strings.Contains(body, `"action":"patient.create"`) {
		t.Fatalf("audit: нет patient.create, body=%s", body[:min(len(body), 500)])
	}
	if !strings.Contains(body, `"action":"patient.delete"`) {
		t.Fatalf("audit: нет patient.delete, body=%s", body[:min(len(body), 500)])
	}
}

func TestE2E_Audit_Records_Doctor(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Create doctor
	createBody := `{"email":"audit-doc@botmax.local","password":"secret123","name":"Audit Doc","role":"doctor"}`
	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/doctors", cookie: access, body: createBody,
	})
	if status != http.StatusCreated {
		t.Fatalf("create: status %d, body=%s", status, body)
	}
	id := extractJSONString(t, body, "id")

	// Delete
	e2eDo(t, srv, e2eReq{
		method: http.MethodDelete, url: "/api/doctors/" + id, cookie: access,
	})

	// Audit
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit?limit=100", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("audit: status %d", status)
	}
	if !strings.Contains(body, `"action":"doctor.create"`) {
		t.Fatalf("audit: нет doctor.create")
	}
	if !strings.Contains(body, `"action":"doctor.delete"`) {
		t.Fatalf("audit: нет doctor.delete")
	}
}

func TestE2E_Audit_Filters(t *testing.T) {
	srv := e2eSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Создаём доктора — гарантирует doctor.create
	e2eDo(t, srv, e2eReq{
		method: http.MethodPost, url: "/api/doctors", cookie: access,
		body: `{"email":"filter-doc@botmax.local","password":"secret123","name":"Filter Doc","role":"doctor"}`,
	})

	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit?action=doctor.create&limit=100", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("audit filter: status %d", status)
	}
	if !strings.Contains(body, `"action":"doctor.create"`) {
		t.Fatalf("audit filter: нет doctor.create")
	}
	// Убедимся, что фильтр работает — нет других action
	if strings.Contains(body, `"action":"auth.login"`) {
		t.Fatalf("audit filter: фильтр по action не работает, в ответе есть auth.login")
	}

	// B16 (6.25): префиксный фильтр — action=doctor.*
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit?action=doctor.*&limit=100", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("audit filter: prefix status %d", status)
	}
	if !strings.Contains(body, `"action":"doctor.create"`) {
		t.Fatalf("audit filter: prefix не вернул doctor.create, body=%s", body[:min(len(body), 400)])
	}
	if strings.Contains(body, `"action":"auth.login"`) {
		t.Fatalf("audit filter: prefix захватил auth.login")
	}

	// Точный фильтр всё ещё работает (обратная совместимость)
	status, _, body = e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: "/api/audit?action=auth.login&limit=100", cookie: access,
	})
	if status != http.StatusOK {
		t.Fatalf("audit filter: exact status %d", status)
	}
	if !strings.Contains(body, `"action":"auth.login"`) {
		t.Fatalf("audit filter: exact не вернул auth.login")
	}
	if strings.Contains(body, `"action":"doctor.create"`) {
		t.Fatalf("audit filter: exact захватил doctor.create")
	}
}


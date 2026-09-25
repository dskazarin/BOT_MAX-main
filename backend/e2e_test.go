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

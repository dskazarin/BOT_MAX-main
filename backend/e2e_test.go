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

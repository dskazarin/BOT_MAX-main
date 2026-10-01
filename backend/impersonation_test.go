package main

// impersonation_test.go — подпатч 6.29 (B20.5): тесты impersonation.
//
// Unit-часть — без БД (db == nil): 405, 400, 200-noop, body-валидация.
// Integration-часть — через newTestDB(t) (см. refresh_test.go) +
// seedDoctorForRefresh + локальный seedDoctorWithRole, чтобы создавать
// superadmin/admin/doctor целей.
//
// Переиспользует fetchAuditRow/lastAuditID из audit_test.go.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------
// Хелперы
// ---------------------------------------------------------------

// reqWithActorCtx — httptest.Request с doctorID/role + actor в context.
// actorID=="" → обычная (не impersonation) сессия.
func reqWithActorCtx(method, path, doctorID string, role Role, actorID string, actorRole Role) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, role)
	ctx = withActor(ctx, actorID, actorRole)
	return httptest.NewRequest(method, path, nil).WithContext(ctx)
}

// readCookieFromRec — значение cookie с указанным именем или "".
func readCookieFromRec(t *testing.T, w *httptest.ResponseRecorder, name string) string {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

// seedDoctorWithRole — вставляет доктора с заданной ролью (doctor/admin/superadmin).
func seedDoctorWithRole(t *testing.T, db *sql.DB, id, role string, active bool) {
	t.Helper()
	activeInt := 0
	if active {
		activeInt = 1
	}
	_, err := db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
		 VALUES (?, NULL, ?, 'x', ?, 'lor', ?, datetime('now'), ?)`,
		id, id+"@test.local", id+" Name", role, activeInt,
	)
	if err != nil {
		t.Fatalf("insert doctor %s (role=%s): %v", id, role, err)
	}
}

// ---------------------------------------------------------------
// Unit: whoami
// ---------------------------------------------------------------

func TestHandleImpersonationWhoami_NotImpersonating(t *testing.T) {
	r := reqWithActorCtx(http.MethodGet, "/api/impersonation/whoami", "D1", RoleDoctor, "", "")
	w := httptest.NewRecorder()
	handleImpersonationWhoami(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var resp impersonationWhoamiResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v; body=%s", err, w.Body.String())
	}
	if resp.Impersonating {
		t.Fatalf("impersonating должен быть false, got body=%s", w.Body.String())
	}
	if resp.ActorID != "" || resp.TargetID != "" {
		t.Fatalf("поля должны быть пустыми, got %+v", resp)
	}
}

func TestHandleImpersonationWhoami_Impersonating_NoDB(t *testing.T) {
	// db == nil → target_name не заполняется, но actor/target_id — есть.
	r := reqWithActorCtx(http.MethodGet, "/api/impersonation/whoami",
		"DOC-1", RoleDoctor, "SUPER-1", RoleSuperadmin)
	w := httptest.NewRecorder()
	handleImpersonationWhoami(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var resp impersonationWhoamiResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Impersonating {
		t.Fatalf("impersonating должен быть true")
	}
	if resp.ActorID != "SUPER-1" || resp.ActorRole != string(RoleSuperadmin) {
		t.Fatalf("actor: got %+v", resp)
	}
	if resp.TargetID != "DOC-1" {
		t.Fatalf("target_id: got %q, want DOC-1", resp.TargetID)
	}
	if resp.TargetName != "" {
		t.Fatalf("target_name должен быть пустым без БД, got %q", resp.TargetName)
	}
}

func TestHandleImpersonationWhoami_MethodNotAllowed(t *testing.T) {
	r := reqWithActorCtx(http.MethodPost, "/api/impersonation/whoami", "D1", RoleDoctor, "", "")
	w := httptest.NewRecorder()
	handleImpersonationWhoami(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: got %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow: got %q, want GET", got)
	}
}

// ---------------------------------------------------------------
// Unit: start (без БД)
// ---------------------------------------------------------------

func TestHandleImpersonationStart_MethodNotAllowed(t *testing.T) {
	r := reqWithActorCtx(http.MethodGet, "/api/impersonation/start",
		"SUPER-1", RoleSuperadmin, "", "")
	w := httptest.NewRecorder()
	handleImpersonationStart(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: got %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow: got %q, want POST", got)
	}
}

func TestHandleImpersonationStart_NotSuperadmin(t *testing.T) {
	// admin пытается имперсонировать (self-check пропустим: actorID=="" — обычная).
	r := reqWithActorCtx(http.MethodPost, "/api/impersonation/start",
		"ADM-1", RoleAdmin, "", "")
	w := httptest.NewRecorder()
	handleImpersonationStart(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("code: got %d, want 403", w.Code)
	}
}

func TestHandleImpersonationStart_NoActorCtx(t *testing.T) {
	// Пустой context (мягкий режим): нет doctorID/role.
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", nil)
	w := httptest.NewRecorder()
	handleImpersonationStart(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code: got %d, want 401", w.Code)
	}
}

// ---------------------------------------------------------------
// Unit: stop (без БД)
// ---------------------------------------------------------------

func TestHandleImpersonationStop_MethodNotAllowed(t *testing.T) {
	r := reqWithActorCtx(http.MethodGet, "/api/impersonation/stop",
		"DOC-1", RoleDoctor, "SUPER-1", RoleSuperadmin)
	w := httptest.NewRecorder()
	handleImpersonationStop(w, r)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: got %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow: got %q, want POST", got)
	}
}

func TestHandleImpersonationStop_NotImpersonating_Noop(t *testing.T) {
	// Обычная сессия → 200 noop (restored=false), без cookie.
	r := reqWithActorCtx(http.MethodPost, "/api/impersonation/stop",
		"SUPER-1", RoleSuperadmin, "", "")
	w := httptest.NewRecorder()
	handleImpersonationStop(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	var resp impersonationStopResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.OK || resp.Restored {
		t.Fatalf("ожидали {ok:true, restored:false}, got %+v", resp)
	}
	if len(w.Result().Cookies()) != 0 {
		t.Fatalf("noop не должен ставить cookie, got %d", len(w.Result().Cookies()))
	}
}

// ---------------------------------------------------------------
// Integration: start (с БД)
// ---------------------------------------------------------------

// withGlobalDB подменяет пакетный db на тестовую БД и восстанавливает.
func withGlobalDB(t *testing.T, testDB *sql.DB) {
	t.Helper()
	prev := db
	db = testDB
	t.Cleanup(func() { db = prev })
}

func TestHandleImpersonationStart_Success(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)

	seedDoctorWithRole(t, testDB, "SUPER-1", "superadmin", true)
	seedDoctorWithRole(t, testDB, "DOC-1", "doctor", true)

	body := strings.NewReader(`{"doctor_id":"DOC-1"}`)
	r := reqWithActorCtx(http.MethodPost, "/api/impersonation/start",
		"SUPER-1", RoleSuperadmin, "", "").WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	r.Body = http.NoBody
	r = httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp impersonationStartResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.OK || resp.Target.ID != "DOC-1" || resp.Target.Role != "doctor" {
		t.Fatalf("resp: %+v", resp)
	}
	if resp.TTLSeconds != int(ImpersonationTTL.Seconds()) {
		t.Fatalf("ttl_seconds: got %d, want %d", resp.TTLSeconds, int(ImpersonationTTL.Seconds()))
	}

	tok := readCookieFromRec(t, w, cookieNameAccess)
	if tok == "" {
		t.Fatalf("Set-Cookie %s отсутствует", cookieNameAccess)
	}
	claims, err := ParseToken(tok, "access")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.DoctorID != "DOC-1" {
		t.Errorf("DoctorID: got %q, want DOC-1", claims.DoctorID)
	}
	if claims.ActorID != "SUPER-1" {
		t.Errorf("ActorID: got %q, want SUPER-1", claims.ActorID)
	}
	if claims.Role != RoleDoctor {
		t.Errorf("Role: got %q, want doctor", claims.Role)
	}
	if claims.ActAs != "DOC-1" {
		t.Errorf("ActAs: got %q, want DOC-1", claims.ActAs)
	}

	// audit-запись
	id := lastAuditID(t, testDB)
	_, doctorID, action, entity, entityID, _, detail := fetchAuditRow(t, testDB, id)
	if action != "impersonation.start" {
		t.Fatalf("action: got %q", action)
	}
	if doctorID != "SUPER-1" || entityID != "DOC-1" || entity != "doctor" {
		t.Fatalf("audit fields: doctorID=%q entity=%q entityID=%q", doctorID, entity, entityID)
	}
	if !strings.Contains(detail, "superadmin") {
		t.Fatalf("detail не содержит actor_role: %q", detail)
	}
}

func TestHandleImpersonationStart_SelfTarget(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)
	seedDoctorWithRole(t, testDB, "SUPER-1", "superadmin", true)

	body := strings.NewReader(`{"doctor_id":"SUPER-1"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "yourself") {
		t.Fatalf("ожидали 'yourself' в теле, got %s", w.Body.String())
	}
}

func TestHandleImpersonationStart_TargetNotFound(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)

	body := strings.NewReader(`{"doctor_id":"NOPE"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("code: got %d, want 404", w.Code)
	}
}

func TestHandleImpersonationStart_TargetInactive(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)
	seedDoctorWithRole(t, testDB, "DOC-OFF", "doctor", false)

	body := strings.NewReader(`{"doctor_id":"DOC-OFF"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "inactive") {
		t.Fatalf("ожидали 'inactive' в теле, got %s", w.Body.String())
	}
}

func TestHandleImpersonationStart_TargetSuperadmin(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)
	seedDoctorWithRole(t, testDB, "SUPER-2", "superadmin", true)

	body := strings.NewReader(`{"doctor_id":"SUPER-2"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "superadmin") {
		t.Fatalf("ожидали 'superadmin' в теле, got %s", w.Body.String())
	}
}

func TestHandleImpersonationStart_EmptyDoctorID(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)

	body := strings.NewReader(`{}`)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/start", body).WithContext(
		withDoctorID(context.Background(), "SUPER-1", RoleSuperadmin))
	w := httptest.NewRecorder()

	handleImpersonationStart(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, want 400", w.Code)
	}
}

// ---------------------------------------------------------------
// Integration: stop (с БД)
// ---------------------------------------------------------------

func TestHandleImpersonationStop_Success(t *testing.T) {
	setupImpersonationEnv(t)
	testDB := newTestDB(t)
	withGlobalDB(t, testDB)
	seedDoctorWithRole(t, testDB, "SUPER-1", "superadmin", true)
	seedDoctorWithRole(t, testDB, "DOC-1", "doctor", true)

	// ctx: активная impersonation-сессия.
	ctx := withDoctorID(context.Background(), "DOC-1", RoleDoctor)
	ctx = withActor(ctx, "SUPER-1", RoleSuperadmin)
	r := httptest.NewRequest(http.MethodPost, "/api/impersonation/stop", http.NoBody).WithContext(ctx)
	w := httptest.NewRecorder()

	handleImpersonationStop(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp impersonationStopResp
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.OK || !resp.Restored {
		t.Fatalf("resp: %+v, want {ok:true, restored:true}", resp)
	}

	tok := readCookieFromRec(t, w, cookieNameAccess)
	if tok == "" {
		t.Fatalf("Set-Cookie %s отсутствует", cookieNameAccess)
	}
	claims, err := ParseToken(tok, "access")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.DoctorID != "SUPER-1" {
		t.Errorf("DoctorID: got %q, want SUPER-1", claims.DoctorID)
	}
	if claims.Role != RoleSuperadmin {
		t.Errorf("Role: got %q, want superadmin", claims.Role)
	}
	if claims.ActorID != "" {
		t.Errorf("ActorID: got %q, want empty (restored)", claims.ActorID)
	}

	id := lastAuditID(t, testDB)
	_, doctorID, action, _, entityID, _, _ := fetchAuditRow(t, testDB, id)
	if action != "impersonation.stop" {
		t.Fatalf("action: got %q, want impersonation.stop", action)
	}
	if doctorID != "SUPER-1" {
		t.Fatalf("doctorID (actor): got %q, want SUPER-1", doctorID)
	}
	if entityID != "DOC-1" {
		t.Fatalf("entityID (target): got %q, want DOC-1", entityID)
	}
}

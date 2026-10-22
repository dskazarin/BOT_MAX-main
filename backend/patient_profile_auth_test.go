package main

// patient_profile_auth_test.go — B30.8a: регрессия patientAuthMiddleware
// на /api/patient/profile (закрывает пункт бэклога «hard-mode проверка»).
//
// Отличие от patient_profile_test.go: там хендлер вызывается напрямую
// с withPatientID в контексте (middleware байпасится). Здесь запрос идёт
// через patientAuthMiddleware, чтобы проверить:
//   - нет cookie botmax_pat_access          → 401 authorization required
//   - doctor access-токен в patient-cookie  → 401 invalid token (роль)
//   - валидный patient access-токен         → 200, свой профиль
//   - patient-токен чужого аккаунта         → 404 (нет утечки чужого профиля)

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// doProfileViaMiddleware — прогон GET /api/patient/profile через
// patientAuthMiddleware с заданным значением cookie ("" = без cookie).
func doProfileViaMiddleware(t *testing.T, cookieValue string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/patient/profile", nil)
	if cookieValue != "" {
		r.AddCookie(&http.Cookie{Name: cookieNamePatientAccess, Value: cookieValue})
	}
	rec := httptest.NewRecorder()
	patientAuthMiddleware(http.HandlerFunc(handlePatientProfile)).ServeHTTP(rec, r)
	return rec
}

// withProfileAuthEnv — временно ставит currentEnv с JWTSecret, чтобы
// IssueAccessToken/ParseToken работали в тестах (ср. withMetricsEnv).
func withProfileAuthEnv(t *testing.T) {
	t.Helper()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{Name: "dev", JWTSecret: "test-profile-auth-jwt-secret-32b!!"}
}

// Нет cookie → 401, тело содержит "authorization required".
func TestPatientProfileMiddleware_NoToken(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAccount(t, "PAT-MW1", "ACC-MW1")

	rec := doProfileViaMiddleware(t, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "authorization required") {
		t.Errorf("body = %q, want contains %q", got, "authorization required")
	}
}

// Doctor access-токен в patient-cookie → 401 invalid token (роль не та).
// Ключевая проверка пункта бэклога: doctor-middleware и patient-middleware
// не должны пересекаться.
func TestPatientProfileMiddleware_DoctorTokenRejected(t *testing.T) {
	setupPatientProfileDB(t)
	withProfileAuthEnv(t)
	seedPatientWithAccount(t, "PAT-MW2", "ACC-MW2")

	tok, err := IssueAccessToken("DOC-MW2", RoleDoctor)
	if err != nil {
		t.Fatalf("IssueAccessToken(doctor): %v", err)
	}

	rec := doProfileViaMiddleware(t, tok)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "invalid token") {
		t.Errorf("body = %q, want contains %q", got, "invalid token")
	}
}

// Валидный patient access-токен → 200, профиль своего пациента.
func TestPatientProfileMiddleware_PatientTokenOK(t *testing.T) {
	setupPatientProfileDB(t)
	withProfileAuthEnv(t)
	seedPatientWithAccount(t, "PAT-MW3", "ACC-MW3")

	tok, err := IssueAccessToken("ACC-MW3", RolePatient)
	if err != nil {
		t.Fatalf("IssueAccessToken(patient): %v", err)
	}

	rec := doProfileViaMiddleware(t, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "Test Patient") {
		t.Errorf("body = %q, want contains %q", got, "Test Patient")
	}
}

// Patient-токен чужого аккаунта → 404 (резолв не находит пациента),
// т.е. нет утечки чужого профиля.
func TestPatientProfileMiddleware_PatientTokenOtherAccount(t *testing.T) {
	setupPatientProfileDB(t)
	withProfileAuthEnv(t)
	seedPatientWithAccount(t, "PAT-MW4", "ACC-MW4")

	tok, err := IssueAccessToken("ACC-UNKNOWN", RolePatient)
	if err != nil {
		t.Fatalf("IssueAccessToken(other): %v", err)
	}

	rec := doProfileViaMiddleware(t, tok)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

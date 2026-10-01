package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireRole(t *testing.T) {
	cases := []struct {
		name     string
		ctxRole  Role
		hasRole  bool
		allowed  []Role
		wantCode int
	}{
		{name: "superadmin allowed for [admin, superadmin]", ctxRole: RoleSuperadmin, hasRole: true, allowed: []Role{RoleAdmin, RoleSuperadmin}, wantCode: http.StatusOK},
		{name: "admin allowed for [admin, superadmin]", ctxRole: RoleAdmin, hasRole: true, allowed: []Role{RoleAdmin, RoleSuperadmin}, wantCode: http.StatusOK},
		{name: "doctor denied for [admin, superadmin]", ctxRole: RoleDoctor, hasRole: true, allowed: []Role{RoleAdmin, RoleSuperadmin}, wantCode: http.StatusForbidden},
		{name: "no role in ctx -> 401", hasRole: false, allowed: []Role{RoleAdmin}, wantCode: http.StatusUnauthorized},
		{name: "single role match", ctxRole: RoleAdmin, hasRole: true, allowed: []Role{RoleAdmin}, wantCode: http.StatusOK},
		{name: "doctor denied for [admin] only", ctxRole: RoleDoctor, hasRole: true, allowed: []Role{RoleAdmin}, wantCode: http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.hasRole {
				ctx = context.WithValue(ctx, ctxKeyDoctorRole, tc.ctxRole)
			}
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			requireRole(tc.allowed...)(next).ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("code = %d, want %d", rec.Code, tc.wantCode)
			}
			if tc.wantCode == http.StatusOK && !called {
				t.Fatal("next handler not called on allow")
			}
			if tc.wantCode != http.StatusOK && called {
				t.Fatal("next handler called on deny")
			}
		})
	}
}

func TestRequireRoleInvalidRolePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for invalid role")
		}
	}()
	requireRole(Role("nurse"))
}

func TestRequireRoleEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty allowed")
		}
	}()
	requireRole()
}

func TestRoleValid(t *testing.T) {
	valid := []Role{RoleSuperadmin, RoleAdmin, RoleDoctor}
	for _, r := range valid {
		if !r.Valid() {
			t.Errorf("Role(%q).Valid() = false, want true", r)
		}
	}
	invalid := []Role{"", "nurse", "ADMIN", "Admin", "super_admin"}
	for _, r := range invalid {
		if r.Valid() {
			t.Errorf("Role(%q).Valid() = true, want false", r)
		}
	}
}

// ---------------------------------------------------------------
// B20.1 — IssueImpersonationToken
// ---------------------------------------------------------------

// TestIssueImpersonationToken_HappyPath — все claims выставлены корректно.
func TestIssueImpersonationToken_HappyPath(t *testing.T) {
	setupImpersonationEnv(t)
	tok, err := IssueImpersonationToken("actor-1", RoleSuperadmin, "doctor-1")
	if err != nil {
		t.Fatalf("IssueImpersonationToken: %v", err)
	}
	if tok == "" {
		t.Fatal("пустой токен")
	}

	claims, err := ParseToken(tok, "access")
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.DoctorID != "doctor-1" {
		t.Errorf("DoctorID: got %q, want %q", claims.DoctorID, "doctor-1")
	}
	if claims.Role != RoleDoctor {
		t.Errorf("Role: got %q, want %q", claims.Role, RoleDoctor)
	}
	if claims.ActorID != "actor-1" {
		t.Errorf("ActorID: got %q, want %q", claims.ActorID, "actor-1")
	}
	if claims.ActorRole != RoleSuperadmin {
		t.Errorf("ActorRole: got %q, want %q", claims.ActorRole, RoleSuperadmin)
	}
	if claims.ActAs != "doctor-1" {
		t.Errorf("ActAs: got %q, want %q", claims.ActAs, "doctor-1")
	}
	if claims.Type != "access" {
		t.Errorf("Type: got %q, want %q", claims.Type, "access")
	}
}

// TestIssueImpersonationToken_EmptyActor — пустой actorID → ошибка.
func TestIssueImpersonationToken_EmptyActor(t *testing.T) {
	setupImpersonationEnv(t)
	if _, err := IssueImpersonationToken("", RoleSuperadmin, "doctor-1"); err == nil {
		t.Fatal("ожидали ошибку при пустом actorID")
	}
}

// TestIssueImpersonationToken_EmptyTarget — пустой targetDoctorID → ошибка.
func TestIssueImpersonationToken_EmptyTarget(t *testing.T) {
	setupImpersonationEnv(t)
	if _, err := IssueImpersonationToken("actor-1", RoleSuperadmin, ""); err == nil {
		t.Fatal("ожидали ошибку при пустом targetDoctorID")
	}
}

// TestIssueImpersonationToken_SelfImpersonation — actorID == targetDoctorID → ошибка.
func TestIssueImpersonationToken_SelfImpersonation(t *testing.T) {
	setupImpersonationEnv(t)
	if _, err := IssueImpersonationToken("x", RoleSuperadmin, "x"); err == nil {
		t.Fatal("ожидали ошибку при self-impersonation")
	}
}

// TestIssueImpersonationToken_InvalidActorRole — невалидная роль → ошибка.
func TestIssueImpersonationToken_InvalidActorRole(t *testing.T) {
	setupImpersonationEnv(t)
	if _, err := IssueImpersonationToken("actor-1", Role("bogus"), "doctor-1"); err == nil {
		t.Fatal("ожидали ошибку при невалидной actorRole")
	}
}

// setupImpersonationEnv — минимальный env для тестов B20.1
// (IssueImpersonationToken + ParseToken). Восстанавливает currentEnv
// через t.Cleanup; jwtSecret() без currentEnv паникует.
func setupImpersonationEnv(t *testing.T) {
	t.Helper()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{
		Name:         "dev",
		IsProd:       false,
		JWTSecret:    "b20-test-secret-32-bytes-minimum-abcdef",
		AuthEnforce:  false,
		CookieSecure: false,
	}
}

// ---------------------------------------------------------------
// B20.2 — actor в контексте + requireNonImpersonated
// ---------------------------------------------------------------

// TestWithActor_NoActor — actorID=="" → контекст без изменений.
func TestWithActor_NoActor(t *testing.T) {
	ctx := context.Background()
	ctx2 := withActor(ctx, "", "")
	if ctx2 != ctx {
		t.Fatalf("withActor(\"\", \"\") должен вернуть тот же ctx")
	}
	if _, ok := actorIDFromCtx(reqWithCtx(ctx2)); ok {
		t.Fatalf("actorIDFromCtx должен вернуть false")
	}
}

// TestActorFromCtx_Empty — без withActor → ("", false).
func TestActorFromCtx_Empty(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if id, ok := actorIDFromCtx(r); ok || id != "" {
		t.Fatalf("actorIDFromCtx: got (%q,%v), want (\"\",false)", id, ok)
	}
	if role, ok := actorRoleFromCtx(r); ok || role != "" {
		t.Fatalf("actorRoleFromCtx: got (%q,%v), want (\"\",false)", role, ok)
	}
}

// TestActorFromCtx_Set — round-trip через withActor.
func TestActorFromCtx_Set(t *testing.T) {
	ctx := withActor(context.Background(), "actor-1", RoleSuperadmin)
	r := reqWithCtx(ctx)
	id, ok := actorIDFromCtx(r)
	if !ok || id != "actor-1" {
		t.Fatalf("actorIDFromCtx: got (%q,%v), want (\"actor-1\",true)", id, ok)
	}
	role, ok := actorRoleFromCtx(r)
	if !ok || role != RoleSuperadmin {
		t.Fatalf("actorRoleFromCtx: got (%q,%v), want (superadmin,true)", role, ok)
	}
}

// TestRequireNonImpersonated_Pass — обычная сессия: next вызван, 200.
func TestRequireNonImpersonated_Pass(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/doctors", nil)
	w := httptest.NewRecorder()
	called := false
	h := requireNonImpersonated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(w, r)
	if !called {
		t.Fatal("next не вызван для обычной сессии")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
}

// TestRequireNonImpersonated_Block — impersonation: 403, next НЕ вызван.
func TestRequireNonImpersonated_Block(t *testing.T) {
	ctx := withActor(context.Background(), "actor-1", RoleSuperadmin)
	r := httptest.NewRequest(http.MethodGet, "/api/doctors", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	called := false
	h := requireNonImpersonated(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	h.ServeHTTP(w, r)
	if called {
		t.Fatal("next вызван в impersonation-режиме — утечка эскалации")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("code: got %d, want 403", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type: got %q, want application/json", ct)
	}
}

// TestAuthMiddleware_PutsActorInCtx — интеграционный: impersonation-токен
// в Authorization проходит authMiddleware и кладёт actor в ctx.
func TestAuthMiddleware_PutsActorInCtx(t *testing.T) {
	setupImpersonationEnv(t)

	tok, err := IssueImpersonationToken("actor-1", RoleSuperadmin, "doctor-1")
	if err != nil {
		t.Fatalf("IssueImpersonationToken: %v", err)
	}

	r := httptest.NewRequest(http.MethodGet, "/api/patients", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()

	var gotActorID string
	var gotActorRole Role
	var gotDoctorID string
	var gotRole Role
	h := authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotActorID, _ = actorIDFromCtx(r)
		gotActorRole, _ = actorRoleFromCtx(r)
		gotDoctorID = doctorIDFromCtx(r)
		gotRole, _ = doctorRoleFromCtx(r)
		w.WriteHeader(http.StatusOK)
	}))
	h.ServeHTTP(w, r)

	if gotActorID != "actor-1" {
		t.Errorf("actorID: got %q, want actor-1", gotActorID)
	}
	if gotActorRole != RoleSuperadmin {
		t.Errorf("actorRole: got %q, want superadmin", gotActorRole)
	}
	if gotDoctorID != "doctor-1" {
		t.Errorf("doctorID: got %q, want doctor-1", gotDoctorID)
	}
	if gotRole != RoleDoctor {
		t.Errorf("role: got %q, want doctor", gotRole)
	}
}

// reqWithCtx — http.Request с заданным ctx (хелпер для тестов B20.2).
func reqWithCtx(ctx context.Context) *http.Request {
	return httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
}

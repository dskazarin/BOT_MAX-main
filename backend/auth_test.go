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

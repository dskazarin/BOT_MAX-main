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

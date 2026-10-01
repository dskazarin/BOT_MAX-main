package main

// doctors_test.go — подпатч 6.14 (B5.4): store-тесты CRUD по докторам.
//
// Переиспользует newTestDB(t) и seedDoctorForRefresh(t, db, id) из refresh_test.go.
// Все тесты — через :memory: SQLite, каждый — свой *sql.DB.
//
// Бизнес-правила (self-delete, admin vs superadmin) — проверены end-to-end
// в B5.3 smoke; здесь — только store.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ---------------------------------------------------------------
// Create + Get
// ---------------------------------------------------------------

func TestDoctors_CreateAndGet(t *testing.T) {
	db := newTestDB(t)

	doc, err := createDoctor(db, CreateDoctorInput{
		Email:     "new@test.local",
		Password:  "secret123",
		Name:      "Новый Доктор",
		Specialty: "lor",
		Role:      RoleDoctor,
		Phone:     "+7-999",
	})
	if err != nil {
		t.Fatalf("createDoctor: %v", err)
	}
	if doc.ID == "" {
		t.Fatalf("createDoctor вернул пустой id")
	}
	if doc.Email != "new@test.local" {
		t.Fatalf("email = %q", doc.Email)
	}
	if doc.PasswordHash == "" || doc.PasswordHash == "secret123" {
		t.Fatalf("пароль должен быть bcrypt-хеширован, got %q", doc.PasswordHash)
	}
	if !IsBcryptHash(doc.PasswordHash) {
		t.Fatalf("PasswordHash не похож на bcrypt: %q", doc.PasswordHash)
	}

	got, err := getDoctorRowByID(db, doc.ID)
	if err != nil {
		t.Fatalf("getDoctorRowByID: %v", err)
	}
	if got.ID != doc.ID || got.Email != doc.Email || got.Name != doc.Name {
		t.Fatalf("round-trip mismatch: %+v vs %+v", got, doc)
	}
	if !got.Active {
		t.Fatalf("свежий доктор должен быть active")
	}
}

// ---------------------------------------------------------------
// Defaults
// ---------------------------------------------------------------

func TestDoctors_CreateDefaults(t *testing.T) {
	db := newTestDB(t)

	doc, err := createDoctor(db, CreateDoctorInput{
		Email:    "d@test.local",
		Password: "x12345678",
		Name:     "Без specialty/role",
		// Specialty пустой → 'lor'; Role пустой → 'doctor'
	})
	if err != nil {
		t.Fatalf("createDoctor: %v", err)
	}
	if doc.Specialty != "lor" {
		t.Fatalf("specialty = %q, ожидали 'lor'", doc.Specialty)
	}
	if doc.Role != string(RoleDoctor) {
		t.Fatalf("role = %q, ожидали 'doctor'", doc.Role)
	}
}

// ---------------------------------------------------------------
// Validation
// ---------------------------------------------------------------

func TestDoctors_CreateValidation(t *testing.T) {
	db := newTestDB(t)

	cases := []struct {
		name string
		in   CreateDoctorInput
	}{
		{"empty-email", CreateDoctorInput{Password: "x", Name: "n"}},
		{"empty-password", CreateDoctorInput{Email: "e@test.local", Name: "n"}},
		{"empty-name", CreateDoctorInput{Email: "e@test.local", Password: "x"}},
		{"invalid-role", CreateDoctorInput{Email: "e@test.local", Password: "x", Name: "n", Role: Role("boss")}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := createDoctor(db, c.in)
			if err == nil {
				t.Fatalf("ожидали ошибку для %+v", c.in)
			}
		})
	}
}

func TestDoctors_CreateUniqueEmail(t *testing.T) {
	db := newTestDB(t)

	in := CreateDoctorInput{Email: "dup@test.local", Password: "x12345678", Name: "A"}
	if _, err := createDoctor(db, in); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, err := createDoctor(db, in)
	if err == nil {
		t.Fatalf("дубликат email должен был упасть")
	}
}

// ---------------------------------------------------------------
// List
// ---------------------------------------------------------------

func TestDoctors_List(t *testing.T) {
	db := newTestDB(t)

	// Создаём через store — сортировка superadmin→admin→doctor.
	if _, err := createDoctor(db, CreateDoctorInput{
		Email: "doc@t.local", Password: "x12345678", Name: "D", Role: RoleDoctor,
	}); err != nil {
		t.Fatalf("create doctor: %v", err)
	}
	if _, err := createDoctor(db, CreateDoctorInput{
		Email: "adm@t.local", Password: "x12345678", Name: "A", Role: RoleAdmin,
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := createDoctor(db, CreateDoctorInput{
		Email: "sa@t.local", Password: "x12345678", Name: "S", Role: RoleSuperadmin,
	}); err != nil {
		t.Fatalf("create superadmin: %v", err)
	}

	docs, err := listDoctors(db, false)
	if err != nil {
		t.Fatalf("listDoctors: %v", err)
	}
	if len(docs) != 3 {
		t.Fatalf("len = %d, ожидали 3", len(docs))
	}
	// Ожидаемый порядок: superadmin, admin, doctor
	wantRoles := []string{"superadmin", "admin", "doctor"}
	for i, want := range wantRoles {
		if docs[i].Role != want {
			t.Fatalf("docs[%d].Role = %q, ожидали %q", i, docs[i].Role, want)
		}
	}

	// Soft-delete одного — in list по умолчанию он не должен появиться.
	if err := deleteDoctorByID(db, docs[0].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	docs2, _ := listDoctors(db, false)
	if len(docs2) != 2 {
		t.Fatalf("после soft-delete: len = %d, ожидали 2", len(docs2))
	}
	// А с includeInactive=true — 3
	docs3, _ := listDoctors(db, true)
	if len(docs3) != 3 {
		t.Fatalf("includeInactive: len = %d, ожидали 3", len(docs3))
	}
}

// ---------------------------------------------------------------
// Update
// ---------------------------------------------------------------

func TestDoctors_Update(t *testing.T) {
	db := newTestDB(t)

	doc, err := createDoctor(db, CreateDoctorInput{
		Email: "u@t.local", Password: "x12345678", Name: "Old", Phone: "111",
	})
	if err != nil {
		t.Fatalf("createDoctor: %v", err)
	}

	// Частичное обновление: только name
	newname := "New"
	upd, err := updateDoctor(db, doc.ID, UpdateDoctorInput{Name: &newname})
	if err != nil {
		t.Fatalf("updateDoctor: %v", err)
	}
	if upd.Name != "New" {
		t.Fatalf("Name = %q, ожидали New", upd.Name)
	}
	if upd.Phone != "111" {
		t.Fatalf("Phone должен остаться 111, got %q", upd.Phone)
	}

	// Пустой in — ничего не меняет
	upd2, err := updateDoctor(db, doc.ID, UpdateDoctorInput{})
	if err != nil {
		t.Fatalf("updateDoctor empty: %v", err)
	}
	if upd2.Name != "New" {
		t.Fatalf("после empty-update Name = %q, ожидали New", upd2.Name)
	}

	// Обновление несуществующего — ErrNoRows
	_, err = updateDoctor(db, "no-such-id", UpdateDoctorInput{Name: &newname})
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update несуществующего: err = %v, ожидали ErrNoRows", err)
	}
}

// ---------------------------------------------------------------
// Delete (soft)
// ---------------------------------------------------------------

func TestDoctors_DeleteSoft(t *testing.T) {
	db := newTestDB(t)

	doc, err := createDoctor(db, CreateDoctorInput{
		Email: "del@t.local", Password: "x12345678", Name: "Del",
	})
	if err != nil {
		t.Fatalf("createDoctor: %v", err)
	}

	if err := deleteDoctorByID(db, doc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	got, err := getDoctorRowByID(db, doc.ID)
	if err != nil {
		t.Fatalf("get после delete: %v", err)
	}
	if got.Active {
		t.Fatalf("после delete Active должен быть false")
	}

	// Идемпотентность: повторный delete — тоже nil
	if err := deleteDoctorByID(db, doc.ID); err != nil {
		t.Fatalf("идемпотентный delete: %v", err)
	}

	// Несуществующий — ErrNoRows
	if err := deleteDoctorByID(db, "no-such"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("delete несуществующего: err = %v, ожидали ErrNoRows", err)
	}
}

// ---------------------------------------------------------------
// HTTP: handleDoctorsList — role-scoped visibility (B18)
// ---------------------------------------------------------------

// TestHandleDoctorsList_RoleFilter проверяет, что non-superadmin
// не видит в списке admin/superadmin, а superadmin видит всех.
// Подменяем глобальную db (см. main.go), т.к. handleDoctorsList
// читает её напрямую. Тесты не параллельные — подмена безопасна.
func TestHandleDoctorsList_RoleFilter(t *testing.T) {
	saved := db
	db = newTestDB(t)
	t.Cleanup(func() { db = saved })

	// Фикстуры:
	//   d1 — doctor, active
	//   d2 — doctor, soft-deleted (проверим include_inactive)
	//   a1 — admin
	//   s1 — superadmin
	d1, err := createDoctor(db, CreateDoctorInput{
		Email: "d1@t.local", Password: "x12345678", Name: "D1", Role: RoleDoctor,
	})
	if err != nil {
		t.Fatalf("create d1: %v", err)
	}
	d2, err := createDoctor(db, CreateDoctorInput{
		Email: "d2@t.local", Password: "x12345678", Name: "D2", Role: RoleDoctor,
	})
	if err != nil {
		t.Fatalf("create d2: %v", err)
	}
	if err := deleteDoctorByID(db, d2.ID); err != nil {
		t.Fatalf("soft-delete d2: %v", err)
	}
	if _, err := createDoctor(db, CreateDoctorInput{
		Email: "a1@t.local", Password: "x12345678", Name: "A1", Role: RoleAdmin,
	}); err != nil {
		t.Fatalf("create a1: %v", err)
	}
	if _, err := createDoctor(db, CreateDoctorInput{
		Email: "s1@t.local", Password: "x12345678", Name: "S1", Role: RoleSuperadmin,
	}); err != nil {
		t.Fatalf("create s1: %v", err)
	}
	_ = d1 // d1 используется только как фикстура

	cases := []struct {
		name            string
		role            Role
		hasRole         bool
		includeInactive bool
		wantLen         int
		wantRoles       []string
	}{
		{
			name: "admin sees only doctors (active)",
			role: RoleAdmin, hasRole: true,
			wantLen: 1, wantRoles: []string{"doctor"},
		},
		{
			name: "superadmin sees all (active)",
			role: RoleSuperadmin, hasRole: true,
			wantLen: 3, wantRoles: []string{"superadmin", "admin", "doctor"},
		},
		{
			name:    "no role in ctx → safe default (doctors only)",
			hasRole: false,
			wantLen: 1, wantRoles: []string{"doctor"},
		},
		{
			name: "admin + include_inactive=1 sees both doctors",
			role: RoleAdmin, hasRole: true, includeInactive: true,
			wantLen: 2, wantRoles: []string{"doctor", "doctor"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.hasRole {
				ctx = context.WithValue(ctx, ctxKeyDoctorRole, tc.role)
			}
			url := "/api/doctors"
			if tc.includeInactive {
				url += "?include_inactive=1"
			}
			req := httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
			rec := httptest.NewRecorder()

			handleDoctorsList(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("code = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			var got []*DoctorRow
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d (roles=%v)", len(got), tc.wantLen, rolesOf(got))
			}
			for i, wantRole := range tc.wantRoles {
				if got[i].Role != wantRole {
					t.Fatalf("got[%d].Role = %q, want %q (roles=%v)", i, got[i].Role, wantRole, rolesOf(got))
				}
			}
		})
	}
}

// rolesOf — хелпер для читаемых сообщений об ошибке.
func rolesOf(docs []*DoctorRow) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.Role
	}
	return out
}

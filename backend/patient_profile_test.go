package main

// patient_profile_test.go — B30.7: тесты self-profile.
//
// Стиль — patient_vitals_test.go: httptest + WithContext(withPatientID(accID)),
// пакетная db подменяется setupPatientProfileDB.
//
// withPatientID кладёт в ctx patientAccountID (patient_accounts.id),
// резолв accID → patients.id делает resolvePatientIDForSelf.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func setupPatientProfileDB(t *testing.T) {
	t.Helper()
	saved := db
	db = newTestDB(t)
	t.Cleanup(func() { db = saved })
}

func reqPatientProfile(accountID, method, body string) *http.Request {
	ctx := withPatientID(context.Background(), accountID)
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/api/patient/profile", nil)
	} else {
		r = httptest.NewRequest(method, "/api/patient/profile", strings.NewReader(body))
	}
	return r.WithContext(ctx)
}

// doProfile — прогон хендлера, возврат status + декодированный PatientProfile (если 200).
func doProfile(t *testing.T, accountID, method, body string) (int, *PatientProfile) {
	t.Helper()
	w := httptest.NewRecorder()
	handlePatientProfile(w, reqPatientProfile(accountID, method, body))
	if w.Code != http.StatusOK {
		return w.Code, nil
	}
	var p PatientProfile
	if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
		t.Fatalf("decode profile: %v (body=%s)", err, w.Body.String())
	}
	return w.Code, &p
}

func TestPatientProfile_GetEmpty(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAccount(t, "PAT-P1", "ACC-P1")

	code, p := doProfile(t, "ACC-P1", http.MethodGet, "")
	if code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", code)
	}
	if p.Name != "Test Patient" {
		t.Errorf("name = %q, want %q", p.Name, "Test Patient")
	}
	if p.BirthDate != "" || p.Gender != "" || p.Height != 0 || p.Weight != 0 {
		t.Errorf("expected empty profile, got %+v", p)
	}
}

func TestPatientProfile_PutHappyAndPersist(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAccount(t, "PAT-P2", "ACC-P2")

	body := `{"birthDate":"1990-01-01","gender":"male","height":180.5,"weight":75.2}`
	code, p := doProfile(t, "ACC-P2", http.MethodPut, body)
	if code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", code)
	}
	if p.BirthDate != "1990-01-01" || p.Gender != "male" {
		t.Errorf("birthDate/gender = %q/%q", p.BirthDate, p.Gender)
	}
	if p.Height != 180.5 || p.Weight != 75.2 {
		t.Errorf("height/weight = %v/%v", p.Height, p.Weight)
	}
	if p.Age <= 0 || p.AgeMonths <= 0 || p.AgeDisplay == "" {
		t.Errorf("age not recalculated: age=%d months=%d display=%q", p.Age, p.AgeMonths, p.AgeDisplay)
	}

	// Повторный GET — данные сохранились в БД.
	code, p2 := doProfile(t, "ACC-P2", http.MethodGet, "")
	if code != http.StatusOK {
		t.Fatalf("GET after PUT status = %d", code)
	}
	if p2.BirthDate != "1990-01-01" || p2.Height != 180.5 || p2.Weight != 75.2 {
		t.Errorf("persisted profile mismatch: %+v", p2)
	}
}

func TestPatientProfile_PutPartialKeepsOthers(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAccount(t, "PAT-P3", "ACC-P3")

	if code, _ := doProfile(t, "ACC-P3", http.MethodPut,
		`{"birthDate":"2000-05-10","gender":"female","height":165,"weight":55}`); code != http.StatusOK {
		t.Fatalf("first PUT status = %d", code)
	}
	// Меняем только вес.
	code, p := doProfile(t, "ACC-P3", http.MethodPut, `{"weight":56.5}`)
	if code != http.StatusOK {
		t.Fatalf("partial PUT status = %d", code)
	}
	if p.BirthDate != "2000-05-10" || p.Gender != "female" || p.Height != 165 {
		t.Errorf("partial PUT clobbered other fields: %+v", p)
	}
	if p.Weight != 56.5 {
		t.Errorf("weight = %v, want 56.5", p.Weight)
	}
}

func TestPatientProfile_PutInvalid(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAccount(t, "PAT-P4", "ACC-P4")

	cases := []struct {
		name string
		body string
	}{
		{"bad gender", `{"gender":"xxx"}`},
		{"bad birthDate", `{"birthDate":"01.01.1990"}`},
		{"future birthDate", `{"birthDate":"2999-01-01"}`},
		{"height too big", `{"height":999}`},
		{"height negative", `{"height":-1}`},
		{"weight too big", `{"weight":999}`},
		{"weight negative", `{"weight":-1}`},
		{"empty name", `{"name":"   "}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := doProfile(t, "ACC-P4", http.MethodPut, c.body)
			if code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body=%s)", code, c.body)
			}
		})
	}
}

func TestPatientProfile_NoAccount(t *testing.T) {
	setupPatientProfileDB(t)
	// Аккаунт не сеем — ctx пуст → resolvePatientIDForSelf → sql.ErrNoRows → 404.
	code, _ := doProfile(t, "", http.MethodGet, "")
	if code != http.StatusNotFound {
		t.Errorf("GET no-account status = %d, want 404", code)
	}
}

func TestCalcAgeFromBirthDate(t *testing.T) {
	// 2026-10-22 — «сегодня» из фикстуры.
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		in       string
		wantAge  int
		wantMon  int
		wantDisp string
		wantErr  bool
	}{
		{"", 0, 0, "", false},
		{"2026-10-22", 0, 0, "0 мес", false},
		{"2026-07-22", 0, 3, "3 мес", false},
		{"2025-10-22", 1, 12, "1 г", false},
		{"2023-05-15", 3, 41, "3 г 5 мес", false},
		{"1990-01-01", 36, 441, "36 г 9 мес", false},
		{"bad", 0, 0, "", true},
		{"2999-01-01", 0, 0, "", true},
	}
	for _, c := range cases {
		age, mon, disp, err := calcAgeFromBirthDate(c.in, now)
		if (err != nil) != c.wantErr {
			t.Errorf("calcAge(%q) err = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if err != nil {
			continue
		}
		if age != c.wantAge || mon != c.wantMon || disp != c.wantDisp {
			t.Errorf("calcAge(%q) = (%d,%d,%q), want (%d,%d,%q)",
				c.in, age, mon, disp, c.wantAge, c.wantMon, c.wantDisp)
		}
	}
}

package main

// consent_scope_b30_9b_test.go — B30.9b: белый список scope в consents.
// Схема consents не менялась, расширение значениями TEXT (этап 2 §6.58).
// Стиль — как в consent_test.go: setupConsentDB + reqConsentRequest
// + прямой вызов handleConsentRequest.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConsentScope_NormalizeHelper(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"", "vitals_read", true},
		{"vitals_read", "vitals_read", true},
		{"symptoms_read", "symptoms_read", true},
		{"allergies_read", "allergies_read", true},
		{"vaccinations_read", "vaccinations_read", true},
		{"diagnoses_read", "diagnoses_read", true},
		{"constant_medications_read", "constant_medications_read", true},
		{"course_medications_read", "course_medications_read", true},
		{"files_read", "files_read", true},
		{"constant_medications_read", "constant_medications_read", true},
		{"course_medications_read", "course_medications_read", true},
		{"files_read", "files_read", true},
		{"constant_medications_read", "constant_medications_read", true},
		{"course_medications_read", "course_medications_read", true},
		{"files_read", "files_read", true},
		{"constant_medications_write", "constant_medications_write", true},
		{"course_medications_write", "course_medications_write", true},
		{"files_write", "files_write", true},
		{"  files_write  ", "files_write", true},
		{"  symptoms_read  ", "symptoms_read", true},
		{"bogus", "", false},
		{"VITALS_READ", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeConsentScope(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("normalizeConsentScope(%q) = (%q,%v), want (%q,%v)",
				c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestConsentScope_UpsertRejectsBogus(t *testing.T) {
	setupConsentDB(t)
	_, err := upsertConsentRequest(db, "PAT-X", "DOC-X", "bogus")
	if err == nil || !strings.Contains(err.Error(), "invalid consent scope") {
		t.Fatalf("want invalid consent scope error, got %v", err)
	}
}

func TestConsentScope_RequestAcceptsSymptomsRead(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "pending")

	// Удалим существующий vitals_read, чтобы проверять именно наш scope.
	if _, err := db.Exec(`DELETE FROM consents WHERE patient_id=? AND doctor_id=?`,
		patientID, doctorID); err != nil {
		t.Fatalf("cleanup consents: %v", err)
	}

	body := `{"patientId":"` + patientID + `","scope":"symptoms_read"}`
	req := reqConsentRequest(doctorID, body)
	rr := httptest.NewRecorder()
	handleConsentRequest(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var scope string
	if err := db.QueryRow(`SELECT scope FROM consents WHERE patient_id=? AND doctor_id=?`,
		patientID, doctorID).Scan(&scope); err != nil {
		t.Fatalf("select scope: %v", err)
	}
	if scope != "symptoms_read" {
		t.Errorf("scope = %q, want symptoms_read", scope)
	}
}

func TestConsentScope_RequestRejectsBogus(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "pending")

	body := `{"patientId":"` + patientID + `","scope":"bogus"}`
	req := reqConsentRequest(doctorID, body)
	rr := httptest.NewRecorder()
	handleConsentRequest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM consents WHERE patient_id=? AND doctor_id=? AND scope='bogus'`,
		patientID, doctorID).Scan(&n)
	if n != 0 {
		t.Errorf("bogus consent persisted: %d", n)
	}
}

func TestConsentScope_RequestEmptyDefaultsToVitals(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "pending")

	// Удалим существующий, чтобы проверять создание именно с пустым scope.
	if _, err := db.Exec(`DELETE FROM consents WHERE patient_id=? AND doctor_id=?`,
		patientID, doctorID); err != nil {
		t.Fatalf("cleanup consents: %v", err)
	}

	body := `{"patientId":"` + patientID + `"}`
	req := reqConsentRequest(doctorID, body)
	rr := httptest.NewRecorder()
	handleConsentRequest(rr, req)

	if rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var scope string
	if err := db.QueryRow(`SELECT scope FROM consents WHERE patient_id=? AND doctor_id=?`,
		patientID, doctorID).Scan(&scope); err != nil {
		t.Fatalf("select scope: %v", err)
	}
	if scope != "vitals_read" {
		t.Errorf("scope = %q, want vitals_read (default)", scope)
	}
}

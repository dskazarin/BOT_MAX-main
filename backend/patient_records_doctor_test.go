package main

// patient_records_doctor_test.go — B30.9d: чтение врачом с consent-проверкой.
// Стиль — consent_test.go: setupConsentDB + reqConsentRequest для doctor-ctx,
// сюда добавляем seedPatientWithAccount для записей.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reqDoctorRecords — GET /api/patients/{pid}/{kind} с doctorID в ctx.
func reqDoctorRecords(doctorID, pid, kind string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodGet, "/api/patients/"+pid+"/"+kind, nil).WithContext(ctx)
}

func approveConsent(t *testing.T, pid, doctorID, scope string) {
	t.Helper()
	id, err := upsertConsentRequest(db, pid, doctorID, scope)
	if err != nil {
		t.Fatalf("upsertConsentRequest: %v", err)
	}
	if err := transitionConsent(db, id, "pending", "approved"); err != nil {
		t.Fatalf("transition approved: %v", err)
	}
}

func seedSymptom(t *testing.T, pid, id, name string) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO symptoms (id, patient_id, name, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`, id, pid, name)
	if err != nil {
		t.Fatalf("seed symptom: %v", err)
	}
}

func TestDoctorRecords_NoAuth_401(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	r := httptest.NewRequest(http.MethodGet, "/api/patients/PAT-1/symptoms", nil)
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestDoctorRecords_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	seedSymptom(t, "PAT-1", "SYM-1", "кашель")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "symptoms"), "PAT-1", "symptoms")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "consent_required") {
		t.Errorf("body = %s, want consent_required", rr.Body.String())
	}
}

func TestDoctorRecords_ApprovedScope_WrongKind_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	// consent только на symptoms_read; запрос на allergies → 403.
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "allergies"), "PAT-1", "allergies")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestDoctorRecords_ApprovedScope_Ok(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	seedSymptom(t, "PAT-1", "SYM-1", "кашель")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "symptoms"), "PAT-1", "symptoms")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != "SYM-1" {
		t.Fatalf("list = %+v", list)
	}
}

func TestDoctorRecords_SoftDeletedHidden(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read")

	// Живая и удалённая
	seedSymptom(t, "PAT-1", "SYM-1", "живая")
	if _, err := db.Exec(`INSERT INTO symptoms (id, patient_id, name, created_at, updated_at, deleted_at)
		VALUES ('SYM-2','PAT-1','удалённая', datetime('now'), datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed deleted symptom: %v", err)
	}

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "symptoms"), "PAT-1", "symptoms")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["id"] != "SYM-1" {
		t.Fatalf("list = %+v (удалённая не должна быть)", list)
	}
}

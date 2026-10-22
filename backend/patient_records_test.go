package main

// patient_records_test.go — B30.9c: self-records пациента.
// Стиль — patient_vitals_test.go: setupPatientVitalsDB + seedPatientWithAccount
// + withPatientID(accID).

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func reqRecords(accID, method, path, body string) *http.Request {
	ctx := withPatientID(context.Background(), accID)
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	return r.WithContext(ctx)
}

func TestPatientRecords_NoAuth_401(t *testing.T) {
	setupPatientVitalsDB(t)
	r := httptest.NewRequest(http.MethodGet, "/api/patient/records/symptoms", nil)
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, r)
	if rr.Code != http.StatusNotFound && rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401/404", rr.Code)
	}
}

func TestPatientRecords_UpsertAndList(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	// clinic_id — снимок; проставим пациенту клинику.
	if _, err := db.Exec(`UPDATE patients SET clinic_id=NULL WHERE id='PAT-1'`); err != nil {
		t.Fatalf("update clinic_id: %v", err)
	}

	body := `{"id":"SYM-1","name":"кашель","severity":"mild","notes":"2 дня"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/symptoms", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodGet, "/api/patient/records/symptoms", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rr.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(list) != 1 || list[0]["id"] != "SYM-1" || list[0]["name"] != "кашель" {
		t.Fatalf("list = %+v", list)
	}
}

func TestPatientRecords_UpsertUpdatesInPlace(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	body1 := `{"id":"SYM-1","name":"кашель","severity":"mild"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/symptoms", body1))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST1 status = %d body=%s", rr.Code, rr.Body.String())
	}
	var created string
	db.QueryRow(`SELECT created_at FROM symptoms WHERE id='SYM-1'`).Scan(&created)

	body2 := `{"id":"SYM-1","name":"кашель","severity":"severe"}`
	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/symptoms", body2))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST2 status = %d body=%s", rr.Code, rr.Body.String())
	}
	var sev, created2 string
	db.QueryRow(`SELECT severity, created_at FROM symptoms WHERE id='SYM-1'`).Scan(&sev, &created2)
	if sev != "severe" {
		t.Errorf("severity = %q, want severe", sev)
	}
	if created != created2 {
		t.Errorf("created_at changed: %q -> %q", created, created2)
	}
}

func TestPatientRecords_SoftDelete(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	body := `{"id":"SYM-1","name":"кашель"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/symptoms", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodDelete, "/api/patient/records/symptoms/SYM-1", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d body=%s", rr.Code, rr.Body.String())
	}
	var deleted sql.NullString
	db.QueryRow(`SELECT deleted_at FROM symptoms WHERE id='SYM-1'`).Scan(&deleted)
	if !deleted.Valid || deleted.String == "" {
		t.Errorf("deleted_at not set: %+v", deleted)
	}

	// повторный DELETE — 404
	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodDelete, "/api/patient/records/symptoms/SYM-1", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("second DELETE status = %d, want 404", rr.Code)
	}
}

func TestPatientRecords_DeleteOtherPatient_404(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedPatientWithAccount(t, "PAT-2", "ACC-2")

	body := `{"id":"SYM-1","name":"x"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/symptoms", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("seed POST status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-2", http.MethodDelete, "/api/patient/records/symptoms/SYM-1", ""))
	if rr.Code != http.StatusNotFound {
		t.Errorf("foreign DELETE status = %d, want 404", rr.Code)
	}
	var deleted sql.NullString
	db.QueryRow(`SELECT deleted_at FROM symptoms WHERE id='SYM-1'`).Scan(&deleted)
	if deleted.Valid {
		t.Errorf("foreign record was soft-deleted")
	}
}

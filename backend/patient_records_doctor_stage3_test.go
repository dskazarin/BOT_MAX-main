package main

// patient_records_doctor_stage3_test.go — B30.9g (этап 3 §6.58): doctor-read
// для constant_medications / course_medications / files с consent-scope
// <kind>_read. Generic handleDoctorPatientRecords уже обслуживает новые kind
// (whitelist расширен в 3b), здесь проверяем 401/403/200 и скрытие soft-deleted.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedStage3Record — прямая вставка строки в новую таблицу (без generic upsert,
// чтобы не зависеть от recordFields и явно контролировать значения).
func seedStage3Record(t *testing.T, table, pid, id, col, val string) {
	t.Helper()
	q := `INSERT INTO ` + table + ` (id, patient_id, ` + col + `, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`
	if _, err := db.Exec(q, id, pid, val); err != nil {
		t.Fatalf("seed %s: %v", table, err)
	}
}

func TestDoctorRecords_Stage3_NoAuth_401(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	for _, kind := range []string{"constant_medications", "course_medications", "files"} {
		r := httptest.NewRequest(http.MethodGet, "/api/patients/PAT-1/"+kind, nil)
		rr := httptest.NewRecorder()
		handleDoctorPatientRecords(rr, r, "PAT-1", kind)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", kind, rr.Code)
		}
	}
}

func TestDoctorRecords_Stage3_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	for _, kind := range []string{"constant_medications", "course_medications", "files"} {
		rr := httptest.NewRecorder()
		handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", kind), "PAT-1", kind)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403; body=%s", kind, rr.Code, rr.Body.String())
			continue
		}
		if !strings.Contains(rr.Body.String(), "consent_required") {
			t.Errorf("%s: body = %s, want consent_required", kind, rr.Body.String())
		}
	}
}

func TestDoctorRecords_Stage3_WrongScope_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	// consent только на files_read; запрос на constant_medications → 403.
	approveConsent(t, "PAT-1", "DOC-1", "files_read")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "constant_medications"),
		"PAT-1", "constant_medications")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rr.Code)
	}
}

func TestDoctorRecords_Stage3_ApprovedScope_Ok(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	seedStage3Record(t, "constant_medications", "PAT-1", "CM-1", "name", "метформин")
	seedStage3Record(t, "course_medications", "PAT-1", "CR-1", "name", "амоксициллин")
	seedStage3Record(t, "files", "PAT-1", "F-1", "filename", "анализ.pdf")

	for _, c := range []struct{ kind, id, key, val string }{
		{"constant_medications", "CM-1", "name", "метформин"},
		{"course_medications", "CR-1", "name", "амоксициллин"},
		{"files", "F-1", "filename", "анализ.pdf"},
	} {
		approveConsent(t, "PAT-1", "DOC-1", c.kind+"_read")

		rr := httptest.NewRecorder()
		handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", c.kind), "PAT-1", c.kind)
		if rr.Code != http.StatusOK {
			t.Errorf("%s: status = %d, body=%s", c.kind, rr.Code, rr.Body.String())
			continue
		}
		var list []map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
			t.Errorf("%s: decode: %v", c.kind, err)
			continue
		}
		if len(list) != 1 || list[0]["id"] != c.id {
			t.Errorf("%s: list = %+v, want id=%s", c.kind, list, c.id)
			continue
		}
		if got, _ := list[0][c.key].(string); got != c.val {
			t.Errorf("%s: %s = %q, want %q", c.kind, c.key, got, c.val)
		}
	}
}

func TestDoctorRecords_Stage3_SoftDeletedHidden(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	seedStage3Record(t, "constant_medications", "PAT-1", "CM-1", "name", "живая")
	if _, err := db.Exec(`INSERT INTO constant_medications
		(id, patient_id, name, created_at, updated_at, deleted_at)
		VALUES ('CM-2','PAT-1','удалённая', datetime('now'), datetime('now'), datetime('now'))`); err != nil {
		t.Fatalf("seed deleted: %v", err)
	}
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_read")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "constant_medications"),
		"PAT-1", "constant_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["id"] != "CM-1" {
		t.Fatalf("list = %+v (удалённая не должна быть)", list)
	}
}

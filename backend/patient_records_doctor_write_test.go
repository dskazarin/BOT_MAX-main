package main

// patient_records_doctor_write_test.go — 3g (этап 3 §6.58): doctor-write
// для constant_medications / course_medications / files с consent-scope
// <kind>_write. Проверяем 401/403/200, разделение read/write, soft-delete,
// partial upsert (status DEFAULT 'active'), изоляцию чужого пациента.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// reqDoctorWrite — запрос от имени доктора с телом (для POST/PUT) или без (DELETE).
func reqDoctorWrite(method, doctorID, path, body string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	return r.WithContext(ctx)
}

func TestDoctorWrite_NoAuth_401(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/api/patients/PAT-1/constant_medications", `{"id":"CM-1","name":"x"}`},
		{http.MethodPut, "/api/patients/PAT-1/course_medications", `{"id":"CR-1","name":"x"}`},
		{http.MethodDelete, "/api/patients/PAT-1/files/F-1", ""},
	}
	for _, c := range cases {
		r := httptest.NewRequest(c.method, c.path, strings.NewReader(c.body))
		rr := httptest.NewRecorder()
		if c.method == http.MethodDelete {
			handleDoctorPatientRecordDelete(rr, r, "PAT-1", "files", "F-1")
		} else {
			handleDoctorPatientRecords(rr, r, "PAT-1", "constant_medications")
		}
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", c.method, c.path, rr.Code)
		}
	}
}

func TestDoctorWrite_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/constant_medications", `{"id":"CM-1","name":"x"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "constant_medications")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("status=%d body=%s, want 403 consent_required", rr.Code, rr.Body.String())
	}
}

func TestDoctorWrite_ReadOnly_WriteDenied_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_read")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/constant_medications", `{"id":"CM-1","name":"x"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "constant_medications")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("read≠write: status=%d body=%s, want 403", rr.Code, rr.Body.String())
	}
}

func TestDoctorWrite_PostCreates_ThenReadSees(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_write")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_read")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/constant_medications",
		`{"id":"CM-1","name":"метформин","dose":"500 мг"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "constant_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var res map[string]any
	json.Unmarshal(rr.Body.Bytes(), &res)
	if res["ok"] != true || res["id"] != "CM-1" {
		t.Fatalf("POST res = %+v", res)
	}

	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "constant_medications"),
		"PAT-1", "constant_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET: status=%d", rr.Code)
	}
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 1 || list[0]["id"] != "CM-1" || list[0]["name"] != "метформин" {
		t.Fatalf("GET list = %+v", list)
	}
}

func TestDoctorWrite_PutUpdates_200(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "course_medications_write")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/course_medications",
		`{"id":"CR-1","name":"амоксициллин","status":"active"}`), "PAT-1", "course_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPut, "DOC-1",
		"/api/patients/PAT-1/course_medications",
		`{"id":"CR-1","status":"completed"}`), "PAT-1", "course_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}

	var status, name string
	if err := db.QueryRow(`SELECT status, name FROM course_medications WHERE id='CR-1'`).
		Scan(&status, &name); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "completed" || name != "амоксициллин" {
		t.Fatalf("after PUT: status=%q name=%q", status, name)
	}
}

func TestDoctorWrite_CourseMed_PartialUpsert_StatusDefault(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "course_medications_write")

	// Без status — не должно падать (status NOT NULL DEFAULT 'active').
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/course_medications",
		`{"id":"CR-2","name":"амоксициллин"}`), "PAT-1", "course_medications")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST no-status: %d %s", rr.Code, rr.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM course_medications WHERE id='CR-2'`).Scan(&status); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %q, want active (DEFAULT)", status)
	}
}

func TestDoctorWrite_WrongPatient_404(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-NOPE/constant_medications", `{"id":"CM-1","name":"x"}`)
	handleDoctorPatientRecords(rr, r, "PAT-NOPE", "constant_medications")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rr.Code)
	}
}

func TestDoctorWrite_Delete_ThenHiddenAndRepeat404(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_write")
	approveConsent(t, "PAT-1", "DOC-1", "constant_medications_read")

	seedStage3Record(t, "constant_medications", "PAT-1", "CM-1", "name", "метформин")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodDelete, "DOC-1",
		"/api/patients/PAT-1/constant_medications/CM-1", "")
	handleDoctorPatientRecordDelete(rr, r, "PAT-1", "constant_medications", "CM-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", rr.Code, rr.Body.String())
	}

	// GET не видит.
	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "constant_medications"),
		"PAT-1", "constant_medications")
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 0 {
		t.Fatalf("после DELETE list = %+v, want empty", list)
	}

	// Повторный DELETE → 404.
	rr = httptest.NewRecorder()
	handleDoctorPatientRecordDelete(rr, reqDoctorWrite(http.MethodDelete, "DOC-1",
		"/api/patients/PAT-1/constant_medications/CM-1", ""), "PAT-1", "constant_medications", "CM-1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("repeat DELETE: %d, want 404", rr.Code)
	}
}

func TestDoctorWrite_Files_ReadOnly_DeleteDenied_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_read")
	seedStage3Record(t, "files", "PAT-1", "F-1", "filename", "x.pdf")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecordDelete(rr, reqDoctorWrite(http.MethodDelete, "DOC-1",
		"/api/patients/PAT-1/files/F-1", ""), "PAT-1", "files", "F-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("files read-only DELETE: %d %s, want 403", rr.Code, rr.Body.String())
	}
}

func TestDoctorWrite_Files_Delete_200(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")
	seedStage3Record(t, "files", "PAT-1", "F-1", "filename", "x.pdf")

	rr := httptest.NewRecorder()
	handleDoctorPatientRecordDelete(rr, reqDoctorWrite(http.MethodDelete, "DOC-1",
		"/api/patients/PAT-1/files/F-1", ""), "PAT-1", "files", "F-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", rr.Code, rr.Body.String())
	}
	var deletedAt *string
	db.QueryRow(`SELECT deleted_at FROM files WHERE id='F-1'`).Scan(&deletedAt)
	if deletedAt == nil {
		t.Fatalf("deleted_at IS NULL, want set")
	}
}

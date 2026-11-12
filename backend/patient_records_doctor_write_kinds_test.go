package main

// patient_records_doctor_write_kinds_test.go — B30.17: doctor-write для
// symptoms/allergies/vaccinations/diagnoses через generic handleDoctorPatientRecords
// (kind+"_write" consent). Паттерн — patient_records_doctor_write_test.go (3g).

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// --- symptoms ---

func TestDoctorWriteKinds_Symptoms_NoAuth_401(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	r := httptest.NewRequest(http.MethodPost, "/api/patients/PAT-1/symptoms",
		strings.NewReader(`{"id":"SYM-1","name":"кашель"}`))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rr.Code)
	}
}

func TestDoctorWriteKinds_Symptoms_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/symptoms", `{"id":"SYM-1","name":"кашель"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("status=%d body=%s, want 403 consent_required", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Symptoms_ReadOnly_WriteDenied_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/symptoms", `{"id":"SYM-1","name":"кашель"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("read!=write: status=%d body=%s, want 403", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Symptoms_ApprovedWrite_Ok(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/symptoms",
		`{"id":"SYM-1","name":"кашель","severity":"mild","notes":"3 дня"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s, want 200 ok", rr.Code, rr.Body.String())
	}

	list, err := listPatientRecords("symptoms", "PAT-1")
	if err != nil {
		t.Fatalf("listPatientRecords: %v", err)
	}
	var found bool
	for _, rec := range list {
		if rec["id"] == "SYM-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("SYM-1 not found in listPatientRecords: %v", list)
	}
}

func TestDoctorWriteKinds_Symptoms_InvalidPayload_400(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/symptoms", `{not json`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Symptoms_ReadAfterWrite(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read")
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/symptoms", `{"id":"SYM-1","name":"кашель"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r = reqDoctorWrite(http.MethodGet, "DOC-1",
		"/api/patients/PAT-1/symptoms", "")
	handleDoctorPatientRecords(rr, r, "PAT-1", "symptoms")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"SYM-1"`) {
		t.Fatalf("read status=%d body=%s, want 200 с SYM-1", rr.Code, rr.Body.String())
	}
}

// --- allergies ---

func TestDoctorWriteKinds_Allergies_NoAuth_401(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	r := httptest.NewRequest(http.MethodPost, "/api/patients/PAT-1/allergies",
		strings.NewReader(`{"id":"ALG-1","allergen":"пенициллин"}`))
	r.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", rr.Code)
	}
}

func TestDoctorWriteKinds_Allergies_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/allergies", `{"id":"ALG-1","allergen":"пенициллин"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("status=%d body=%s, want 403 consent_required", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Allergies_ReadOnly_WriteDenied_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "allergies_read")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/allergies", `{"id":"ALG-1","allergen":"пенициллин"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("read!=write: status=%d body=%s, want 403", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Allergies_ApprovedWrite_Ok(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "allergies_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/allergies",
		`{"id":"ALG-1","allergen":"пенициллин","reaction":"сыпь","severity":"moderate"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ok":true`) {
		t.Fatalf("status=%d body=%s, want 200 ok", rr.Code, rr.Body.String())
	}

	list, err := listPatientRecords("allergies", "PAT-1")
	if err != nil {
		t.Fatalf("listPatientRecords: %v", err)
	}
	var found bool
	for _, rec := range list {
		if rec["id"] == "ALG-1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ALG-1 not found in listPatientRecords: %v", list)
	}
}

func TestDoctorWriteKinds_Allergies_InvalidPayload_400(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "allergies_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/allergies", `{not json`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s, want 400", rr.Code, rr.Body.String())
	}
}

func TestDoctorWriteKinds_Allergies_ReadAfterWrite(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "allergies_read")
	approveConsent(t, "PAT-1", "DOC-1", "allergies_write")

	rr := httptest.NewRecorder()
	r := reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/allergies", `{"id":"ALG-1","allergen":"пенициллин"}`)
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusOK {
		t.Fatalf("write status=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	r = reqDoctorWrite(http.MethodGet, "DOC-1",
		"/api/patients/PAT-1/allergies", "")
	handleDoctorPatientRecords(rr, r, "PAT-1", "allergies")
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"ALG-1"`) {
		t.Fatalf("read status=%d body=%s, want 200 с ALG-1", rr.Code, rr.Body.String())
	}
}

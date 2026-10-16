package main

// sync_test.go — B30.4: тесты GET /api/patients/{id}/sync.
//
// Стиль — 1:1 с consent_test.go: собираем *http.Request с ролью в контексте
// через withDoctorID, вызываем хендлер напрямую через handlePatientSync.
//
// Покрытие:
//   - happy      : doctor + approved consent + vitals -> 200, records непустой
//   - no consent : consent отсутствует -> 403 consent_required
//   - pending    : status='pending' -> 403 consent_required
//   - revoked    : status='revoked' -> 403 consent_required
//   - expired    : expires_at в прошлом -> 403 consent_expired
//   - wrong scope: scope != 'vitals_read' -> 403 consent_required
//   - non-doctor : нет doctor-контекста -> 403 doctor only
//   - bad limit  : ?limit=abc / 0 / 9999 -> 400 invalid limit
//   - limit ok   : ?limit=2 -> 200, records ровно 2
//   - method     : POST -> 405 method not allowed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------
// Хелперы
// ---------------------------------------------------------------

// reqSync — GET /api/patients/{id}/sync с ролью doctor.
func reqSync(doctorID, patientID, query string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodGet,
		"/api/patients/"+patientID+"/sync"+query, nil).WithContext(ctx)
}

// reqSyncNoDoctor — GET без doctor-контекста.
func reqSyncNoDoctor(patientID string) *http.Request {
	return httptest.NewRequest(http.MethodGet,
		"/api/patients/"+patientID+"/sync", nil)
}

// callSync — вызывает handlePatientSync напрямую и возвращает recorder.
func callSync(t *testing.T, r *http.Request, patientID string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handlePatientSync(w, r, patientID)
	return w
}

// setupSyncEnvNoConsent — doctor + patient без consent.
func setupSyncEnvNoConsent(t *testing.T) {
	t.Helper()
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")
}

// hasErr — есть ли подстрока want в теле ответа.
func hasErr(w *httptest.ResponseRecorder, want string) bool {
	return strings.Contains(w.Body.String(), want)
}

// ---------------------------------------------------------------
// Тесты
// ---------------------------------------------------------------

func TestSync_HappyPath(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "approved")

	for i, bp := range []string{"120/80", "130/85", "125/82"} {
		recorded := "2026-01-0" + string(rune('1'+i)) + " 10:00:00"
		if _, err := db.Exec(`INSERT INTO vitals_history
                        (patient_id, bp, temp, spo2, recorded) VALUES (?,?,?,?,?)`,
			patientID, bp, 36.6, 98, recorded); err != nil {
			t.Fatalf("seed vitals %d: %v", i, err)
		}
	}

	w := callSync(t, reqSync(doctorID, patientID, ""), patientID)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp struct {
		PatientID string         `json:"patientId"`
		ConsentID string         `json:"consentId"`
		Limit     int            `json:"limit"`
		Records   []VitalsRecord `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	if resp.PatientID != patientID {
		t.Errorf("patientId: want %s, got %s", patientID, resp.PatientID)
	}
	if resp.ConsentID == "" {
		t.Errorf("consentId пустой")
	}
	if resp.Limit != syncDefaultLimit {
		t.Errorf("limit: want %d, got %d", syncDefaultLimit, resp.Limit)
	}
	if len(resp.Records) != 3 {
		t.Errorf("records: want 3, got %d", len(resp.Records))
	}
	if len(resp.Records) > 0 && resp.Records[0].BP != "125/82" {
		t.Errorf("первый record: want 125/82, got %s", resp.Records[0].BP)
	}
}

func TestSync_NoConsent(t *testing.T) {
	setupSyncEnvNoConsent(t)
	w := callSync(t, reqSync("DOC-1", "PAT-1", ""), "PAT-1")
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_required") {
		t.Fatalf("want 403 consent_required, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_ConsentPending(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "pending")
	w := callSync(t, reqSync(doctorID, patientID, ""), patientID)
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_required") {
		t.Fatalf("want 403 consent_required, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_ConsentRevoked(t *testing.T) {
	_, doctorID, patientID, consentID := setupConsentEnv(t, "approved")
	if err := transitionConsent(db, consentID, "approved", "revoked"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	w := callSync(t, reqSync(doctorID, patientID, ""), patientID)
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_required") {
		t.Fatalf("want 403 consent_required, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_ConsentExpired(t *testing.T) {
	_, doctorID, patientID, consentID := setupConsentEnv(t, "approved")
	if _, err := db.Exec(
		`UPDATE consents SET expires_at = ? WHERE id = ?`,
		"2000-01-01 00:00:00", consentID,
	); err != nil {
		t.Fatalf("set expires_at: %v", err)
	}
	w := callSync(t, reqSync(doctorID, patientID, ""), patientID)
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_expired") {
		t.Fatalf("want 403 consent_expired, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_WrongScope(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")
	if _, err := db.Exec(`INSERT INTO consents
                (id, patient_id, doctor_id, scope, status, requested_at, approved_at)
                VALUES ('CON-OTHER','PAT-1','DOC-1','other_scope','approved',
                        '2026-01-01 00:00:00','2026-01-01 00:00:00')`); err != nil {
		t.Fatalf("seed wrong-scope consent: %v", err)
	}
	w := callSync(t, reqSync("DOC-1", "PAT-1", ""), "PAT-1")
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_required") {
		t.Fatalf("want 403 consent_required, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_NonDoctor(t *testing.T) {
	setupConsentEnv(t, "approved")
	w := callSync(t, reqSyncNoDoctor("PAT-1"), "PAT-1")
	if w.Code != http.StatusForbidden || !hasErr(w, "doctor only") {
		t.Fatalf("want 403 doctor only, got %d %s", w.Code, w.Body.String())
	}
}

func TestSync_BadLimit(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "approved")
	for _, q := range []string{"?limit=abc", "?limit=0", "?limit=-5", "?limit=9999"} {
		w := callSync(t, reqSync(doctorID, patientID, q), patientID)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d body=%s", q, w.Code, w.Body.String())
		}
	}
}

func TestSync_LimitApplied(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "approved")
	for i := 0; i < 5; i++ {
		recorded := "2026-02-0" + string(rune('1'+i)) + " 10:00:00"
		if _, err := db.Exec(`INSERT INTO vitals_history
                        (patient_id, bp, temp, spo2, recorded) VALUES (?,?,?,?,?)`,
			patientID, "120/80", 36.6, 98, recorded); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	w := callSync(t, reqSync(doctorID, patientID, "?limit=2"), patientID)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Limit   int            `json:"limit"`
		Records []VitalsRecord `json:"records"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Limit != 2 {
		t.Errorf("limit: want 2, got %d", resp.Limit)
	}
	if len(resp.Records) != 2 {
		t.Errorf("records: want 2, got %d", len(resp.Records))
	}
}

func TestSync_MethodNotAllowed(t *testing.T) {
	_, doctorID, patientID, _ := setupConsentEnv(t, "approved")
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	r := httptest.NewRequest(http.MethodPost,
		"/api/patients/"+patientID+"/sync", nil).WithContext(ctx)
	w := callSync(t, r, patientID)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d body=%s", w.Code, w.Body.String())
	}
}

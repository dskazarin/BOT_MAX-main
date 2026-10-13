package main

// encounters_http_test.go — B29.1.6: HTTP-тесты handleEncountersList.
//
// Переиспользует e2eSetup/e2eLogin/e2eDo из e2e_test.go (общий db, currentEnv,
// http.DefaultServeMux). Дополнительно:
//   - одноразово регистрирует /api/encounters через registerEncounterRoutes;
//   - проставляет clinic_id для ADMIN-001 (для RBAC-тестов "чужая клиника").
//
// Данные сеются напрямую через createPatient/createEncounter.

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	encHTTPOnce sync.Once
	encHTTPEnv  bool
)

// encHTTPSetup — однократно: routes + clinic_id для ADMIN-001.
func encHTTPSetup(t *testing.T) *httptest.Server {
	t.Helper()
	srv := e2eSetup(t) // гарантирует db, currentEnv, seed

	encHTTPOnce.Do(func() {
		// Клиника для admin и "чужая" клиника — для RBAC.
		if _, err := db.Exec(`INSERT OR IGNORE INTO clinics (id, name, created) VALUES ('CL-1', 'Clinic One', datetime('now'))`); err != nil {
			t.Fatalf("insert CL-1: %v", err)
		}
		if _, err := db.Exec(`INSERT OR IGNORE INTO clinics (id, name, created) VALUES ('CL-2', 'Clinic Two', datetime('now'))`); err != nil {
			t.Fatalf("insert CL-2: %v", err)
		}
		if _, err := db.Exec(`UPDATE doctors SET clinic_id = 'CL-1' WHERE id = 'ADMIN-001'`); err != nil {
			t.Fatalf("set ADMIN-001 clinic: %v", err)
		}
		registerEncounterRoutes()
		encHTTPEnv = true
	})
	if !encHTTPEnv {
		t.Fatalf("encHTTPSetup: env не установлен")
	}
	return srv
}

// encSeedPatient — пациент с нужным doctor_id и clinic_id.
func encSeedPatient(t *testing.T, patientID, doctorID, clinicID string) {
	t.Helper()
	var cid any = nil
	if clinicID != "" {
		cid = clinicID
	}
	_, err := db.Exec(
		`INSERT OR REPLACE INTO patients (id, doctor_id, clinic_id, name, gender, created)
		 VALUES (?, ?, ?, 'HTTP Patient', 'male', datetime('now'))`,
		patientID, doctorID, cid,
	)
	if err != nil {
		t.Fatalf("insert patient %s: %v", patientID, err)
	}
}

// encSeedList — N приёмов c детерминированными started_at/kind/status.
func encSeedList(t *testing.T, patientID, doctorID, clinicID string, n int) {
	t.Helper()
	kinds := []string{"consultation", "study", "procedure", "lab", "followup"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		in := CreateEncounterInput{
			PatientID: patientID,
			DoctorID:  doctorID,
			ClinicID:  clinicID,
			Kind:      kinds[i%len(kinds)],
			StartedAt: ts,
			Title:     "http-enc",
		}
		enc, err := createEncounter(db, in)
		if err != nil {
			t.Fatalf("createEncounter %d: %v", i, err)
		}
		if i%2 == 0 {
			if err := finishEncounter(db, enc.ID); err != nil {
				t.Fatalf("finish %d: %v", i, err)
			}
		}
	}
}

// encHTTPGet — GET с cookie, возвращает (status, body).
func encHTTPGet(t *testing.T, srv *httptest.Server, path, accessCookie string) (int, string) {
	t.Helper()
	status, _, body := e2eDo(t, srv, e2eReq{
		method: http.MethodGet, url: path, cookie: accessCookie,
	})
	return status, body
}

// --- 1. 400: невалидный kind ---
func TestEncountersHTTP_BadKind(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, body := encHTTPGet(t, srv, "/api/encounters?patient_id=P1&kind=xyz", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", status, body)
	}
	if !strings.Contains(body, "invalid kind") {
		t.Errorf("body missing 'invalid kind': %s", body)
	}
}

// --- 2. 400: невалидный status ---
func TestEncountersHTTP_BadStatus(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, body := encHTTPGet(t, srv, "/api/encounters?patient_id=P1&status=open", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", status, body)
	}
	if !strings.Contains(body, "invalid status") {
		t.Errorf("body missing 'invalid status': %s", body)
	}
}

// --- 3. 400: from > to ---
func TestEncountersHTTP_FromAfterTo(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, body := encHTTPGet(t, srv,
		"/api/encounters?patient_id=P1&from=2026-06-01T00:00:00Z&to=2026-01-01T00:00:00Z", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", status, body)
	}
	// json.NewEncoder экранирует '>' в '\u003e', поэтому проверяем после Unmarshal.
	var errResp map[string]string
	if err := json.Unmarshal([]byte(body), &errResp); err != nil {
		t.Fatalf("json: %v, body=%s", err, body)
	}
	if errResp["error"] != "from > to" {
		t.Errorf("error=%q, want %q", errResp["error"], "from > to")
	}
}

// --- 4. 400: limit > 200 ---
func TestEncountersHTTP_LimitTooBig(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, _ := encHTTPGet(t, srv, "/api/encounters?patient_id=P1&limit=201", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
}

// --- 5. 400: offset > 10000 ---
func TestEncountersHTTP_OffsetTooBig(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, _ := encHTTPGet(t, srv, "/api/encounters?patient_id=P1&offset=10001", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
}

// --- 6. 400: нет ни одного скоупа ---
func TestEncountersHTTP_NoScope(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, _ := encHTTPGet(t, srv, "/api/encounters", access)
	if status != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", status)
	}
}

// --- 7. 200: doctor видит свои encounter'ы, total есть ---
func TestEncountersHTTP_DoctorPatientList(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	encSeedPatient(t, "P-HTTP-1", "DOC-001", "")
	encSeedList(t, "P-HTTP-1", "DOC-001", "", 5)

	status, body := encHTTPGet(t, srv, "/api/encounters?patient_id=P-HTTP-1", access)
	if status != http.StatusOK {
		t.Fatalf("status=%d, body=%s", status, body)
	}
	var resp struct {
		Items  []map[string]any `json:"items"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
		Count  int              `json:"count"`
		Total  int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("json: %v, body=%s", err, body)
	}
	if resp.Count != 5 || resp.Total != 5 {
		t.Errorf("count=%d total=%d, want 5/5, body=%s", resp.Count, resp.Total, body)
	}
	if resp.Limit != encounterListLimitDefault {
		t.Errorf("limit=%d, want %d", resp.Limit, encounterListLimitDefault)
	}
}

// --- 8. 200: no_total=1 → нет ключа total ---
func TestEncountersHTTP_NoTotal(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	encSeedPatient(t, "P-HTTP-2", "DOC-001", "")
	encSeedList(t, "P-HTTP-2", "DOC-001", "", 3)

	status, body := encHTTPGet(t, srv, "/api/encounters?patient_id=P-HTTP-2&no_total=1", access)
	if status != http.StatusOK {
		t.Fatalf("status=%d, body=%s", status, body)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("json: %v", err)
	}
	if _, ok := m["total"]; ok {
		t.Errorf("total shouldn't be present when no_total=1: %s", body)
	}
	if _, ok := m["items"]; !ok {
		t.Errorf("items missing: %s", body)
	}
}

// --- 9. 200: next_cursor присутствует на неполной странице ---
func TestEncountersHTTP_NextCursorPresent(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	encSeedPatient(t, "P-HTTP-3", "DOC-001", "")
	encSeedList(t, "P-HTTP-3", "DOC-001", "", 6)

	status, body := encHTTPGet(t, srv, "/api/encounters?patient_id=P-HTTP-3&limit=3", access)
	if status != http.StatusOK {
		t.Fatalf("status=%d, body=%s", status, body)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(body), &m)
	if _, ok := m["next_cursor"]; !ok {
		t.Errorf("next_cursor missing on non-last page: %s", body)
	}
	if m["count"].(float64) != 3 {
		t.Errorf("count=%v, want 3", m["count"])
	}
}

// --- 10. 403: doctor запрашивает чужой doctor_id ---
func TestEncountersHTTP_DoctorForeignDoctorID(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "doctor@local", "doctor@local")

	status, body := encHTTPGet(t, srv, "/api/encounters?doctor_id=ADMIN-001", access)
	if status != http.StatusForbidden {
		t.Fatalf("status=%d, want 403, body=%s", status, body)
	}
}

// --- 11. 403: admin запрашивает чужую клинику ---
func TestEncountersHTTP_AdminForeignClinic(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	status, body := encHTTPGet(t, srv, "/api/encounters?clinic_id=CL-2", access)
	if status != http.StatusForbidden {
		t.Fatalf("status=%d, want 403, body=%s", status, body)
	}
}

// --- 12. 200: admin видит свою клинику ---
func TestEncountersHTTP_AdminOwnClinic(t *testing.T) {
	srv := encHTTPSetup(t)
	access, _, _ := e2eLogin(t, srv, "admin@botmax.local", "admin")

	// Пациент и encounter в CL-1.
	encSeedPatient(t, "P-HTTP-4", "DOC-001", "CL-1")
	encSeedList(t, "P-HTTP-4", "DOC-001", "CL-1", 4)

	status, body := encHTTPGet(t, srv, "/api/encounters?clinic_id=CL-1", access)
	if status != http.StatusOK {
		t.Fatalf("status=%d, body=%s", status, body)
	}
	var resp struct {
		Count int `json:"count"`
	}
	_ = json.Unmarshal([]byte(body), &resp)
	if resp.Count != 4 {
		t.Errorf("count=%d, want 4, body=%s", resp.Count, body)
	}
}

// --- Санити: клиника CL-1, CL-2 существуют ---
func TestEncountersHTTP_ClinicsExist(t *testing.T) {
	_ = encHTTPSetup(t)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM clinics WHERE id IN ('CL-1','CL-2')`).Scan(&n); err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 2 {
		t.Fatalf("clinics n=%d, want 2", n)
	}
	_ = sql.ErrNoRows // импорт used
}

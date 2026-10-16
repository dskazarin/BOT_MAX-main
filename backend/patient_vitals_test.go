package main

// patient_vitals_test.go — B30.5: тесты self-vitals.
//
// Стиль — consent_test.go: httptest + WithContext(withPatientID(accID)),
// пакетная db подменяется setupPatientVitalsDB.
//
// ВАЖНО: withPatientID кладёт в ctx patientAccountID (patient_accounts.id),
// не patients.id. Резолв accID → pid делает resolvePatientIDForSelf.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// setupPatientVitalsDB — подмена пакетной db на свежую :memory:.
func setupPatientVitalsDB(t *testing.T) {
	t.Helper()
	saved := db
	db = newTestDB(t)
	t.Cleanup(func() { db = saved })
}

// seedPatientWithAccount — patients + patient_accounts (FK-порядок).
// Схема patients: NOT NULL только id/name/created (B30.2b → doctor_id nullable).
func seedPatientWithAccount(t *testing.T, patientID, accountID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO patients (id, name, created) VALUES (?, ?, datetime('now'))`,
		patientID, "Test Patient",
	)
	if err != nil {
		t.Fatalf("insert patient %s: %v", patientID, err)
	}
	_, err = db.Exec(
		`INSERT INTO patient_accounts (id, patient_id, login, password_hash, status, created, updated)
		 VALUES (?, ?, ?, 'x', 'active', datetime('now'), datetime('now'))`,
		accountID, patientID, accountID+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert patient_account %s: %v", accountID, err)
	}
}

// reqPatientVitals — GET/PUT с patientAccountID в контексте.
func reqPatientVitals(accountID, method, body string) *http.Request {
	ctx := withPatientID(context.Background(), accountID)
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, "/api/patient/vitals", nil)
	} else {
		r = httptest.NewRequest(method, "/api/patient/vitals", strings.NewReader(body))
	}
	return r.WithContext(ctx)
}

// reqPatientVitalsHistory — GET history с limit в query.
func reqPatientVitalsHistory(accountID, query string) *http.Request {
	ctx := withPatientID(context.Background(), accountID)
	url := "/api/patient/vitals/history"
	if query != "" {
		url += "?" + query
	}
	return httptest.NewRequest(http.MethodGet, url, nil).WithContext(ctx)
}

// ---------------------------------------------------------------

// GET без записи → 200 пустой объект.
func TestPatientVitals_GetEmpty(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	rec := httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodGet, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, ожидали 200, body=%s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["bp"] != "" || got["spo2"].(float64) != 0 {
		t.Fatalf("ожидали пустой объект, got=%v", got)
	}
}

// PUT → GET: значения сохранились.
func TestPatientVitals_PutGet(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	body := `{"bp":"120/80","temp":36.6,"spo2":98}`
	rec := httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodPut, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodGet, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status %d", rec.Code)
	}
	var got map[string]any
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got["bp"] != "120/80" || got["spo2"].(float64) != 98 {
		t.Fatalf("не сохранилось: %v", got)
	}
}

// PUT дважды → history 2 записи, свежие сверху.
func TestPatientVitals_History(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	for _, b := range []string{
		`{"bp":"110/70","temp":36.5,"spo2":97}`,
		`{"bp":"120/80","temp":36.6,"spo2":98}`,
	} {
		rec := httptest.NewRecorder()
		handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodPut, b))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT status %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handlePatientVitalsHistory(rec, reqPatientVitalsHistory("ACC1", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("history status %d", rec.Code)
	}
	var records []VitalsRecord
	if err := json.Unmarshal(rec.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("ожидали 2 записи, got=%d", len(records))
	}
	if records[0].BP != "120/80" {
		t.Fatalf("свежая сверху ожидалась 120/80, got=%s", records[0].BP)
	}
}

// limit=1 → одна запись.
func TestPatientVitals_HistoryLimit(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	for _, b := range []string{
		`{"bp":"110/70","temp":36.5,"spo2":97}`,
		`{"bp":"120/80","temp":36.6,"spo2":98}`,
	} {
		rec := httptest.NewRecorder()
		handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodPut, b))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT status %d", rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	handlePatientVitalsHistory(rec, reqPatientVitalsHistory("ACC1", "limit=1"))
	var records []VitalsRecord
	json.Unmarshal(rec.Body.Bytes(), &records)
	if len(records) != 1 {
		t.Fatalf("limit=1 ожидали 1, got=%d", len(records))
	}
}

// Чужой/несуществующий accID → 404.
func TestPatientVitals_AccountNotFound(t *testing.T) {
	setupPatientVitalsDB(t)

	rec := httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("NOPE", http.MethodGet, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, ожидали 404", rec.Code)
	}
}

// Нет accID в ctx → 404.
func TestPatientVitals_NoCtx(t *testing.T) {
	setupPatientVitalsDB(t)

	r := httptest.NewRequest(http.MethodGet, "/api/patient/vitals", nil)
	rec := httptest.NewRecorder()
	handlePatientVitals(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, ожидали 404", rec.Code)
	}
}

// PUT невалидный JSON → 400.
func TestPatientVitals_PutBadJSON(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	rec := httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodPut, "{not-json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, ожидали 400", rec.Code)
	}
}

// DELETE → 405.
func TestPatientVitals_MethodNotAllowed(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "P1", "ACC1")

	rec := httptest.NewRecorder()
	handlePatientVitals(rec, reqPatientVitals("ACC1", http.MethodDelete, ""))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d, ожидали 405", rec.Code)
	}
}

// Middleware: запрос без cookie botmax_pat_access → 401.
func TestPatientVitalsMiddleware_NoToken(t *testing.T) {
	setupPatientVitalsDB(t)

	r := httptest.NewRequest(http.MethodGet, "/api/patient/vitals", nil)
	rec := httptest.NewRecorder()
	patientAuthMiddleware(http.HandlerFunc(handlePatientVitals)).ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, ожидали 401, body=%s", rec.Code, rec.Body.String())
	}
}

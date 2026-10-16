package main

// patient_auth_test.go — B30.3: регрессия на nullable-поля в Scan.
//
// Покрывает три места, исправленных в B30.3:
//   - getVitals        : vitals (bp/temp/spo2/updated — все nullable)
//   - getVitalsHistory : vitals_history (bp/temp/spo2/recorded — все nullable)
//   - getCard          : cards (data/updated — оба nullable)
//
// Плюс — базовое покрытие getPatientAccountByLogin/ByID (LastLogin — sql.NullString,
// это штатный паттерн, отдельной регрессии не требует, но пусть будет smoke).

import (
	"database/sql"
	"errors"
	"testing"
)

// seedPatient — минимальный patient (id, name, created).
func seedPatient(t *testing.T, db *sql.DB, id, name string) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO patients (id, name, created) VALUES (?, ?, ?)`,
		id, name, "2026-01-01T00:00:00Z",
	); err != nil {
		t.Fatalf("seedPatient(%s): %v", id, err)
	}
}

// --- vitals ---

func TestGetVitals_NullFields(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-V", "V")

	// Все клинические поля NULL — именно тот случай, ради которого B30.3.
	if _, err := db.Exec(
		`INSERT INTO vitals (patient_id, bp, temp, spo2, updated)
		 VALUES ('PAT-V', NULL, NULL, NULL, NULL)`,
	); err != nil {
		t.Fatalf("insert vitals: %v", err)
	}

	bp, temp, spo2, updated, err := getVitals(db, "PAT-V")
	if err != nil {
		t.Fatalf("getVitals (регрессия B30.3): %v", err)
	}
	if bp != "" || temp != 0 || spo2 != 0 || updated != "" {
		t.Errorf("want zero-values, got bp=%q temp=%v spo2=%v updated=%q",
			bp, temp, spo2, updated)
	}
}

func TestGetVitals_NoRows(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-V0", "V0")

	_, _, _, _, err := getVitals(db, "PAT-V0")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

// --- vitals_history ---

func TestGetVitalsHistory_NullFields(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-H", "H")

	if _, err := db.Exec(
		`INSERT INTO vitals_history (patient_id, bp, temp, spo2, recorded)
		 VALUES ('PAT-H', NULL, NULL, NULL, NULL)`,
	); err != nil {
		t.Fatalf("insert vitals_history: %v", err)
	}

	recs, err := getVitalsHistory(db, "PAT-H", 10)
	if err != nil {
		t.Fatalf("getVitalsHistory (регрессия B30.3): %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("len(recs) = %d, want 1", len(recs))
	}
	r := recs[0]
	if r.BP != "" || r.Temp != 0 || r.Spo2 != 0 || r.Recorded != "" {
		t.Errorf("want zero-values, got BP=%q Temp=%v Spo2=%v Recorded=%q",
			r.BP, r.Temp, r.Spo2, r.Recorded)
	}
}

func TestGetVitalsHistory_Empty(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-H0", "H0")

	recs, err := getVitalsHistory(db, "PAT-H0", 10)
	if err != nil {
		t.Fatalf("getVitalsHistory empty: %v", err)
	}
	if len(recs) != 0 {
		t.Errorf("len(recs) = %d, want 0 (не nil)", len(recs))
	}
}

// --- cards ---

func TestGetCard_NullFields(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-C", "C")

	if _, err := db.Exec(
		`INSERT INTO cards (patient_id, data, updated) VALUES ('PAT-C', NULL, NULL)`,
	); err != nil {
		t.Fatalf("insert cards: %v", err)
	}

	data, updated, err := getCard(db, "PAT-C")
	if err != nil {
		t.Fatalf("getCard (регрессия B30.3): %v", err)
	}
	if data != "" || updated != "" {
		t.Errorf("want empty, got data=%q updated=%q", data, updated)
	}
}

func TestGetCard_NoRows(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-C0", "C0")

	_, _, err := getCard(db, "PAT-C0")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("err = %v, want sql.ErrNoRows", err)
	}
}

// --- patient_accounts (smoke, LastLogin — sql.NullString) ---

func TestGetPatientAccountByLogin_NullLastLogin(t *testing.T) {
	db := newFileTestDB(t)
	seedPatient(t, db, "PAT-A", "Alice")

	if _, err := db.Exec(
		`INSERT INTO patient_accounts
		   (id, patient_id, login, password_hash, status, created, updated, last_login)
		 VALUES ('PA-1','PAT-A','alice','h','active','2026-01-01T00:00:00Z','2026-01-01T00:00:00Z', NULL)`,
	); err != nil {
		t.Fatalf("insert patient_accounts: %v", err)
	}

	a, err := getPatientAccountByLogin(db, "alice")
	if err != nil {
		t.Fatalf("getPatientAccountByLogin: %v", err)
	}
	if a.ID != "PA-1" || a.PatientName != "Alice" {
		t.Errorf("got ID=%q PatientName=%q", a.ID, a.PatientName)
	}
	if a.LastLogin.Valid {
		t.Errorf("LastLogin.Valid = true, want false (NULL)")
	}
}

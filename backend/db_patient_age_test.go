package main

// db_patient_age_test.go — B30.8c: нормализация ageDisplay при чтении.
//
// Старые записи patients.age_display хранят произвольный формат («35 лет»),
// patient_profile.go пишет «N г M мес». normalizeAgeDisplay в db.go
// пересчитывает Age/AgeMonths/AgeDisplay из birth_date при чтении —
// врач в карточке видит единый формат.

import (
	"testing"
	"time"
)

// seedPatientWithAgeDisplay — пациент с заданными birth_date и age_display
// в обход createPatient (чтобы положить «старый» формат напрямую в БД).
func seedPatientWithAgeDisplay(t *testing.T, id, birthDate, ageDisplay string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO patients (id, name, birth_date, age_display, created)
		 VALUES (?, ?, ?, ?, datetime('now'))`,
		id, "Age Test", birthDate, ageDisplay,
	)
	if err != nil {
		t.Fatalf("insert patient %s: %v", id, err)
	}
}

// Со старым age_display + birth_date → пересчитано в формат formatAgeDisplay.
func TestGetPatientByID_NormalizesAgeDisplay(t *testing.T) {
	setupPatientProfileDB(t) // создаёт db = newTestDB(t)
	seedPatientWithAgeDisplay(t, "PAT-AGE1", "1990-01-01", "35 лет")

	p, err := getPatientByID(db, "PAT-AGE1")
	if err != nil {
		t.Fatalf("getPatientByID: %v", err)
	}

	wantAge, wantMonths, wantDisplay, err := calcAgeFromBirthDate("1990-01-01", time.Now().UTC())
	if err != nil {
		t.Fatalf("calcAgeFromBirthDate: %v", err)
	}
	if p.AgeDisplay != wantDisplay {
		t.Errorf("AgeDisplay = %q, want %q (старый формат не должен просачиваться)", p.AgeDisplay, wantDisplay)
	}
	if p.Age != wantAge || p.AgeMonths != wantMonths {
		t.Errorf("Age/AgeMonths = %d/%d, want %d/%d", p.Age, p.AgeMonths, wantAge, wantMonths)
	}
	if p.AgeDisplay == "35 лет" {
		t.Errorf("AgeDisplay остался старым %q — normalizeAgeDisplay не сработал", p.AgeDisplay)
	}
}

// Без birth_date → age_display остаётся как в БД (пересчитывать не из чего).
func TestGetPatientByID_NoBirthDateKeepsDisplay(t *testing.T) {
	setupPatientProfileDB(t)
	seedPatientWithAgeDisplay(t, "PAT-AGE2", "", "35 лет")

	p, err := getPatientByID(db, "PAT-AGE2")
	if err != nil {
		t.Fatalf("getPatientByID: %v", err)
	}
	if p.AgeDisplay != "35 лет" {
		t.Errorf("AgeDisplay = %q, want %q (без birth_date не пересчитываем)", p.AgeDisplay, "35 лет")
	}
}

// getPatientsByDoctor нормализует так же, как getPatientByID.
func TestGetPatientsByDoctor_NormalizesAgeDisplay(t *testing.T) {
	setupPatientProfileDB(t)
	// doctor_id — FK на doctors.id, поэтому сначала сид доктора.
	if _, err := db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
		 VALUES ('DOC-AGE', NULL, 'doc-age@test.local', 'x', 'Test', 'lor', 'doctor', datetime('now'), 1)`,
	); err != nil {
		t.Fatalf("insert doctor: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO patients (id, doctor_id, name, birth_date, age_display, created)
		 VALUES ('PAT-AGE3', 'DOC-AGE', 'Age Test', '1990-01-01', '35 лет', datetime('now'))`,
	); err != nil {
		t.Fatalf("insert patient: %v", err)
	}

	list, err := getPatientsByDoctor(db, "DOC-AGE")
	if err != nil {
		t.Fatalf("getPatientsByDoctor: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}

	_, _, wantDisplay, err := calcAgeFromBirthDate("1990-01-01", time.Now().UTC())
	if err != nil {
		t.Fatalf("calcAgeFromBirthDate: %v", err)
	}
	if list[0].AgeDisplay != wantDisplay {
		t.Errorf("AgeDisplay = %q, want %q", list[0].AgeDisplay, wantDisplay)
	}
}

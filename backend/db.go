package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// InitDB открывает SQLite-базу и создаёт схему, если её нет.
// Безопасно вызывать при каждом старте сервера (CREATE TABLE IF NOT EXISTS).
func InitDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	// PRAGMA применяются на каждое соединение — выполняем сразу.
	pragmas := []string{
		"PRAGMA foreign_keys = ON;",
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous = NORMAL;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}

	schema := []string{
		// === Клиники (закрытая регистрация, флаг active) ===
		`CREATE TABLE IF NOT EXISTS clinics (
			id       TEXT PRIMARY KEY,
			name     TEXT NOT NULL,
			address  TEXT,
			phone    TEXT,
			email    TEXT,
			created  TEXT NOT NULL,
			active   INTEGER DEFAULT 1
		);`,

		// === Врачи ===
		`CREATE TABLE IF NOT EXISTS doctors (
			id            TEXT PRIMARY KEY,
			clinic_id     TEXT,
			email         TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			name          TEXT NOT NULL,
			specialty     TEXT DEFAULT 'lor',
			role          TEXT NOT NULL DEFAULT 'doctor',
			phone         TEXT,
			created       TEXT NOT NULL,
			last_login    TEXT,
			active        INTEGER DEFAULT 1,
			FOREIGN KEY(clinic_id) REFERENCES clinics(id) ON DELETE SET NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_doctors_clinic ON doctors(clinic_id);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_doctors_email ON doctors(email);`,

		// === Пациенты (владелец — врач, клиника опционально) ===
		`CREATE TABLE IF NOT EXISTS patients (
			id          TEXT PRIMARY KEY,
			doctor_id   TEXT NOT NULL,
			clinic_id   TEXT,
			name        TEXT NOT NULL,
			age         INTEGER,
			age_months  INTEGER,
			age_display TEXT,
			birth_date  TEXT,
			gender      TEXT,
			height      REAL,
			weight      REAL,
			phone       TEXT,
			email       TEXT,
			status      TEXT,
			diagnosis   TEXT,
			symptoms    TEXT,
			ai_checked  INTEGER DEFAULT 0,
			ai_result   TEXT,
			created     TEXT NOT NULL,
			last_sync   TEXT,
			FOREIGN KEY(doctor_id) REFERENCES doctors(id) ON DELETE CASCADE,
			FOREIGN KEY(clinic_id) REFERENCES clinics(id) ON DELETE SET NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_patients_doctor ON patients(doctor_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_patients_clinic ON patients(clinic_id, status);`,

		// === Витальные (текущие) ===
		`CREATE TABLE IF NOT EXISTS vitals (
			patient_id TEXT PRIMARY KEY,
			bp         TEXT,
			temp       REAL,
			spo2       INTEGER,
			updated    TEXT,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE
		);`,

		// === История витальных ===
		`CREATE TABLE IF NOT EXISTS vitals_history (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			patient_id TEXT NOT NULL,
			bp         TEXT,
			temp       REAL,
			spo2       INTEGER,
			recorded   TEXT,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_vitals_history_patient ON vitals_history(patient_id, recorded);`,
		`CREATE INDEX IF NOT EXISTS idx_vitals_history_recorded ON vitals_history(recorded);`,

		// === Карточка (JSON) ===
		`CREATE TABLE IF NOT EXISTS cards (
			patient_id TEXT PRIMARY KEY,
			data       TEXT,
			updated    TEXT,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE
		);`,

		// === Приглашения в клинику (задел на шаг 6+) ===
		`CREATE TABLE IF NOT EXISTS invites (
			token     TEXT PRIMARY KEY,
			clinic_id TEXT NOT NULL,
			email     TEXT,
			role      TEXT DEFAULT 'doctor',
			created   TEXT NOT NULL,
			expires   TEXT NOT NULL,
			used      INTEGER DEFAULT 0,
			FOREIGN KEY(clinic_id) REFERENCES clinics(id) ON DELETE CASCADE
		);`,
	}

	for _, stmt := range schema {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			head := stmt
			if len(head) > 60 {
				head = head[:60]
			}
			return nil, fmt.Errorf("schema %q: %w", head, err)
		}
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

// seedDefaultDoctor вставляет тестового врача DOC-001 (частник, без клиники),
// если таблица doctors пуста. Нужен, чтобы шаги 2–5 (CRUD) имели валидного
// владельца пациентов до появления реальной авторизации (шаг 6).
func seedDefaultDoctor(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM doctors`).Scan(&count); err != nil {
		return fmt.Errorf("count doctors: %w", err)
	}
	if count > 0 {
		return nil
	}
	_, err := db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
		 VALUES (?, NULL, ?, ?, ?, 'lor', 'doctor', datetime('now'), 1)`,
		"DOC-001",
		"doctor@local",
		"!disabled",
		"Тестовый врач",
	)
	if err != nil {
		return fmt.Errorf("seed doctor: %w", err)
	}
	fmt.Println("🌱 Создан тестовый врач DOC-001 (doctor@local)")
	return nil
}

// getPatientsByDoctor возвращает всех пациентов врача, отсортированных по created DESC.
func getPatientsByDoctor(db *sql.DB, doctorID string) ([]Patient, error) {
	rows, err := db.Query(`
		SELECT id, doctor_id, COALESCE(clinic_id, ''), name, COALESCE(age, 0),
		       COALESCE(age_months, 0), COALESCE(age_display, ''), COALESCE(birth_date, ''),
		       COALESCE(gender, ''), COALESCE(height, 0), COALESCE(weight, 0),
		       COALESCE(phone, ''), COALESCE(email, ''), COALESCE(status, ''),
		       COALESCE(diagnosis, ''), COALESCE(symptoms, ''), COALESCE(ai_checked, 0),
		       COALESCE(ai_result, ''), COALESCE(created, ''), COALESCE(last_sync, '')
		FROM patients
		WHERE doctor_id = ?
		ORDER BY created DESC`, doctorID)
	if err != nil {
		return nil, fmt.Errorf("query patients: %w", err)
	}
	defer rows.Close()

	patients := []Patient{}
	for rows.Next() {
		var p Patient
		var aiChecked int
		var symptoms, aiResult string
		if err := rows.Scan(
			&p.ID, &p.DoctorID, &p.ClinicID, &p.Name, &p.Age,
			&p.AgeMonths, &p.AgeDisplay, &p.BirthDate,
			&p.Gender, &p.Height, &p.Weight,
			&p.Phone, &p.Email, &p.Status,
			&p.Diagnosis, &symptoms, &aiChecked, &aiResult,
			&p.Created, &p.LastSync,
		); err != nil {
			return nil, fmt.Errorf("scan patient: %w", err)
		}
		p.AIChecked = aiChecked == 1
		if symptoms != "" {
			p.Symptoms = json.RawMessage(symptoms)
		}
		if aiResult != "" {
			p.AIResult = json.RawMessage(aiResult)
		}
		patients = append(patients, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return patients, nil
}

// createPatient вставляет нового пациента в БД.
func createPatient(db *sql.DB, p *Patient) error {
	aiChecked := 0
	if p.AIChecked {
		aiChecked = 1
	}
	var symptoms, aiResult interface{}
	if len(p.Symptoms) > 0 {
		symptoms = string(p.Symptoms)
	}
	if len(p.AIResult) > 0 {
		aiResult = string(p.AIResult)
	}
	_, err := db.Exec(`
		INSERT INTO patients (
			id, doctor_id, clinic_id, name, age, age_months, age_display,
			birth_date, gender, height, weight, phone, email, status,
			diagnosis, symptoms, ai_checked, ai_result, created, last_sync
		) VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.DoctorID, p.ClinicID, p.Name, p.Age, p.AgeMonths, p.AgeDisplay,
		p.BirthDate, p.Gender, p.Height, p.Weight, p.Phone, p.Email, p.Status,
		p.Diagnosis, symptoms, aiChecked, aiResult, p.Created, p.LastSync)
	if err != nil {
		return fmt.Errorf("insert patient: %w", err)
	}
	return nil
}

// getPatientByID возвращает одного пациента по ID.
func getPatientByID(db *sql.DB, id string) (*Patient, error) {
	var p Patient
	var aiChecked int
	var symptoms, aiResult string
	err := db.QueryRow(`
		SELECT id, doctor_id, COALESCE(clinic_id, ''), name, COALESCE(age, 0),
		       COALESCE(age_months, 0), COALESCE(age_display, ''), COALESCE(birth_date, ''),
		       COALESCE(gender, ''), COALESCE(height, 0), COALESCE(weight, 0),
		       COALESCE(phone, ''), COALESCE(email, ''), COALESCE(status, ''),
		       COALESCE(diagnosis, ''), COALESCE(symptoms, ''), COALESCE(ai_checked, 0),
		       COALESCE(ai_result, ''), COALESCE(created, ''), COALESCE(last_sync, '')
		FROM patients WHERE id = ?`, id).Scan(
		&p.ID, &p.DoctorID, &p.ClinicID, &p.Name, &p.Age,
		&p.AgeMonths, &p.AgeDisplay, &p.BirthDate,
		&p.Gender, &p.Height, &p.Weight,
		&p.Phone, &p.Email, &p.Status,
		&p.Diagnosis, &symptoms, &aiChecked, &aiResult,
		&p.Created, &p.LastSync,
	)
	if err != nil {
		return nil, err
	}
	p.AIChecked = aiChecked == 1
	if symptoms != "" {
		p.Symptoms = json.RawMessage(symptoms)
	}
	if aiResult != "" {
		p.AIResult = json.RawMessage(aiResult)
	}
	return &p, nil
}

// updatePatient обновляет существующего пациента по id.
// Возвращает sql.ErrNoRows, если пациент не найден.
func updatePatient(db *sql.DB, p *Patient) error {
	aiChecked := 0
	if p.AIChecked {
		aiChecked = 1
	}
	var symptoms, aiResult interface{}
	if len(p.Symptoms) > 0 {
		symptoms = string(p.Symptoms)
	}
	if len(p.AIResult) > 0 {
		aiResult = string(p.AIResult)
	}
	res, err := db.Exec(`
		UPDATE patients SET
			clinic_id = NULLIF(?, ''),
			name = ?, age = ?, age_months = ?, age_display = ?,
			birth_date = ?, gender = ?, height = ?, weight = ?,
			phone = ?, email = ?, status = ?,
			diagnosis = ?, symptoms = ?, ai_checked = ?, ai_result = ?,
			last_sync = ?
		WHERE id = ?`,
		p.ClinicID, p.Name, p.Age, p.AgeMonths, p.AgeDisplay,
		p.BirthDate, p.Gender, p.Height, p.Weight,
		p.Phone, p.Email, p.Status,
		p.Diagnosis, symptoms, aiChecked, aiResult,
		p.LastSync, p.ID)
	if err != nil {
		return fmt.Errorf("update patient: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// deletePatient удаляет пациента по id.
// Возвращает sql.ErrNoRows, если пациент не найден.
func deletePatient(db *sql.DB, id string) error {
	res, err := db.Exec(`DELETE FROM patients WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete patient: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// === Vitals CRUD (Шаг 5, B1) ===

// VitalsRecord — запись истории витальных показателей.
type VitalsRecord struct {
	ID        int     `json:"id"`
	PatientID string  `json:"patientId"`
	BP        string  `json:"bp"`
	Temp      float64 `json:"temp"`
	Spo2      int     `json:"spo2"`
	Recorded  string  `json:"recorded"`
}

// getVitals возвращает текущие витальные пациента.
// Если записи нет — возвращает sql.ErrNoRows (фронт → fallback на localStorage).
func getVitals(db *sql.DB, patientID string) (bp string, temp float64, spo2 int, updated string, err error) {
	row := db.QueryRow(
		`SELECT bp, temp, spo2, updated FROM vitals WHERE patient_id = ?`,
		patientID,
	)
	err = row.Scan(&bp, &temp, &spo2, &updated)
	if err != nil {
		return "", 0, 0, "", err
	}
	return bp, temp, spo2, updated, nil
}

// upsertVitals вставляет или обновляет текущие витальные.
// Обновляет поле updated текущим временем (RFC3339, UTC).
func upsertVitals(db *sql.DB, patientID, bp string, temp float64, spo2 int) error {
	_, err := db.Exec(`
		INSERT INTO vitals (patient_id, bp, temp, spo2, updated)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(patient_id) DO UPDATE SET
			bp = excluded.bp,
			temp = excluded.temp,
			spo2 = excluded.spo2,
			updated = excluded.updated
	`, patientID, bp, temp, spo2, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert vitals: %w", err)
	}
	return nil
}

// appendVitalsHistory добавляет запись в историю витальных.
func appendVitalsHistory(db *sql.DB, patientID, bp string, temp float64, spo2 int) error {
	_, err := db.Exec(`
		INSERT INTO vitals_history (patient_id, bp, temp, spo2, recorded)
		VALUES (?, ?, ?, ?, ?)
	`, patientID, bp, temp, spo2, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("append vitals history: %w", err)
	}
	return nil
}

// getVitalsHistory возвращает историю витальных пациента.
// Свежие сверху (ORDER BY recorded DESC). limit<=0 → 50.
// Возвращает []VitalsRecord{} (не nil) при отсутствии записей.
func getVitalsHistory(db *sql.DB, patientID string, limit int) ([]VitalsRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := db.Query(`
		SELECT id, patient_id, bp, temp, spo2, recorded
		FROM vitals_history
		WHERE patient_id = ?
		ORDER BY recorded DESC
		LIMIT ?
	`, patientID, limit)
	if err != nil {
		return nil, fmt.Errorf("query vitals history: %w", err)
	}
	defer rows.Close()

	records := []VitalsRecord{}
	for rows.Next() {
		var r VitalsRecord
		if err := rows.Scan(&r.ID, &r.PatientID, &r.BP, &r.Temp, &r.Spo2, &r.Recorded); err != nil {
			return nil, fmt.Errorf("scan vitals history: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}
	return records, nil
}

// === Cards CRUD (Шаг 5, B2) ===

// getCard возвращает JSON-данные карточки пациента.
// Если записи нет — sql.ErrNoRows (фронт → localStorage).
func getCard(db *sql.DB, patientID string) (data string, updated string, err error) {
	row := db.QueryRow(
		`SELECT data, updated FROM cards WHERE patient_id = ?`,
		patientID,
	)
	err = row.Scan(&data, &updated)
	if err != nil {
		return "", "", err
	}
	return data, updated, nil
}

// upsertCard вставляет или обновляет данные карточки.
// data — сериализованный JSON (строка). updated — RFC3339 UTC.
func upsertCard(db *sql.DB, patientID, data string) error {
	_, err := db.Exec(`
		INSERT INTO cards (patient_id, data, updated)
		VALUES (?, ?, ?)
		ON CONFLICT(patient_id) DO UPDATE SET
			data = excluded.data,
			updated = excluded.updated
	`, patientID, data, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert card: %w", err)
	}
	return nil
}

package main

import (
	"database/sql"
	"fmt"

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

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

// InitDB открывает SQLite-базу и создаёт схему, если её нет.
// Безопасно вызывать при каждом старте сервера (CREATE TABLE IF NOT EXISTS).
func InitDB(path string) (*sql.DB, error) {
	// PRAGMA-параметры передаются через DSN: modernc.org/sqlite применяет их
	// к КАЖДОМУ новому соединению пула. Через db.Exec PRAGMA сработал бы
	// только на одном соединении из пула — остальные открылись бы
	// с дефолтными настройками (foreign_keys=OFF).
	const dsnParams = "?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=busy_timeout(5000)"

	db, err := sql.Open("sqlite", path+dsnParams)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
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
			doctor_id   TEXT,
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
			FOREIGN KEY(doctor_id) REFERENCES doctors(id) ON DELETE SET NULL,
			FOREIGN KEY(clinic_id) REFERENCES clinics(id) ON DELETE SET NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_patients_doctor ON patients(doctor_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_patients_clinic ON patients(clinic_id, status);`,

		// === B28.1: Encounters (эпизоды приёмов) ===
		`CREATE TABLE IF NOT EXISTS encounters (
			id              TEXT PRIMARY KEY,
			patient_id      TEXT NOT NULL,
			doctor_id       TEXT,
			clinic_id       TEXT,
			specialty       TEXT NOT NULL DEFAULT '',
			title           TEXT NOT NULL DEFAULT '',
			kind            TEXT NOT NULL DEFAULT 'consultation',
			started_at      TEXT NOT NULL,
			finished_at     TEXT,
			status          TEXT NOT NULL DEFAULT 'draft',
			complaint       TEXT NOT NULL DEFAULT '',
			anamnesis       TEXT NOT NULL DEFAULT '',
			examination     TEXT NOT NULL DEFAULT '',
			diagnosis_text  TEXT NOT NULL DEFAULT '',
			diagnosis_icd10 TEXT NOT NULL DEFAULT '',
			recommendations TEXT NOT NULL DEFAULT '',
			common_json     TEXT NOT NULL DEFAULT '{}',
			specialty_json  TEXT NOT NULL DEFAULT '{}',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE,
			FOREIGN KEY(doctor_id)  REFERENCES doctors(id)  ON DELETE SET NULL,
			FOREIGN KEY(clinic_id)  REFERENCES clinics(id)  ON DELETE SET NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_enc_patient ON encounters(patient_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_enc_doctor  ON encounters(doctor_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_enc_clinic  ON encounters(clinic_id, started_at DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_enc_patient_kind ON encounters(patient_id, kind, started_at DESC);`,

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

		// === Refresh-токены (B4.1) ===
		// === B27.1: password reset tokens ===
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			token_hash TEXT NOT NULL UNIQUE,
			doctor_id  TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			used_at    TEXT,
			FOREIGN KEY (doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_prt_doctor ON password_reset_tokens(doctor_id);`,
		`CREATE INDEX IF NOT EXISTS idx_prt_expires ON password_reset_tokens(expires_at);`,

		`CREATE TABLE IF NOT EXISTS refresh_tokens (
			jti         TEXT PRIMARY KEY,
			doctor_id   TEXT NOT NULL,
			issued_at   TEXT NOT NULL,
			expires_at  TEXT NOT NULL,
			revoked_at  TEXT,
			replaced_by TEXT,
			FOREIGN KEY(doctor_id) REFERENCES doctors(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_doctor ON refresh_tokens(doctor_id);`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_expires ON refresh_tokens(expires_at);`,

		// === Audit-log (B9.1) ===
		`CREATE TABLE IF NOT EXISTS audit_log (
                id         INTEGER PRIMARY KEY AUTOINCREMENT,
                ts         TEXT NOT NULL,
                doctor_id  TEXT,
                action     TEXT NOT NULL,
                entity     TEXT,
                entity_id  TEXT,
                ip         TEXT,
                detail     TEXT
        );`,
		`CREATE INDEX IF NOT EXISTS idx_audit_ts     ON audit_log(ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_doctor ON audit_log(doctor_id, ts DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action, ts DESC);`,

		// === B30.1: аккаунты пациентов (надстройка над patients) ===
		// Пациент — самостоятельный субъект с логином/паролем.
		// Один patients -> максимум один patient_accounts (UNIQUE).
		`CREATE TABLE IF NOT EXISTS patient_accounts (
			id            TEXT PRIMARY KEY,
			patient_id    TEXT NOT NULL UNIQUE,
			login         TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'active',
			created       TEXT NOT NULL,
			updated       TEXT NOT NULL,
			last_login    TEXT,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE
		);`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_patient_accounts_login   ON patient_accounts(login);`,
		`CREATE        INDEX IF NOT EXISTS idx_patient_accounts_patient ON patient_accounts(patient_id);`,

		// === B30.1: согласия patient <-> doctor ===
		// status: pending | approved | denied | revoked
		// scope:  vitals_read (расширяемо значениями TEXT в будущем)
		// UNIQUE(patient_id, doctor_id, scope) — одно активное согласие
		// на пару+scope; история переходов — в consents_audit.
		`CREATE TABLE IF NOT EXISTS consents (
			id           TEXT PRIMARY KEY,
			patient_id   TEXT NOT NULL,
			doctor_id    TEXT NOT NULL,
			scope        TEXT NOT NULL DEFAULT 'vitals_read',
			status       TEXT NOT NULL DEFAULT 'pending',
			requested_at TEXT NOT NULL,
			approved_at  TEXT,
			revoked_at   TEXT,
			expires_at   TEXT,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE,
			FOREIGN KEY(doctor_id)  REFERENCES doctors(id)  ON DELETE CASCADE,
			UNIQUE(patient_id, doctor_id, scope)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_consents_doctor_status  ON consents(doctor_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_consents_patient_status ON consents(patient_id, status);`,

		// === B30.1: append-only аудит согласий ===
		// Без FK на consents — запись должна пережить удаление consent.
		// action:     request | approve | deny | revoke
		// actor_type: patient | doctor | admin
		`CREATE TABLE IF NOT EXISTS consents_audit (
			id         TEXT PRIMARY KEY,
			consent_id TEXT NOT NULL,
			patient_id TEXT NOT NULL,
			doctor_id  TEXT NOT NULL,
			action     TEXT NOT NULL,
			actor_type TEXT NOT NULL,
			actor_id   TEXT,
			ip         TEXT,
			user_agent TEXT,
			created    TEXT NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_consents_audit_consent ON consents_audit(consent_id, created DESC);`,
		`CREATE INDEX IF NOT EXISTS idx_consents_audit_patient ON consents_audit(patient_id, created DESC);`,
		// === B30.2: refresh-токены пациентов (по образцу refresh_tokens) ===
		// FK на patient_accounts (не на patients) — токен принадлежит
		// аккаунту, не карте. Удалили аккаунт — токены не нужны.
		`CREATE TABLE IF NOT EXISTS patient_refresh_tokens (
			jti         TEXT PRIMARY KEY,
			patient_id  TEXT NOT NULL,
			issued_at   TEXT NOT NULL,
			expires_at  TEXT NOT NULL,
			revoked_at  TEXT,
			replaced_by TEXT,
			FOREIGN KEY(patient_id) REFERENCES patient_accounts(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_patient_refresh_patient ON patient_refresh_tokens(patient_id);`,
		`CREATE INDEX IF NOT EXISTS idx_patient_refresh_expires ON patient_refresh_tokens(expires_at);`,
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

	// === B27.1: идемпотентные миграции (ALTER TABLE ADD COLUMN) ===
	// SQLite не поддерживает "ADD COLUMN IF NOT EXISTS" — проверяем через
	// PRAGMA table_info и добавляем только отсутствующие.
	// ВАЖНО: выполняется ПОСЛЕ цикла schema (таблицы уже созданы).
	migrations := []struct{ table, column, ddl string }{
		{"doctors", "must_change_password", "INTEGER NOT NULL DEFAULT 0"},
		{"doctors", "password_changed_at", "TEXT"},
		{"doctors", "failed_login_count", "INTEGER NOT NULL DEFAULT 0"},
		{"doctors", "locked_until", "TEXT"},
		{"consents", "expires_at", "TEXT"},
	}
	for _, m := range migrations {
		if err := ensureColumn(db, m.table, m.column, m.ddl); err != nil {
			db.Close()
			return nil, fmt.Errorf("migrate %s.%s: %w", m.table, m.column, err)
		}
	}

	// === B30.2b: patients.doctor_id → NULLABLE + ON DELETE SET NULL ===
	// Пересоздание таблицы (SQLite не умеет ALTER COLUMN). Идемпотентно:
	// если doctor_id уже nullable — no-op. См. migratePatientsDoctorIdNullable.
	if err := migratePatientsDoctorIdNullable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate patients.doctor_id: %w", err)
	}

	// === B30.2c: encounters.doctor_id → NULLABLE + ON DELETE SET NULL ===
	// Тот же паттерн, что B30.2b (см. §6.61 / §6.62). Идемпотентно: если
	// doctor_id уже nullable — no-op. См. migrateEncountersDoctorIdNullable.
	if err := migrateEncountersDoctorIdNullable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate encounters.doctor_id: %w", err)
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

	// Первый запуск: таблица пуста — создаём DOC-001 с bcrypt-паролем.
	if count == 0 {
		hash, err := HashPassword("doctor@local")
		if err != nil {
			return fmt.Errorf("hash default password: %w", err)
		}
		_, err = db.Exec(
			`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
			 VALUES (?, NULL, ?, ?, ?, 'lor', 'doctor', datetime('now'), 1)`,
			"DOC-001",
			"doctor@local",
			hash,
			"Тестовый врач",
		)
		if err != nil {
			return fmt.Errorf("seed doctor: %w", err)
		}
		slog.Info("seed: создан тестовый врач DOC-001", "email", "doctor@local")
		return nil
	}

	// Таблица не пуста: мигрируем placeholder у DOC-001 → bcrypt (идемпотентно).
	var currentHash string
	err := db.QueryRow(`SELECT password_hash FROM doctors WHERE id = 'DOC-001'`).Scan(&currentHash)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read DOC-001 password_hash: %w", err)
	}
	if IsBcryptHash(currentHash) {
		return nil
	}
	newHash, err := HashPassword("doctor@local")
	if err != nil {
		return fmt.Errorf("hash default password: %w", err)
	}
	if _, err := db.Exec(`UPDATE doctors SET password_hash = ? WHERE id = 'DOC-001'`, newHash); err != nil {
		return fmt.Errorf("update DOC-001 password_hash: %w", err)
	}
	slog.Info("seed: DOC-001 пароль обновлён на bcrypt (было placeholder)")
	return nil
}

// normalizeAgeDisplay — единый формат ageDisplay при чтении пациента.
//
// Старые записи в patients.age_display хранят произвольный формат
// («35 лет»), а patient_profile.go пишет «36 г 9 мес». Если birth_date
// заполнена — пересчитываем Age/AgeMonths/AgeDisplay из неё, чтобы врач
// в карточке видел единый формат независимо от того, когда запись создана.
// Ошибки парсинга birth_date игнорируем: остаётся то, что в БД.
func normalizeAgeDisplay(p *Patient) {
	if p == nil || p.BirthDate == "" {
		return
	}
	age, ageMonths, display, err := calcAgeFromBirthDate(p.BirthDate, time.Now().UTC())
	if err != nil {
		return
	}
	p.Age = age
	p.AgeMonths = ageMonths
	p.AgeDisplay = display
}

// getPatientsByDoctor возвращает всех пациентов врача, отсортированных по created DESC.
func getPatientsByDoctor(db *sql.DB, doctorID string) ([]Patient, error) {
	rows, err := db.Query(`
		SELECT id, COALESCE(doctor_id, ''), COALESCE(clinic_id, ''), name, COALESCE(age, 0),
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
		normalizeAgeDisplay(&p)
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
		SELECT id, COALESCE(doctor_id, ''), COALESCE(clinic_id, ''), name, COALESCE(age, 0),
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
	normalizeAgeDisplay(&p)
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

// deletePatientByDoctor удаляет пациента по id с проверкой владельца.
// Возвращает sql.ErrNoRows, если пациента нет или он принадлежит
// другому врачу. Шаг 6.5.
//
// С 6.8 не используется в handlePatientDelete (там admin-only),
// но оставлена для возможных owner-based сценариев (архив, 6.9+).
func deletePatientByDoctor(db *sql.DB, id, doctorID string) error {
	res, err := db.Exec(`DELETE FROM patients WHERE id = ? AND doctor_id = ?`, id, doctorID)
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

// deletePatientByID удаляет пациента по id БЕЗ проверки владельца.
// Используется admin/superadmin — гейтинг снаружи (requireRole).
// Возвращает sql.ErrNoRows, если пациента нет.
func deletePatientByID(db *sql.DB, id string) error {
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

// seedAdminRoles идемпотентно создаёт служебных superadmin и admin.
// Не зависит от seedDefaultDoctor — вызывается отдельно из main.
// Если врач с таким email уже есть — пропускает.
func seedDefaultPatientAccount(db *sql.DB) error {
	var exists int
	if err := db.QueryRow("SELECT COUNT(*) FROM patient_accounts WHERE login = ?", "patient@local").Scan(&exists); err != nil {
		return fmt.Errorf("check patient@local: %w", err)
	}
	if exists > 0 {
		slog.Info("seed: patient@local уже есть, пропуск")
		return nil
	}
	_, err := db.Exec("INSERT OR IGNORE INTO patients (id, name, birth_date, gender, created) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)", "PAT-DEMO", "Тестовый пациент", "1990-01-01", "male")
	if err != nil {
		return fmt.Errorf("seed patient PAT-DEMO: %w", err)
	}
	hash, err := HashPassword("patient@local")
	if err != nil {
		return fmt.Errorf("hash patient@local: %w", err)
	}
	_, err = db.Exec("INSERT OR IGNORE INTO patient_accounts (id, patient_id, login, password_hash, status, created, updated) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)", "PACC-DEMO", "PAT-DEMO", "patient@local", hash, "active")
	if err != nil {
		return fmt.Errorf("seed patient_accounts PACC-DEMO: %w", err)
	}
	slog.Info("seed: создан тестовый пациент PACC-DEMO", "login", "patient@local")
	return nil
}

func seedAdminRoles(db *sql.DB) error {
	seeds := []struct {
		ID       string
		Email    string
		Password string
		Role     string
		Name     string
	}{
		{"SUPER-001", "superadmin@botmax.local", "superadmin", "superadmin", "Суперадмин"},
		{"ADMIN-001", "admin@botmax.local", "admin", "admin", "Тестовый админ"},
	}

	for _, s := range seeds {
		var exists int
		if err := db.QueryRow(`SELECT COUNT(*) FROM doctors WHERE email = ?`, s.Email).Scan(&exists); err != nil {
			return fmt.Errorf("check %s: %w", s.Email, err)
		}
		if exists > 0 {
			slog.Info("seed: уже есть, пропуск", "email", s.Email)
			continue
		}
		hash, err := HashPassword(s.Password)
		if err != nil {
			return fmt.Errorf("hash for %s: %w", s.Email, err)
		}
		_, err = db.Exec(
			`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
			 VALUES (?, NULL, ?, ?, ?, 'lor', ?, datetime('now'), 1)`,
			s.ID, s.Email, hash, s.Name, s.Role,
		)
		if err != nil {
			return fmt.Errorf("insert %s: %w", s.Email, err)
		}
		slog.Info("seed: создан", "id", s.ID, "email", s.Email, "role", s.Role)
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
		`SELECT COALESCE(bp,''), COALESCE(temp,0), COALESCE(spo2,0), COALESCE(updated,'')
		FROM vitals WHERE patient_id = ?`,
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
		SELECT id, patient_id, COALESCE(bp,''), COALESCE(temp,0), COALESCE(spo2,0), COALESCE(recorded,'')
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
		`SELECT COALESCE(data,''), COALESCE(updated,'') FROM cards WHERE patient_id = ?`,
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

// ---------------------------------------------------------------
// Doctor — Шаг 6.3: чтение doctors для аутентификации.
// ---------------------------------------------------------------

// Doctor — минимальное представление строки doctors, нужное логину.
type Doctor struct {
	ID           string
	Email        string
	PasswordHash string
	Name         string
	Role         string
	Active       bool
}

// getDoctorByEmail ищет врача по email. Возвращает sql.ErrNoRows,
// если не найден — handler логина трактует это как 401.
func getDoctorByEmail(db *sql.DB, email string) (*Doctor, error) {
	const q = `
		SELECT id, email, password_hash, name, role, active
		FROM doctors
		WHERE LOWER(email) = LOWER(?)
	`
	var d Doctor
	err := db.QueryRow(q, email).Scan(
		&d.ID, &d.Email, &d.PasswordHash, &d.Name, &d.Role, &d.Active,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// getDoctorByID ищет врача по id. Возвращает sql.ErrNoRows, если нет.
func getDoctorByID(db *sql.DB, id string) (*Doctor, error) {
	const q = `
		SELECT id, email, password_hash, name, role, active
		FROM doctors
		WHERE id = ?
	`
	var d Doctor
	err := db.QueryRow(q, id).Scan(
		&d.ID, &d.Email, &d.PasswordHash, &d.Name, &d.Role, &d.Active,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// touchLastLogin обновляет last_login. Ошибку логина не валит —
// handler решит, писать её в лог или нет.
func touchLastLogin(db *sql.DB, doctorID string) error {
	_, err := db.Exec(
		`UPDATE doctors SET last_login = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), doctorID,
	)
	if err != nil {
		return fmt.Errorf("touch last_login: %w", err)
	}
	return nil
}

// ensureColumn — идемпотентно добавляет колонку в таблицу (B27.1).
// SQLite не поддерживает "ALTER TABLE ... ADD COLUMN IF NOT EXISTS",
// поэтому сначала проверяем PRAGMA table_info(table), и добавляем только
// если колонки нет.
func ensureColumn(db *sql.DB, table, column, ddl string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return fmt.Errorf("pragma table_info(%s): %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("scan table_info: %w", err)
		}
		if name == column {
			return nil // колонка уже есть
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows.Err: %w", err)
	}
	if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + ddl); err != nil {
		return fmt.Errorf("alter table: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------
// B30.2b: patients.doctor_id → NULLABLE + ON DELETE SET NULL
// ---------------------------------------------------------------
//
// Предыстория: до B30.2b схема patients имела doctor_id TEXT NOT NULL
// и FK ON DELETE CASCADE. Удаление врача каскадно удаляло его пациентов,
// а через дочерние FK (cards/encounters/vitals/consents/patient_accounts/
// vitals_history, все ON DELETE CASCADE) — и всю медицинскую историю.
//
// После B30.2b: doctor_id NULLABLE, FK ON DELETE SET NULL. Удаление врача
// «освобождает» пациентов (doctor_id = NULL), история сохраняется.
//
// SQLite не умеет ALTER COLUMN, поэтому единственный путь — пересоздание:
//   CREATE patients_new → INSERT SELECT * → DROP patients → RENAME → reindex.
// Порядок колонок patients_new 1:1 совпадает с patients — это позволяет
// использовать INSERT ... SELECT * без перечисления колонок.
//
// КРИТИЧНО: PRAGMA foreign_keys=OFF выполняется на ОДНОМ соединении
// (db.Conn), а не на db.Exec. Иначе PRAGMA применится к случайному
// соединению пула, а транзакция пойдёт по другому — и с foreign_keys=ON
// DROP TABLE patients упадёт из-за шести дочерних FK (или каскадно
// снесёт все данные). Это тот же класс бага, что §6.60 (DSN).

// migratePatientsDoctorIdNullable идемпотентно пересоздаёт patients
// с doctor_id NULLABLE и FK ON DELETE SET NULL.
//
// Идемпотентность: если doctor_id уже nullable (PRAGMA table_info →
// notnull == 0), функция возвращает nil, ничего не делая.
func migratePatientsDoctorIdNullable(db *sql.DB) error {
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("db.Conn: %w", err)
	}
	defer conn.Close()

	// Шаг 1: миграция уже применена?
	applied, err := patientsDoctorIdIsNullable(ctx, conn)
	if err != nil {
		return err
	}
	if applied {
		return nil // no-op
	}

	// Шаг 2: foreign_keys = OFF на ЭТОМ соединении.
	// SQLite требует переключать этот PRAGMA вне транзакции.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("pragma foreign_keys=off: %w", err)
	}

	// Шаг 3: пересоздание в транзакции.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op после Commit

	steps := []struct {
		name string
		sql  string
	}{
		{"create patients_new", `CREATE TABLE patients_new (
			id          TEXT PRIMARY KEY,
			doctor_id   TEXT,
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
			FOREIGN KEY(doctor_id) REFERENCES doctors(id) ON DELETE SET NULL,
			FOREIGN KEY(clinic_id) REFERENCES clinics(id) ON DELETE SET NULL
		)`},
		{"copy data", `INSERT INTO patients_new SELECT * FROM patients`},
		{"drop old", `DROP TABLE patients`},
		{"rename", `ALTER TABLE patients_new RENAME TO patients`},
		{"index idx_patients_doctor", `CREATE INDEX idx_patients_doctor ON patients(doctor_id, status)`},
		{"index idx_patients_clinic", `CREATE INDEX idx_patients_clinic ON patients(clinic_id, status)`},
	}

	for _, s := range steps {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	// Шаг 4: foreign_key_check внутри транзакции (до Commit).
	// Если после RENAME остались нарушения FK — откатываем всё.
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	hasViolation := rows.Next()
	rows.Close()
	if hasViolation {
		return fmt.Errorf("foreign_key_check: обнаружены нарушения после миграции")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Шаг 5: включить foreign_keys обратно на этом соединении.
	// DSN всё равно включит его на всех новых соединениях пула, но
	// текущее соединение вернётся в пул — пусть будет в известном состоянии.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("pragma foreign_keys=on: %w", err)
	}

	slog.Info("migrate: patients.doctor_id → NULLABLE + ON DELETE SET NULL")
	return nil
}

// patientsDoctorIdIsNullable возвращает true, если у patients.doctor_id
// notnull == 0 (т.е. колонка nullable — миграция уже применена).
//
// Стиль и PRAGMA table_info — как у ensureColumn (B27.1).
func patientsDoctorIdIsNullable(ctx context.Context, conn *sql.Conn) (bool, error) {
	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(patients)")
	if err != nil {
		return false, fmt.Errorf("pragma table_info(patients): %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info: %w", err)
		}
		if name == "doctor_id" {
			found = true
			if notnull == 0 {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("rows.Err: %w", err)
	}
	if !found {
		return false, fmt.Errorf("колонка patients.doctor_id не найдена")
	}
	return false, nil
}

// migrateEncountersDoctorIdNullable идемпотентно пересоздаёт encounters
// с doctor_id NULLABLE и FK ON DELETE SET NULL.
//
// Паттерн — 1:1 с migratePatientsDoctorIdNullable (B30.2b, §6.61/§6.62):
// PRAGMA foreign_keys управляется ВНЕ транзакции на выделенном conn;
// rebuild в tx; foreign_key_check до Commit.
//
// Идемпотентность: если doctor_id уже nullable (PRAGMA table_info →
// notnull == 0), функция возвращает nil, ничего не делая.
func migrateEncountersDoctorIdNullable(db *sql.DB) error {
	ctx := context.Background()

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("db.Conn: %w", err)
	}
	defer conn.Close()

	// Шаг 1: миграция уже применена?
	applied, err := encountersDoctorIdIsNullable(ctx, conn)
	if err != nil {
		return err
	}
	if applied {
		return nil // no-op
	}

	// Шаг 2: foreign_keys = OFF на ЭТОМ соединении.
	// SQLite требует переключать этот PRAGMA вне транзакции.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("pragma foreign_keys=off: %w", err)
	}

	// Шаг 3: пересоздание в транзакции.
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // no-op после Commit

	steps := []struct {
		name string
		sql  string
	}{
		{"create encounters_new", `CREATE TABLE encounters_new (
			id              TEXT PRIMARY KEY,
			patient_id      TEXT NOT NULL,
			doctor_id       TEXT,
			clinic_id       TEXT,
			specialty       TEXT NOT NULL DEFAULT '',
			title           TEXT NOT NULL DEFAULT '',
			kind            TEXT NOT NULL DEFAULT 'consultation',
			started_at      TEXT NOT NULL,
			finished_at     TEXT,
			status          TEXT NOT NULL DEFAULT 'draft',
			complaint       TEXT NOT NULL DEFAULT '',
			anamnesis       TEXT NOT NULL DEFAULT '',
			examination     TEXT NOT NULL DEFAULT '',
			diagnosis_text  TEXT NOT NULL DEFAULT '',
			diagnosis_icd10 TEXT NOT NULL DEFAULT '',
			recommendations TEXT NOT NULL DEFAULT '',
			common_json     TEXT NOT NULL DEFAULT '{}',
			specialty_json  TEXT NOT NULL DEFAULT '{}',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL,
			FOREIGN KEY(patient_id) REFERENCES patients(id) ON DELETE CASCADE,
			FOREIGN KEY(doctor_id)  REFERENCES doctors(id)  ON DELETE SET NULL,
			FOREIGN KEY(clinic_id)  REFERENCES clinics(id)  ON DELETE SET NULL
		)`},
		{"copy data", `INSERT INTO encounters_new (
			id, patient_id, doctor_id, clinic_id, specialty, title, kind,
			started_at, finished_at, status, complaint, anamnesis, examination,
			diagnosis_text, diagnosis_icd10, recommendations, common_json,
			specialty_json, created_at, updated_at
		) SELECT
			id, patient_id, doctor_id, clinic_id, specialty, title, kind,
			started_at, finished_at, status, complaint, anamnesis, examination,
			diagnosis_text, diagnosis_icd10, recommendations, common_json,
			specialty_json, created_at, updated_at
		FROM encounters`},
		{"drop old", `DROP TABLE encounters`},
		{"rename", `ALTER TABLE encounters_new RENAME TO encounters`},
		{"index idx_enc_patient", `CREATE INDEX idx_enc_patient ON encounters(patient_id, started_at DESC)`},
		{"index idx_enc_doctor", `CREATE INDEX idx_enc_doctor ON encounters(doctor_id, started_at DESC)`},
		{"index idx_enc_clinic", `CREATE INDEX idx_enc_clinic ON encounters(clinic_id, started_at DESC)`},
		{"index idx_enc_patient_kind", `CREATE INDEX idx_enc_patient_kind ON encounters(patient_id, kind, started_at DESC)`},
	}

	for _, s := range steps {
		if _, err := tx.ExecContext(ctx, s.sql); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	// Шаг 4: foreign_key_check внутри транзакции (до Commit).
	rows, err := tx.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("foreign_key_check: %w", err)
	}
	hasViolation := rows.Next()
	rows.Close()
	if hasViolation {
		return fmt.Errorf("foreign_key_check: обнаружены нарушения после миграции")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	// Шаг 5: включить foreign_keys обратно на этом соединении.
	if _, err := conn.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("pragma foreign_keys=on: %w", err)
	}

	slog.Info("migrate: encounters.doctor_id → NULLABLE + ON DELETE SET NULL")
	return nil
}

// encountersDoctorIdIsNullable возвращает true, если у encounters.doctor_id
// notnull == 0 (т.е. колонка nullable — миграция уже применена).
//
// Стиль и PRAGMA table_info — 1:1 с patientsDoctorIdIsNullable.
func encountersDoctorIdIsNullable(ctx context.Context, conn *sql.Conn) (bool, error) {
	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(encounters)")
	if err != nil {
		return false, fmt.Errorf("pragma table_info(encounters): %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scan table_info: %w", err)
		}
		if name == "doctor_id" {
			found = true
			if notnull == 0 {
				return true, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("rows.Err: %w", err)
	}
	if !found {
		return false, fmt.Errorf("колонка encounters.doctor_id не найдена")
	}
	return false, nil
}

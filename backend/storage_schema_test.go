package main

// storage_schema_test.go — B30.10 §6.59 step4a: миграция схемы files.
// Чистая БД — через InitDB; legacy-БД — через сырой sql.Open (без миграций),
// чтобы проверить, что openDB-путь (InitDB) докатывает storage_* идемпотентно.

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func filesColumnSet(t *testing.T, db *sql.DB) map[string]bool {
	t.Helper()
	cols := map[string]bool{}
	rows, err := db.Query(`PRAGMA table_info(files)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(files): %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid     int
			name    string
			ctype   string
			notnull int
			dflt    any
			pk      int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		cols[name] = true
	}
	return cols
}

func TestFilesSchema_HasStorageColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	cols := filesColumnSet(t, db)
	for _, want := range []string{"storage_kind", "storage_key"} {
		if !cols[want] {
			t.Fatalf("колонка files.%s отсутствует (cols=%v)", want, cols)
		}
	}

	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_files_storage_kind'`,
	).Scan(&n); err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	if n != 1 {
		t.Fatalf("idx_files_storage_kind отсутствует (n=%d)", n)
	}
}

func TestFilesSchema_LegacyDBUpgrade(t *testing.T) {
	// 1) «старая» схема files — без storage_kind/storage_key, через сырой sql.Open,
	//    без InitDB (иначе миграции накатятся сразу и тест будет бессмысленным).
	path := filepath.Join(t.TempDir(), "legacy.db")
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("sql.Open(legacy): %v", err)
	}
	if _, err := raw.Exec(`
                -- legacy: 1:1 с боевой схемой ДО B30.10 §6.59
                -- (нет files.storage_kind / storage_key и idx_files_storage_kind).
                -- ВАЖНО: patients.status/doctor_id/clinic_id и encounters.* обязательны —
                -- на них ссылаются индексы из db.go:schema (idx_patients_*,
                -- idx_enc_*). Иначе CREATE INDEX IF NOT EXISTS упадёт раньше
                -- ensureColumn-цикла, и тест будет падать не по делу.
                CREATE TABLE patients (
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
                        last_sync   TEXT
                );
                CREATE TABLE encounters (
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
                        updated_at      TEXT NOT NULL
                );
                CREATE TABLE files (
                        id          TEXT PRIMARY KEY,
                        patient_id  TEXT NOT NULL,
                        clinic_id   TEXT,
                        filename    TEXT NOT NULL DEFAULT '',
                        mime        TEXT,
                        size_bytes  INTEGER,
                        data        BLOB,
                        notes       TEXT,
                        created_at  TEXT NOT NULL,
                        updated_at  TEXT NOT NULL,
                        deleted_at  TEXT
                );
                CREATE INDEX idx_patients_clinic   ON patients(clinic_id, status);
                CREATE INDEX idx_patients_doctor   ON patients(doctor_id, status);
                CREATE INDEX idx_enc_clinic        ON encounters(clinic_id, started_at DESC);
                CREATE INDEX idx_enc_doctor        ON encounters(doctor_id, started_at DESC);
                CREATE INDEX idx_enc_patient       ON encounters(patient_id, started_at DESC);
                CREATE INDEX idx_enc_patient_kind  ON encounters(patient_id, kind, started_at DESC);
                CREATE INDEX idx_files_patient     ON files(patient_id, updated_at DESC);
                CREATE INDEX idx_files_alive       ON files(patient_id, deleted_at);
`); err != nil {
		raw.Close()
		t.Fatalf("legacy schema: %v", err)
	}
	raw.Close()

	// 2) InitDB должен докатить storage_* + индекс, не теряя данные.
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB(legacy): %v", err)
	}
	defer db.Close()

	cols := filesColumnSet(t, db)
	for _, want := range []string{"storage_kind", "storage_key"} {
		if !cols[want] {
			t.Fatalf("legacy: колонка files.%s не докатилась (cols=%v)", want, cols)
		}
	}
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_files_storage_kind'`,
	).Scan(&n); err != nil {
		t.Fatalf("sqlite_master: %v", err)
	}
	if n != 1 {
		t.Fatalf("legacy: idx_files_storage_kind не создан (n=%d)", n)
	}

	// 3) существующие строки получили дефолт storage_kind='blob'.
	if _, err := db.Exec(`INSERT INTO patients (id, name, created) VALUES ('P', 'Тест', 't')`); err != nil {
		t.Fatalf("insert patient: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO files (id, patient_id, created_at, updated_at) VALUES ('F','P','t','t')`); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	var kind string
	if err := db.QueryRow(`SELECT storage_kind FROM files WHERE id='F'`).Scan(&kind); err != nil {
		t.Fatalf("select storage_kind: %v", err)
	}
	if kind != "blob" {
		t.Fatalf("storage_kind=%q, want 'blob' (дефолт)", kind)
	}
}

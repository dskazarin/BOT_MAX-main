package main

// db_test.go — подпатч 6.36 (B27.1): тесты схемы и миграций.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// newFileTestDB — файловая SQLite в t.TempDir() (не :memory:).
// Нужна тестам, которые проверяют поведение через db.Conn(ctx): с :memory:
// каждое соединение пула открывало бы свою независимую БД.
func newFileTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB(%s): %v", path, err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// TestInitDB_ForeignKeysOnAllConns — регрессия: PRAGMA foreign_keys
// должен быть включён на КАЖДОМ соединении пула, а не только на том,
// где его выполнили через db.Exec.
//
// Использует файловую БД (t.TempDir()), а не :memory: — иначе каждое
// соединение пула открывало бы свою БД, и проверка теряла бы смысл.
func TestInitDB_ForeignKeysOnAllConns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	db.SetMaxOpenConns(5)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		conn, err := db.Conn(ctx)
		if err != nil {
			t.Fatalf("db.Conn #%d: %v", i, err)
		}

		var fk int
		if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
			conn.Close()
			t.Fatalf("PRAGMA foreign_keys на conn #%d: %v", i, err)
		}
		conn.Close()

		if fk != 1 {
			t.Errorf("conn #%d: foreign_keys = %d, ожидалось 1", i, fk)
		}
	}
}

// TestInitDB_SetNullOnDoctorDelete — после B30.2b удаление врача
// НЕ удаляет пациентов, а устанавливает patients.doctor_id = NULL
// (ON DELETE SET NULL). История пациентов сохраняется.
func TestInitDB_SetNullOnDoctorDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	if err := seedDefaultDoctor(db); err != nil {
		t.Fatalf("seedDefaultDoctor: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO patients (id, doctor_id, name, created)
		VALUES ('PAT-TEST', 'DOC-001', 'Test', '2026-01-01')
	`)
	if err != nil {
		t.Fatalf("insert patient: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM patients WHERE id = 'PAT-TEST'`,
	).Scan(&count); err != nil {
		t.Fatalf("count before delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("пациент не вставился, count = %d", count)
	}

	if _, err := db.Exec(`DELETE FROM doctors WHERE id = 'DOC-001'`); err != nil {
		t.Fatalf("delete doctor: %v", err)
	}

	if err := db.QueryRow(
		`SELECT COUNT(*) FROM patients WHERE id = 'PAT-TEST'`,
	).Scan(&count); err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 1 {
		t.Fatalf("пациент удалён, а должен остаться: count = %d", count)
	}

	var doctorID sql.NullString
	if err := db.QueryRow(
		`SELECT doctor_id FROM patients WHERE id = 'PAT-TEST'`,
	).Scan(&doctorID); err != nil {
		t.Fatalf("select doctor_id: %v", err)
	}
	if doctorID.Valid {
		t.Errorf("doctor_id = %q, ожидалось NULL (SET NULL)", doctorID.String)
	}
}

// TestInitDB_PasswordResetTokens — таблица и индексы созданы.
func TestInitDB_PasswordResetTokens(t *testing.T) {
	db := newTestDB(t)

	// Таблица есть.
	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='password_reset_tokens'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("password_reset_tokens не найдена: %v", err)
	}

	// Индексы есть.
	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='password_reset_tokens'`,
	)
	if err != nil {
		t.Fatalf("query индексов: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var idx string
		if err := rows.Scan(&idx); err != nil {
			t.Fatalf("scan индекса: %v", err)
		}
		got[idx] = true
	}
	for _, want := range []string{"idx_prt_doctor", "idx_prt_expires"} {
		if !got[want] {
			t.Errorf("индекс %s не найден; got=%v", want, got)
		}
	}
}

// TestInitDB_DoctorsMigrations — поля B27 добавлены.
func TestInitDB_DoctorsMigrations(t *testing.T) {
	db := newTestDB(t)

	want := map[string]bool{
		"must_change_password": true,
		"password_changed_at":  true,
		"failed_login_count":   true,
		"locked_until":         true,
	}

	rows, err := db.Query(`PRAGMA table_info(doctors)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
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
			t.Fatalf("scan: %v", err)
		}
		got[name] = true
	}
	for col := range want {
		if !got[col] {
			t.Errorf("колонка doctors.%s отсутствует", col)
		}
	}
}

// TestEnsureColumn_Idempotent — двойной вызов не должен падать.
func TestEnsureColumn_Idempotent(t *testing.T) {
	db := newTestDB(t)

	// Первый вызов — колонка может уже существовать (миграция выше) или создастся.
	if err := ensureColumn(db, "doctors", "test_col_b27", "TEXT"); err != nil {
		t.Fatalf("первый ensureColumn: %v", err)
	}
	// Второй вызов — no-op.
	if err := ensureColumn(db, "doctors", "test_col_b27", "TEXT"); err != nil {
		t.Fatalf("второй ensureColumn (idempotent): %v", err)
	}

	// Проверим, что колонка ровно одна (не дублируется).
	rows, err := db.Query(`PRAGMA table_info(doctors)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	count := 0
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		_ = rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk)
		if name == "test_col_b27" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("колонка test_col_b27 должна быть одна, got %d", count)
	}
}

// TestInitDB_Encounters — B28.1: таблица encounters + 3 индекса.
func TestInitDB_Encounters(t *testing.T) {
	db := newTestDB(t)

	var name string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='table' AND name='encounters'`,
	).Scan(&name)
	if err != nil {
		t.Fatalf("encounters не найдена: %v", err)
	}

	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='encounters'`,
	)
	if err != nil {
		t.Fatalf("query индексов: %v", err)
	}
	defer rows.Close()
	got := map[string]bool{}
	for rows.Next() {
		var idx string
		_ = rows.Scan(&idx)
		got[idx] = true
	}
	for _, want := range []string{"idx_enc_patient", "idx_enc_doctor", "idx_enc_clinic", "idx_enc_patient_kind"} {
		if !got[want] {
			t.Errorf("индекс %s не найден; got=%v", want, got)
		}
	}
}

// TestNewID — B28.1: генератор ID.
func TestNewID(t *testing.T) {
	id1, err := newID("ENC")
	if err != nil {
		t.Fatalf("newID: %v", err)
	}
	id2, err := newID("ENC")
	if err != nil {
		t.Fatalf("newID: %v", err)
	}
	if id1 == id2 {
		t.Fatalf("ID не уникальны: %q == %q", id1, id2)
	}
	if len(id1) < 10 {
		t.Fatalf("ID слишком короткий: %q", id1)
	}
	if id1[:4] != "ENC-" {
		t.Fatalf("ID не начинается с ENC-: %q", id1)
	}
}

// ---------------------------------------------------------------
// B30.2b: миграция patients.doctor_id → NULLABLE + ON DELETE SET NULL
// ---------------------------------------------------------------

// TestMigrate_DoctorIdNullable_Applied — после InitDB doctor_id nullable.
func TestMigrate_DoctorIdNullable_Applied(t *testing.T) {
	db := newFileTestDB(t)

	rows, err := db.Query(`PRAGMA table_info(patients)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	found := false
	var notnull int
	for rows.Next() {
		var (
			cid       int
			name      string
			ctype     string
			nn        int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &ctype, &nn, &dfltValue, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "doctor_id" {
			notnull = nn
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor_id не найдена")
	}
	if notnull != 0 {
		t.Errorf("doctor_id.notnull = %d, ожидалось 0 (nullable)", notnull)
	}
}

// TestMigrate_DoctorIdNullable_Idempotent — повторный InitDB на той же БД
// не должен падать и не должен менять схему.
func TestMigrate_DoctorIdNullable_Idempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	db1, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB #1: %v", err)
	}
	db1.Close()

	db2, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB #2 (idempotent): %v", err)
	}
	defer db2.Close()

	var notnull int
	if err := db2.QueryRow(`SELECT "notnull" FROM pragma_table_info('patients') WHERE name = 'doctor_id'`).Scan(&notnull); err != nil {
		t.Fatalf("read notnull: %v", err)
	}
	if notnull != 0 {
		t.Errorf("после второго InitDB doctor_id.notnull = %d, ожидалось 0", notnull)
	}
}

// TestMigrate_DoctorIdNullable_DataPreserved — данные не потеряны.
func TestMigrate_DoctorIdNullable_DataPreserved(t *testing.T) {
	db := newFileTestDB(t)
	seedDoctorForRefresh(t, db, "DOC-DATA")

	for _, id := range []string{"P1", "P2", "P3"} {
		if _, err := db.Exec(
			`INSERT INTO patients (id, doctor_id, name, created) VALUES (?, 'DOC-DATA', ?, '2026-01-01')`,
			id, "Пациент "+id,
		); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM patients`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 3 {
		t.Errorf("count = %d, ожидалось 3", count)
	}

	var doctorID string
	if err := db.QueryRow(`SELECT doctor_id FROM patients WHERE id = 'P1'`).Scan(&doctorID); err != nil {
		t.Fatalf("select: %v", err)
	}
	if doctorID != "DOC-DATA" {
		t.Errorf("doctor_id = %q, ожидалось DOC-DATA", doctorID)
	}
}

// TestMigrate_DoctorIdNullable_FKOnDeleteSetNull — удаление доктора
// устанавливает doctor_id = NULL (не CASCADE).
func TestMigrate_DoctorIdNullable_FKOnDeleteSetNull(t *testing.T) {
	db := newFileTestDB(t)
	seedDoctorForRefresh(t, db, "DOC-FK")

	if _, err := db.Exec(
		`INSERT INTO patients (id, doctor_id, name, created) VALUES ('PAT-FK', 'DOC-FK', 'Test', '2026-01-01')`,
	); err != nil {
		t.Fatalf("insert patient: %v", err)
	}

	if _, err := db.Exec(`DELETE FROM doctors WHERE id = 'DOC-FK'`); err != nil {
		t.Fatalf("delete doctor: %v", err)
	}

	var doctorID sql.NullString
	if err := db.QueryRow(`SELECT doctor_id FROM patients WHERE id = 'PAT-FK'`).Scan(&doctorID); err != nil {
		t.Fatalf("select doctor_id: %v", err)
	}
	if doctorID.Valid {
		t.Errorf("doctor_id = %q, ожидалось NULL", doctorID.String)
	}
}

// TestMigrate_DoctorIdNullable_IndexesRecreated — оба индекса на месте.
func TestMigrate_DoctorIdNullable_IndexesRecreated(t *testing.T) {
	db := newFileTestDB(t)

	rows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='patients'`,
	)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()

	got := map[string]bool{}
	for rows.Next() {
		var idx string
		_ = rows.Scan(&idx)
		got[idx] = true
	}
	for _, want := range []string{"idx_patients_doctor", "idx_patients_clinic"} {
		if !got[want] {
			t.Errorf("индекс %s не найден; got=%v", want, got)
		}
	}
}

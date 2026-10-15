package main

// db_test.go — подпатч 6.36 (B27.1): тесты схемы и миграций.

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

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

// TestInitDB_CascadeDeleteOnDoctor — с включённым foreign_keys удаление
// врача каскадно удаляет его пациентов (семантика ДО B30.2b).
// После B30.2b семантика станет ON DELETE SET NULL — тест будет заменён
// на TestInitDB_SetNullOnDoctorDelete.
func TestInitDB_CascadeDeleteOnDoctor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	defer db.Close()

	// InitDB создаёт схему, но не создаёт DOC-001 — сид вызывается
	// отдельно (seed.go / main). В тесте вызываем явно.
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
	if count != 0 {
		t.Errorf("каскад не сработал: count = %d, ожидалось 0", count)
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

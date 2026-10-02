package main

// refresh_test.go — подпатч 6.13 (B4.5): тесты store refresh-токенов.
//
// Использует in-memory SQLite (:memory:) с полной схемой через InitDB.
// Каждый тест — свой *sql.DB (не параллельно), чтобы не мешать друг другу.

import (
	"database/sql"
	"testing"
	"time"
)

// newTestDB открывает :memory: SQLite со схемой.
// Закрывается автоматически через t.Cleanup.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB(:memory:): %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// seedDoctorForRefresh создаёт врача — нужен для FK refresh_tokens.doctor_id.
func seedDoctorForRefresh(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
		 VALUES (?, NULL, ?, 'x', 'Test', 'lor', 'doctor', datetime('now'), 1)`,
		id, id+"@test.local",
	)
	if err != nil {
		t.Fatalf("insert doctor %s: %v", id, err)
	}
}

// ---------------------------------------------------------------
// Save + IsRevoked: базовый цикл
// ---------------------------------------------------------------

func TestRefresh_SaveAndNotRevoked(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	exp := time.Now().Add(time.Hour)
	if err := SaveRefreshJTI(db, "jti-1", "D1", exp); err != nil {
		t.Fatalf("SaveRefreshJTI: %v", err)
	}

	revoked, err := IsRefreshRevoked(db, "jti-1")
	if err != nil {
		t.Fatalf("IsRefreshRevoked: %v", err)
	}
	if revoked {
		t.Fatalf("свежий jti должен быть НЕ revoked")
	}
}

// Неизвестный jti → revoked=true (безопасный дефолт).
func TestRefresh_UnknownIsRevoked(t *testing.T) {
	db := newTestDB(t)

	revoked, err := IsRefreshRevoked(db, "never-existed")
	if err != nil {
		t.Fatalf("IsRefreshRevoked: %v", err)
	}
	if !revoked {
		t.Fatalf("неизвестный jti должен считаться revoked")
	}
}

// Пустой jti → revoked=true, без ошибки.
func TestRefresh_EmptyJTIRevoked(t *testing.T) {
	db := newTestDB(t)

	revoked, err := IsRefreshRevoked(db, "")
	if err != nil {
		t.Fatalf("IsRefreshRevoked(\"\"): %v", err)
	}
	if !revoked {
		t.Fatalf("пустой jti должен считаться revoked")
	}
}

// ---------------------------------------------------------------
// Revoke
// ---------------------------------------------------------------

func TestRefresh_Revoke(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D2")

	if err := SaveRefreshJTI(db, "jti-rev", "D2", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshJTI: %v", err)
	}

	if err := RevokeRefreshJTI(db, "jti-rev", "jti-new"); err != nil {
		t.Fatalf("RevokeRefreshJTI: %v", err)
	}

	revoked, err := IsRefreshRevoked(db, "jti-rev")
	if err != nil {
		t.Fatalf("IsRefreshRevoked: %v", err)
	}
	if !revoked {
		t.Fatalf("после RevokeRefreshJTI jti должен быть revoked")
	}

	// Проверим replaced_by
	var replacedBy sql.NullString
	if err := db.QueryRow(
		`SELECT replaced_by FROM refresh_tokens WHERE jti = ?`, "jti-rev",
	).Scan(&replacedBy); err != nil {
		t.Fatalf("select replaced_by: %v", err)
	}
	if !replacedBy.Valid || replacedBy.String != "jti-new" {
		t.Fatalf("replaced_by = %v, ожидали \"jti-new\"", replacedBy)
	}
}

// Повторный Revoke — не падает (WHERE revoked_at IS NULL).
func TestRefresh_RevokeIdempotent(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D3")

	if err := SaveRefreshJTI(db, "jti-idem", "D3", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshJTI: %v", err)
	}
	if err := RevokeRefreshJTI(db, "jti-idem", "first"); err != nil {
		t.Fatalf("Revoke #1: %v", err)
	}
	if err := RevokeRefreshJTI(db, "jti-idem", "second"); err != nil {
		t.Fatalf("Revoke #2: %v", err)
	}

	// replaced_by должен остаться "first" (второй UPDATE не прошёл, WHERE не сработал).
	var replacedBy sql.NullString
	if err := db.QueryRow(
		`SELECT replaced_by FROM refresh_tokens WHERE jti = ?`, "jti-idem",
	).Scan(&replacedBy); err != nil {
		t.Fatalf("select replaced_by: %v", err)
	}
	if replacedBy.String != "first" {
		t.Fatalf("replaced_by = %q, ожидали \"first\" (идемпотентность)", replacedBy.String)
	}
}

// ---------------------------------------------------------------
// RevokeAllForDoctor — каскад
// ---------------------------------------------------------------

func TestRefresh_RevokeAllForDoctor(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D4")
	seedDoctorForRefresh(t, db, "D5") // чужой врач — не должен пострадать

	exp := time.Now().Add(time.Hour)
	// три активных jti у D4
	for _, j := range []string{"j1", "j2", "j3"} {
		if err := SaveRefreshJTI(db, j, "D4", exp); err != nil {
			t.Fatalf("SaveRefreshJTI(%s): %v", j, err)
		}
	}
	// один у D5
	if err := SaveRefreshJTI(db, "foreign", "D5", exp); err != nil {
		t.Fatalf("SaveRefreshJTI(foreign): %v", err)
	}

	n, err := RevokeAllForDoctor(db, "D4")
	if err != nil {
		t.Fatalf("RevokeAllForDoctor: %v", err)
	}
	if n != 3 {
		t.Fatalf("RevokeAllForDoctor отозвал %d, ожидали 3", n)
	}

	for _, j := range []string{"j1", "j2", "j3"} {
		revoked, _ := IsRefreshRevoked(db, j)
		if !revoked {
			t.Fatalf("jti %s должен быть revoked после каскада", j)
		}
	}

	// D5 не пострадал
	revoked, _ := IsRefreshRevoked(db, "foreign")
	if revoked {
		t.Fatalf("чужой jti (D5) не должен быть revoked")
	}
}

// ---------------------------------------------------------------
// Lazy-cleanup: SaveRefreshJTI удаляет expired
// ---------------------------------------------------------------

func TestRefresh_SaveCleansExpired(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D6")

	// Вставим вручную уже истёкший jti.
	_, err := db.Exec(
		`INSERT INTO refresh_tokens (jti, doctor_id, issued_at, expires_at, revoked_at, replaced_by)
		 VALUES (?, ?, datetime('now', '-2 days'), datetime('now', '-1 day'), NULL, NULL)`,
		"old-jti", "D6",
	)
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}

	// SaveRefreshJTI должен удалить old-jti.
	if err := SaveRefreshJTI(db, "new-jti", "D6", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshJTI: %v", err)
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM refresh_tokens WHERE jti = ?`, "old-jti",
	).Scan(&count); err != nil {
		t.Fatalf("count old-jti: %v", err)
	}
	if count != 0 {
		t.Fatalf("old-jti должен был быть удалён (cleanup), count=%d", count)
	}
}

// ---------------------------------------------------------------
// B23.1 — refreshCleanup (retention)
// ---------------------------------------------------------------

// TestRefreshCleanup_DeletesOld — удаляет записи, у которых expires_at
// старше N дней.
func TestRefreshCleanup_DeletesOld(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// 10 дней назад — заведомо старше 5-дневного retention'а.
	old := time.Now().AddDate(0, 0, -10).UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO refresh_tokens (jti, doctor_id, issued_at, expires_at) VALUES (?, ?, datetime('now'), ?)`,
		"old-jti", "D1", old,
	)
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}

	n, err := refreshCleanup(db, 5)
	if err != nil {
		t.Fatalf("refreshCleanup: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted: got %d, want 1", n)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM refresh_tokens WHERE jti = 'old-jti'`).Scan(&count)
	if count != 0 {
		t.Fatalf("старая запись должна быть удалена, осталось %d", count)
	}
}

// TestRefreshCleanup_KeepsFresh — свежие записи не трогает.
func TestRefreshCleanup_KeepsFresh(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// через 1 час — свежая.
	fresh := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO refresh_tokens (jti, doctor_id, issued_at, expires_at) VALUES (?, ?, datetime('now'), ?)`,
		"fresh-jti", "D1", fresh,
	)
	if err != nil {
		t.Fatalf("insert fresh: %v", err)
	}

	n, err := refreshCleanup(db, 5)
	if err != nil {
		t.Fatalf("refreshCleanup: %v", err)
	}
	if n != 0 {
		t.Fatalf("deleted: got %d, want 0", n)
	}
}

// TestRefreshCleanup_ZeroDays_Error — days <= 0 → ошибка.
func TestRefreshCleanup_ZeroDays_Error(t *testing.T) {
	db := newTestDB(t)
	if _, err := refreshCleanup(db, 0); err == nil {
		t.Fatal("ожидали ошибку при days=0")
	}
}

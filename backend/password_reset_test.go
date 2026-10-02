package main

// password_reset_test.go — подпатч 6.36 (B27.2): тесты store password reset
// + brute-force lock.
//
// Переиспользует newTestDB(t), seedDoctorForRefresh(t, db, id) из refresh_test.go.

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// ---------------------------------------------------------------
// Password reset tokens
// ---------------------------------------------------------------

func TestPasswordResetToken_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	if err := CreatePasswordResetToken(db, "D1", "hash-1"); err != nil {
		t.Fatalf("CreatePasswordResetToken: %v", err)
	}

	row, err := GetPasswordResetToken(db, "hash-1")
	if err != nil {
		t.Fatalf("GetPasswordResetToken: %v", err)
	}
	if row.DoctorID != "D1" {
		t.Errorf("DoctorID: got %q, want D1", row.DoctorID)
	}
	if row.UsedAt.Valid {
		t.Errorf("UsedAt должен быть NULL")
	}
	if row.ExpiresAt.Before(time.Now()) {
		t.Errorf("ExpiresAt должен быть в будущем")
	}
}

func TestPasswordResetToken_Expired(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// Вставим протухший токен напрямую.
	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	created := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO password_reset_tokens (token_hash, doctor_id, created_at, expires_at, used_at)
		 VALUES (?, ?, ?, ?, NULL)`,
		"expired-hash", "D1", created, past,
	)
	if err != nil {
		t.Fatalf("insert expired: %v", err)
	}

	if _, err := GetPasswordResetToken(db, "expired-hash"); !errors.Is(err, ErrPasswordResetTokenInvalid) {
		t.Fatalf("ожидали ErrPasswordResetTokenInvalid, got %v", err)
	}
}

func TestPasswordResetToken_Consumed(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	if err := CreatePasswordResetToken(db, "D1", "hash-2"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := ConsumePasswordResetToken(db, "hash-2"); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if _, err := GetPasswordResetToken(db, "hash-2"); !errors.Is(err, ErrPasswordResetTokenInvalid) {
		t.Fatalf("ожидали ErrPasswordResetTokenInvalid после Consume, got %v", err)
	}

	// Повторный Consume — идемпотентно.
	if err := ConsumePasswordResetToken(db, "hash-2"); err != nil {
		t.Fatalf("второй Consume: %v", err)
	}
}

func TestPasswordResetToken_DeleteExpired(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// Протухший.
	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	_, _ = db.Exec(
		`INSERT INTO password_reset_tokens (token_hash, doctor_id, created_at, expires_at, used_at)
		 VALUES (?, ?, ?, ?, NULL)`,
		"old", "D1", past, past,
	)
	// Живой.
	if err := CreatePasswordResetToken(db, "D1", "fresh"); err != nil {
		t.Fatalf("Create fresh: %v", err)
	}

	n, err := DeleteExpiredPasswordResetTokens(db)
	if err != nil {
		t.Fatalf("DeleteExpired: %v", err)
	}
	if n != 1 {
		t.Fatalf("удалено: got %d, want 1", n)
	}
	// Живой остался.
	if _, err := GetPasswordResetToken(db, "fresh"); err != nil {
		t.Fatalf("свежий токен должен остаться: %v", err)
	}
}

// ---------------------------------------------------------------
// UpdatePasswordHash + SetMustChangePassword
// ---------------------------------------------------------------

func TestUpdatePasswordHash_ClearsMustChangeAndLock(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// Поставим флаг + заблокируем.
	if err := SetMustChangePassword(db, "D1", true); err != nil {
		t.Fatalf("SetMustChangePassword: %v", err)
	}
	if err := LockDoctor(db, "D1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("LockDoctor: %v", err)
	}

	// Обновим пароль.
	if err := UpdatePasswordHash(db, "D1", "new-bcrypt-hash"); err != nil {
		t.Fatalf("UpdatePasswordHash: %v", err)
	}

	// Проверим: пароль, must_change=0, locked_until=NULL, password_changed_at заполнено.
	var (
		hash       string
		mustChange int
		locked     sql.NullString
		changedAt  sql.NullString
	)
	err := db.QueryRow(
		`SELECT password_hash, must_change_password, locked_until, password_changed_at
		 FROM doctors WHERE id = 'D1'`,
	).Scan(&hash, &mustChange, &locked, &changedAt)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if hash != "new-bcrypt-hash" {
		t.Errorf("hash: got %q", hash)
	}
	if mustChange != 0 {
		t.Errorf("must_change_password: got %d, want 0", mustChange)
	}
	if locked.Valid && locked.String != "" {
		t.Errorf("locked_until должен быть NULL, got %v", locked)
	}
	if !changedAt.Valid || changedAt.String == "" {
		t.Errorf("password_changed_at должен быть установлен")
	}
}

func TestSetMustChangePassword_Toggle(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	if err := SetMustChangePassword(db, "D1", true); err != nil {
		t.Fatalf("set true: %v", err)
	}
	var v int
	_ = db.QueryRow(`SELECT must_change_password FROM doctors WHERE id='D1'`).Scan(&v)
	if v != 1 {
		t.Fatalf("got %d, want 1", v)
	}
	if err := SetMustChangePassword(db, "D1", false); err != nil {
		t.Fatalf("set false: %v", err)
	}
	_ = db.QueryRow(`SELECT must_change_password FROM doctors WHERE id='D1'`).Scan(&v)
	if v != 0 {
		t.Fatalf("got %d, want 0", v)
	}
}

// ---------------------------------------------------------------
// Brute-force lock
// ---------------------------------------------------------------

func TestFailedLogin_IncrementAndLock(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	for i := 1; i <= MaxFailedLogins; i++ {
		n, err := IncrementFailedLogin(db, "D1")
		if err != nil {
			t.Fatalf("итерация %d: %v", i, err)
		}
		if n != i {
			t.Fatalf("итерация %d: count = %d", i, n)
		}
	}

	// После 5 неудач — заблокирован.
	locked, until, err := IsDoctorLocked(db, "D1")
	if err != nil {
		t.Fatalf("IsDoctorLocked: %v", err)
	}
	if !locked {
		t.Fatalf("доктор должен быть заблокирован после %d неудач", MaxFailedLogins)
	}
	if until.Before(time.Now()) {
		t.Errorf("until должен быть в будущем, got %v", until)
	}
}

func TestFailedLogin_Reset(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	_, _ = IncrementFailedLogin(db, "D1")
	_, _ = IncrementFailedLogin(db, "D1")

	if err := ResetFailedLogin(db, "D1"); err != nil {
		t.Fatalf("ResetFailedLogin: %v", err)
	}

	var count int
	var locked sql.NullString
	_ = db.QueryRow(
		`SELECT failed_login_count, locked_until FROM doctors WHERE id='D1'`,
	).Scan(&count, &locked)
	if count != 0 {
		t.Errorf("failed_login_count: got %d, want 0", count)
	}
	if locked.Valid && locked.String != "" {
		t.Errorf("locked_until должен быть NULL")
	}
}

func TestIsDoctorLocked_NotLocked(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	locked, until, err := IsDoctorLocked(db, "D1")
	if err != nil {
		t.Fatalf("IsDoctorLocked: %v", err)
	}
	if locked {
		t.Fatalf("не должен быть заблокирован")
	}
	if !until.IsZero() {
		t.Errorf("until должен быть zero, got %v", until)
	}
}

func TestIsDoctorLocked_Expired(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")

	// Заблокирован в прошлом.
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	_, _ = db.Exec(`UPDATE doctors SET locked_until = ? WHERE id = 'D1'`, past)

	locked, _, err := IsDoctorLocked(db, "D1")
	if err != nil {
		t.Fatalf("IsDoctorLocked: %v", err)
	}
	if locked {
		t.Fatalf("протухшая блокировка не должна считаться активной")
	}
}

package main

// password_reset.go — подпатч 6.36 (B27.2): store-слой password reset
// + brute-force lock (B27).
//
// Схема (см. db.go):
//   password_reset_tokens (id, token_hash, doctor_id, created_at, expires_at, used_at)
//   doctors (+ must_change_password, password_changed_at, failed_login_count, locked_until)
//
// Время хранится в RFC3339 (как refresh_tokens) — SQLite корректно
// сравнивает с datetime('now').

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	// PasswordResetTokenTTL — время жизни токена сброса пароля.
	PasswordResetTokenTTL = 1 * time.Hour
	// MaxFailedLogins — после скольких неудач блокируем врача.
	MaxFailedLogins = 5
	// DoctorLockDuration — на сколько блокируем после MaxFailedLogins.
	DoctorLockDuration = 15 * time.Minute
)

// ErrPasswordResetTokenInvalid — токен не найден, протух или уже использован.
var ErrPasswordResetTokenInvalid = errors.New("password reset token invalid")

// PasswordResetRow — запись из password_reset_tokens.
type PasswordResetRow struct {
	ID        int64
	TokenHash string
	DoctorID  string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    sql.NullTime
}

// ---------------------------------------------------------------
// Password reset tokens
// ---------------------------------------------------------------

// CreatePasswordResetToken создаёт токен для doctorID.
// Принимает УЖЕ захэшированный tokenHash (sha256 hex).
// Очистка протухших — отдельной функцией DeleteExpiredPasswordResetTokens
// (вызывается из retention-цикла, см. B27.5).
func CreatePasswordResetToken(db *sql.DB, doctorID, tokenHash string) error {
	if doctorID == "" || tokenHash == "" {
		return fmt.Errorf("empty doctorID or tokenHash")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(PasswordResetTokenTTL)
	_, err := db.Exec(
		`INSERT INTO password_reset_tokens (token_hash, doctor_id, created_at, expires_at, used_at)
		 VALUES (?, ?, ?, ?, NULL)`,
		tokenHash, doctorID,
		now.Format(time.RFC3339),
		expiresAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert password_reset_tokens: %w", err)
	}
	return nil
}

// GetPasswordResetToken возвращает валидный токен по хешу.
// Возвращает ErrPasswordResetTokenInvalid, если токен не найден,
// протух или уже использован.
func GetPasswordResetToken(db *sql.DB, tokenHash string) (*PasswordResetRow, error) {
	if tokenHash == "" {
		return nil, ErrPasswordResetTokenInvalid
	}
	var (
		row              PasswordResetRow
		createdAt, expAt string
		usedAt           sql.NullString
	)
	err := db.QueryRow(
		`SELECT id, token_hash, doctor_id, created_at, expires_at, used_at
		 FROM password_reset_tokens WHERE token_hash = ?`,
		tokenHash,
	).Scan(&row.ID, &row.TokenHash, &row.DoctorID, &createdAt, &expAt, &usedAt)
	if err == sql.ErrNoRows {
		return nil, ErrPasswordResetTokenInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("select password_reset_tokens: %w", err)
	}
	row.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	row.ExpiresAt, _ = time.Parse(time.RFC3339, expAt)
	if usedAt.Valid {
		row.UsedAt.Valid = true
		row.UsedAt.Time, _ = time.Parse(time.RFC3339, usedAt.String)
	}
	if row.UsedAt.Valid {
		return nil, ErrPasswordResetTokenInvalid
	}
	if !row.ExpiresAt.IsZero() && time.Now().After(row.ExpiresAt) {
		return nil, ErrPasswordResetTokenInvalid
	}
	return &row, nil
}

// ConsumePasswordResetToken помечает токен использованным.
// Идемпотентно: повторный вызов не падает.
func ConsumePasswordResetToken(db *sql.DB, tokenHash string) error {
	if tokenHash == "" {
		return fmt.Errorf("empty tokenHash")
	}
	_, err := db.Exec(
		`UPDATE password_reset_tokens SET used_at = ?
		 WHERE token_hash = ? AND used_at IS NULL`,
		time.Now().UTC().Format(time.RFC3339), tokenHash,
	)
	if err != nil {
		return fmt.Errorf("update password_reset_tokens: %w", err)
	}
	return nil
}

// DeleteExpiredPasswordResetTokens удаляет токены с expires_at < now.
// Возвращает количество удалённых.
func DeleteExpiredPasswordResetTokens(db *sql.DB) (int64, error) {
	res, err := db.Exec(
		`DELETE FROM password_reset_tokens WHERE expires_at < ?`,
		time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("delete password_reset_tokens: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ---------------------------------------------------------------
// Doctors: пароль + must_change_password
// ---------------------------------------------------------------

// UpdatePasswordHash устанавливает новый password_hash + password_changed_at,
// снимает must_change_password и сбрасывает failed_login_count/locked_until
// (успешная смена пароля = «разблокировка»).
func UpdatePasswordHash(db *sql.DB, doctorID, newHash string) error {
	if doctorID == "" || newHash == "" {
		return fmt.Errorf("empty doctorID or newHash")
	}
	_, err := db.Exec(
		`UPDATE doctors
		 SET password_hash = ?,
		     password_changed_at = ?,
		     must_change_password = 0,
		     failed_login_count = 0,
		     locked_until = NULL
		 WHERE id = ?`,
		newHash,
		time.Now().UTC().Format(time.RFC3339),
		doctorID,
	)
	if err != nil {
		return fmt.Errorf("update doctors.password_hash: %w", err)
	}
	return nil
}

// SetMustChangePassword устанавливает/снимает флаг must_change_password.
func SetMustChangePassword(db *sql.DB, doctorID string, value bool) error {
	if doctorID == "" {
		return fmt.Errorf("empty doctorID")
	}
	v := 0
	if value {
		v = 1
	}
	_, err := db.Exec(
		`UPDATE doctors SET must_change_password = ? WHERE id = ?`,
		v, doctorID,
	)
	if err != nil {
		return fmt.Errorf("update doctors.must_change_password: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------
// Brute-force lock
// ---------------------------------------------------------------

// IncrementFailedLogin увеличивает failed_login_count и при достижении
// MaxFailedLogins выставляет locked_until = now + DoctorLockDuration.
// Возвращает новый счётчик.
func IncrementFailedLogin(db *sql.DB, doctorID string) (int, error) {
	if doctorID == "" {
		return 0, fmt.Errorf("empty doctorID")
	}
	// Атомарный инкремент.
	res, err := db.Exec(
		`UPDATE doctors SET failed_login_count = failed_login_count + 1 WHERE id = ?`,
		doctorID,
	)
	if err != nil {
		return 0, fmt.Errorf("increment failed_login_count: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, sql.ErrNoRows
	}

	var count int
	if err := db.QueryRow(
		`SELECT failed_login_count FROM doctors WHERE id = ?`, doctorID,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("select failed_login_count: %w", err)
	}

	if count >= MaxFailedLogins {
		until := time.Now().UTC().Add(DoctorLockDuration)
		if err := LockDoctor(db, doctorID, until); err != nil {
			return count, err
		}
	}
	return count, nil
}

// LockDoctor выставляет locked_until (для ручной блокировки).
func LockDoctor(db *sql.DB, doctorID string, until time.Time) error {
	if doctorID == "" {
		return fmt.Errorf("empty doctorID")
	}
	_, err := db.Exec(
		`UPDATE doctors SET locked_until = ? WHERE id = ?`,
		until.UTC().Format(time.RFC3339), doctorID,
	)
	if err != nil {
		return fmt.Errorf("update locked_until: %w", err)
	}
	return nil
}

// ResetFailedLogin сбрасывает failed_login_count и locked_until
// (при успешном входе или смене пароля).
func ResetFailedLogin(db *sql.DB, doctorID string) error {
	if doctorID == "" {
		return fmt.Errorf("empty doctorID")
	}
	_, err := db.Exec(
		`UPDATE doctors SET failed_login_count = 0, locked_until = NULL WHERE id = ?`,
		doctorID,
	)
	if err != nil {
		return fmt.Errorf("reset failed_login: %w", err)
	}
	return nil
}

// IsDoctorLocked — true, если locked_until > now.
// Возвращает (locked, timeUntilUnlock, err); если не заблокирован, until = zero.
func IsDoctorLocked(db *sql.DB, doctorID string) (bool, time.Time, error) {
	if doctorID == "" {
		return false, time.Time{}, nil
	}
	var lockedUntil sql.NullString
	err := db.QueryRow(
		`SELECT locked_until FROM doctors WHERE id = ?`, doctorID,
	).Scan(&lockedUntil)
	if err == sql.ErrNoRows {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, fmt.Errorf("select locked_until: %w", err)
	}
	if !lockedUntil.Valid || lockedUntil.String == "" {
		return false, time.Time{}, nil
	}
	until, err := time.Parse(time.RFC3339, lockedUntil.String)
	if err != nil {
		return false, time.Time{}, fmt.Errorf("parse locked_until: %w", err)
	}
	if time.Now().After(until) {
		return false, time.Time{}, nil
	}
	return true, until, nil
}

package main

// refresh.go — подпатч 6.13 (B4.1): store для refresh-токенов.
//
// Модель — «одноразовые refresh + blacklist jti»:
//   * при выдаче refresh-токена его jti сохраняется в refresh_tokens;
//   * при ротации старый jti помечается revoked_at + replaced_by;
//   * если приходит УЖЕ отозванный jti — это replay → каскад:
//     все активные refresh врача сжигаются (RevokeAllForDoctor);
//   * logout отзывает jti.
//
// Все функции работают с уже открытым *sql.DB.

import (
	"database/sql"
	"fmt"
	"time"
)

// SaveRefreshJTI записывает выданный refresh-jti.
// Заодно чистит истёкшие записи (lazy-cleanup).
func SaveRefreshJTI(db *sql.DB, jti, doctorID string, expiresAt time.Time) error {
	if jti == "" || doctorID == "" {
		return fmt.Errorf("empty jti or doctorID")
	}

	if _, err := db.Exec(
		`DELETE FROM refresh_tokens WHERE expires_at < datetime('now')`,
	); err != nil {
		_ = err
	}

	_, err := db.Exec(
		`INSERT INTO refresh_tokens (jti, doctor_id, issued_at, expires_at, revoked_at, replaced_by)
		 VALUES (?, ?, datetime('now'), ?, NULL, NULL)`,
		jti, doctorID, expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("insert refresh_tokens: %w", err)
	}
	return nil
}

// refreshCleanup удаляет строки refresh_tokens, у которых expires_at
// старше N дней (B23, retention). Формат cutoff — как в auditCleanup
// (совместим с SQLite datetime('now')).
//
// Логика: days <= 0 — ошибка (вызывающий должен проверять).
// Не удаляет active-токены: cutoff гарантирует, что запись УЖЕ протухла
// и лежит минимум `days` дней.
func refreshCleanup(db *sql.DB, days int) (int64, error) {
	if days <= 0 {
		return 0, fmt.Errorf("days must be > 0")
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	res, err := db.Exec(`DELETE FROM refresh_tokens WHERE expires_at < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("refresh cleanup: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// IsRefreshRevoked — true, если jti неизвестен ИЛИ revoked_at уже выставлен.
func IsRefreshRevoked(db *sql.DB, jti string) (bool, error) {
	if jti == "" {
		return true, nil
	}
	var revokedAt sql.NullString
	err := db.QueryRow(
		`SELECT revoked_at FROM refresh_tokens WHERE jti = ?`, jti,
	).Scan(&revokedAt)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("select refresh_tokens: %w", err)
	}
	return revokedAt.Valid, nil
}

// RevokeRefreshJTI помечает jti отозванным. replacedBy — jti нового токена.
func RevokeRefreshJTI(db *sql.DB, jti, replacedBy string) error {
	if jti == "" {
		return fmt.Errorf("empty jti")
	}
	_, err := db.Exec(
		`UPDATE refresh_tokens
		 SET revoked_at = datetime('now'), replaced_by = ?
		 WHERE jti = ? AND revoked_at IS NULL`,
		replacedBy, jti,
	)
	if err != nil {
		return fmt.Errorf("update refresh_tokens: %w", err)
	}
	return nil
}

// RevokeAllForDoctor — каскад: помечает отозванными все активные refresh врача.
func RevokeAllForDoctor(db *sql.DB, doctorID string) (int64, error) {
	if doctorID == "" {
		return 0, fmt.Errorf("empty doctorID")
	}
	res, err := db.Exec(
		`UPDATE refresh_tokens
		 SET revoked_at = datetime('now')
		 WHERE doctor_id = ? AND revoked_at IS NULL`,
		doctorID,
	)
	if err != nil {
		return 0, fmt.Errorf("revoke all: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

package main

// password_reset_test.go — подпатч 6.36 (B27.2): тесты store password reset
// + brute-force lock.
//
// Переиспользует newTestDB(t), seedDoctorForRefresh(t, db, id) из refresh_test.go.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ---------------------------------------------------------------
// B27.3a — HTTP API password reset (request + confirm)
// ---------------------------------------------------------------

// withDevEnv — временно ставит currentEnv в dev (для тестов).
func withDevEnv(t *testing.T) {
	t.Helper()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{
		Name:      "dev",
		IsProd:    false,
		JWTSecret: "b27-3a-test-secret-32-bytes-minimum-abc",
	}
}

// TestPasswordResetRequest_Success — есть активный доктор.
func TestPasswordResetRequest_Success(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)
	withDevEnv(t)

	body := strings.NewReader(`{"email":"d1@test.local"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/request", body)
	w := httptest.NewRecorder()

	handlePasswordResetRequest(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("ok != true: %v", resp)
	}
	tok, ok := resp["token"].(string)
	if !ok || tok == "" {
		t.Fatalf("в dev ожидали token в ответе, got %v", resp)
	}

	var count int
	_ = db.QueryRow(`SELECT COUNT(*) FROM password_reset_tokens WHERE doctor_id='D1'`).Scan(&count)
	if count != 1 {
		t.Fatalf("токен не сохранён, count=%d", count)
	}
}

// TestPasswordResetRequest_NotFound — email не найден, всё равно 200.
func TestPasswordResetRequest_NotFound(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	withDevEnv(t)

	body := strings.NewReader(`{"email":"nobody@test.local"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/request", body)
	w := httptest.NewRecorder()

	handlePasswordResetRequest(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Fatalf("ok != true: %v", resp)
	}
	if _, hasTok := resp["token"]; hasTok {
		t.Fatalf("токен не должен возвращаться, если email не найден")
	}
}

// TestPasswordResetConfirm_Success — полный happy-path.
func TestPasswordResetConfirm_Success(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)
	withDevEnv(t)

	body := strings.NewReader(`{"email":"d1@test.local"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/request", body)
	w := httptest.NewRecorder()
	handlePasswordResetRequest(w, r)

	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	tok, _ := resp["token"].(string)
	if tok == "" {
		t.Fatalf("токен пустой")
	}

	body2 := strings.NewReader(`{"token":"` + tok + `","new_password":"new-password-123"}`)
	r2 := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/confirm", body2)
	w2 := httptest.NewRecorder()
	handlePasswordResetConfirm(w2, r2)

	if w2.Code != http.StatusOK {
		t.Fatalf("confirm code: got %d, body=%s", w2.Code, w2.Body.String())
	}

	doc, err := getDoctorByEmail(db, "d1@test.local")
	if err != nil {
		t.Fatalf("getDoctorByEmail: %v", err)
	}
	if err := CheckPassword(doc.PasswordHash, "new-password-123"); err != nil {
		t.Fatalf("новый пароль не работает: %v", err)
	}

	var usedAt sql.NullString
	_ = db.QueryRow(`SELECT used_at FROM password_reset_tokens WHERE doctor_id='D1'`).Scan(&usedAt)
	if !usedAt.Valid {
		t.Fatalf("used_at должен быть установлен")
	}
}

// TestPasswordResetConfirm_InvalidToken — неверный токен.
func TestPasswordResetConfirm_InvalidToken(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	withDevEnv(t)

	body := strings.NewReader(`{"token":"deadbeef","new_password":"valid-password-123"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/confirm", body)
	w := httptest.NewRecorder()
	handlePasswordResetConfirm(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, want 400", w.Code)
	}
}

// TestPasswordResetConfirm_WeakPassword — пароль < minPasswordLen.
func TestPasswordResetConfirm_WeakPassword(t *testing.T) {
	db := newTestDB(t)
	withGlobalDB(t, db)
	withDevEnv(t)

	body := strings.NewReader(`{"token":"abc","new_password":"123"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/confirm", body)
	w := httptest.NewRecorder()
	handlePasswordResetConfirm(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("code: got %d, want 400", w.Code)
	}
}

// TestPasswordResetConfirm_RevokesRefresh — при смене пароля отзываем refresh.
func TestPasswordResetConfirm_RevokesRefresh(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)
	withDevEnv(t)

	if err := SaveRefreshJTI(db, "jti-1", "D1", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveRefreshJTI: %v", err)
	}

	body := strings.NewReader(`{"email":"d1@test.local"}`)
	w := httptest.NewRecorder()
	handlePasswordResetRequest(w, httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/request", body))
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	tok, _ := resp["token"].(string)

	body2 := strings.NewReader(`{"token":"` + tok + `","new_password":"new-password-456"}`)
	w2 := httptest.NewRecorder()
	handlePasswordResetConfirm(w2, httptest.NewRequest(http.MethodPost, "/api/auth/password-reset/confirm", body2))

	if w2.Code != http.StatusOK {
		t.Fatalf("confirm code: got %d, body=%s", w2.Code, w2.Body.String())
	}

	var revokedAt sql.NullString
	_ = db.QueryRow(`SELECT revoked_at FROM refresh_tokens WHERE jti='jti-1'`).Scan(&revokedAt)
	if !revokedAt.Valid {
		t.Fatalf("refresh-токен должен быть отозван после reset")
	}
}

// TestPasswordResetRequest_MethodNotAllowed — GET → 405.
func TestPasswordResetRequest_MethodNotAllowed(t *testing.T) {
	withDevEnv(t)
	w := httptest.NewRecorder()
	handlePasswordResetRequest(w, httptest.NewRequest(http.MethodGet, "/api/auth/password-reset/request", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: got %d, want 405", w.Code)
	}
}

// ---------------------------------------------------------------
// B27.4 — change-password + admin reset + middleware
// ---------------------------------------------------------------

// TestChangePassword_Success — happy path.
func TestChangePassword_Success(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)

	// Проставим реальный bcrypt-хеш (seedDoctorForRefresh ставит 'x').
	oldHash, _ := HashPassword("old-password-1")
	_, _ = db.Exec(`UPDATE doctors SET password_hash = ? WHERE id='D1'`, oldHash)

	body := strings.NewReader(`{"old_password":"old-password-1","new_password":"new-password-2"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", body)
	ctx := withDoctorID(r.Context(), "D1", RoleDoctor)
	r = r.WithContext(ctx)
	w := httptest.NewRecorder()

	handleChangePassword(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	// Проверим, что новый пароль работает.
	var h string
	_ = db.QueryRow(`SELECT password_hash FROM doctors WHERE id='D1'`).Scan(&h)
	if err := CheckPassword(h, "new-password-2"); err != nil {
		t.Fatalf("новый пароль не работает: %v", err)
	}
}

// TestChangePassword_WrongOld — неверный старый пароль.
func TestChangePassword_WrongOld(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)

	oldHash, _ := HashPassword("old-password-1")
	_, _ = db.Exec(`UPDATE doctors SET password_hash = ? WHERE id='D1'`, oldHash)

	body := strings.NewReader(`{"old_password":"WRONG","new_password":"new-password-2"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", body)
	r = r.WithContext(withDoctorID(r.Context(), "D1", RoleDoctor))
	w := httptest.NewRecorder()

	handleChangePassword(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("code: got %d, want 401", w.Code)
	}
}

// TestChangePassword_ClearsMustChange — флаг снимается.
func TestChangePassword_ClearsMustChange(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)

	oldHash, _ := HashPassword("old-password-1")
	_, _ = db.Exec(`UPDATE doctors SET password_hash = ?, must_change_password = 1 WHERE id='D1'`, oldHash)

	body := strings.NewReader(`{"old_password":"old-password-1","new_password":"new-password-3"}`)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", body)
	r = r.WithContext(withDoctorID(r.Context(), "D1", RoleDoctor))
	w := httptest.NewRecorder()

	handleChangePassword(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var v int
	_ = db.QueryRow(`SELECT must_change_password FROM doctors WHERE id='D1'`).Scan(&v)
	if v != 0 {
		t.Fatalf("must_change_password должен быть снят, got %d", v)
	}
}

// TestMiddleware_BlocksWhenMustChange — 403 с флагом.
func TestMiddleware_BlocksWhenMustChange(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)
	_, _ = db.Exec(`UPDATE doctors SET must_change_password = 1 WHERE id='D1'`)

	called := false
	h := requirePasswordChanged(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	r := httptest.NewRequest(http.MethodGet, "/api/patients", nil)
	r = r.WithContext(withDoctorID(r.Context(), "D1", RoleDoctor))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if called {
		t.Fatalf("next не должен вызываться при must_change_password=1")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("code: got %d, want 403", w.Code)
	}
}

// TestMiddleware_AllowsChangePassword — whitelist пропускает.
func TestMiddleware_AllowsChangePassword(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)
	_, _ = db.Exec(`UPDATE doctors SET must_change_password = 1 WHERE id='D1'`)

	called := false
	h := requirePasswordChanged(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	r := httptest.NewRequest(http.MethodPost, "/api/auth/change-password", nil)
	r = r.WithContext(withDoctorID(r.Context(), "D1", RoleDoctor))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !called {
		t.Fatalf("next должен вызываться для /api/auth/change-password")
	}
}

// TestAdminResetPassword_Success — temp password + флаг.
func TestAdminResetPassword_Success(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	withGlobalDB(t, db)

	body := strings.NewReader(``)
	r := httptest.NewRequest(http.MethodPost, "/api/doctors/D1/reset-password", body)
	r = r.WithContext(withDoctorID(r.Context(), "ADMIN-1", RoleAdmin))
	w := httptest.NewRecorder()

	handleDoctorResetPassword(w, r, "D1")

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, body=%s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	tmp, _ := resp["temporary_password"].(string)
	if tmp == "" {
		t.Fatalf("temporary_password пустой: %v", resp)
	}

	// Проверим, что пароль работает + флаг установлен.
	var h string
	var mustChange int
	_ = db.QueryRow(`SELECT password_hash, must_change_password FROM doctors WHERE id='D1'`).Scan(&h, &mustChange)
	if err := CheckPassword(h, tmp); err != nil {
		t.Fatalf("temp пароль не работает: %v", err)
	}
	if mustChange != 1 {
		t.Fatalf("must_change_password должен быть =1")
	}
}

// TestAdminResetPassword_CannotResetSuperadmin — admin не может сбросить superadmin.
func TestAdminResetPassword_CannotResetSuperadmin(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	// Создадим superadmin'а.
	_, _ = db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, created, active)
		 VALUES ('SUPER-1', NULL, 'super@test.local', 'x', 'Super', 'lor', 'superadmin', datetime('now'), 1)`,
	)
	withGlobalDB(t, db)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/doctors/SUPER-1/reset-password", nil)
	r = r.WithContext(withDoctorID(r.Context(), "ADMIN-1", RoleAdmin))

	handleDoctorResetPassword(w, r, "SUPER-1")

	if w.Code != http.StatusForbidden {
		t.Fatalf("code: got %d, want 403", w.Code)
	}
}

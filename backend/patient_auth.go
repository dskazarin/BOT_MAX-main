package main

// patient_auth.go — B30.2: auth-контур пациента.
//
// Модель:
//   Пациент — самостоятельный субъект. У него свой аккаунт
//   (patient_accounts) со своим логином/паролем. JWT переиспользует
//   Claims/AccessTTL/RefreshTTL, но:
//     - Role     = RolePatient
//     - Claims.DoctorID = patientAccountID (поле исторически называется
//       "doctorID", семантически здесь это subjectID; в JWT лежит как sub)
//     - cookies: botmax_pat_access / botmax_pat_refresh
//     - refresh хранится в patient_refresh_tokens (не refresh_tokens,
//       у той FK на doctors.id)
//
// Что НЕ делает этот файл (сознательно):
//   - Регистрация пациента — патч 2b (требует миграции
//     patients.doctor_id → NULL).
//   - Consent-флоу — патч 3 (расширение consents + patient_invites).
//   - Запись в audit_log — это врачебный журнал; auth-события пациента
//     пишем только в slog. Аудит пациента (если понадобится) — отдельная
//     таблица patient_audit в будущем.
//
// patientAuthMiddleware — hard-mode всегда: soft-режим для пациента
// недопустим (иначе можно читать чужие данные без токена).

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// nowUTC — единый формат времени для patient_*-таблиц.
// RFC3339 в UTC, как в refresh.go (совместимо с refresh_tokens).
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ---------------------------------------------------------------
// Context
// ---------------------------------------------------------------

const ctxKeyPatientID ctxKey = "patientID"

// withPatientID кладёт patientID в context.
func withPatientID(ctx context.Context, patientID string) context.Context {
	return context.WithValue(ctx, ctxKeyPatientID, patientID)
}

// patientIDFromCtx возвращает patientID из context (пусто, если нет).
// Не путать с doctorIDFromCtx — тот читает другой ключ.
func patientIDFromCtx(r *http.Request) string {
	v, _ := r.Context().Value(ctxKeyPatientID).(string)
	return v
}

// ---------------------------------------------------------------
// Модель patient_accounts
// ---------------------------------------------------------------

// PatientAccount — строка patient_accounts + денормализованное имя пациента.
type PatientAccount struct {
	ID           string
	PatientID    string
	Login        string
	PasswordHash string
	Status       string
	Created      string
	Updated      string
	LastLogin    sql.NullString
	PatientName  string // из patients.name (join)
}

// getPatientAccountByLogin ищет аккаунт по login (нормализованному).
func getPatientAccountByLogin(db *sql.DB, login string) (*PatientAccount, error) {
	const q = `
		SELECT pa.id, pa.patient_id, pa.login, pa.password_hash, pa.status,
		       pa.created, pa.updated, pa.last_login,
		       COALESCE(p.name, '')
		FROM patient_accounts pa
		LEFT JOIN patients p ON p.id = pa.patient_id
		WHERE pa.login = ?`
	var a PatientAccount
	err := db.QueryRow(q, login).Scan(
		&a.ID, &a.PatientID, &a.Login, &a.PasswordHash, &a.Status,
		&a.Created, &a.Updated, &a.LastLogin, &a.PatientName,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// getPatientAccountByID ищет аккаунт по id.
func getPatientAccountByID(db *sql.DB, id string) (*PatientAccount, error) {
	const q = `
		SELECT pa.id, pa.patient_id, pa.login, pa.password_hash, pa.status,
		       pa.created, pa.updated, pa.last_login,
		       COALESCE(p.name, '')
		FROM patient_accounts pa
		LEFT JOIN patients p ON p.id = pa.patient_id
		WHERE pa.id = ?`
	var a PatientAccount
	err := db.QueryRow(q, id).Scan(
		&a.ID, &a.PatientID, &a.Login, &a.PasswordHash, &a.Status,
		&a.Created, &a.Updated, &a.LastLogin, &a.PatientName,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// touchPatientLastLogin обновляет last_login и updated.
func touchPatientLastLogin(db *sql.DB, id string) error {
	_, err := db.Exec(
		`UPDATE patient_accounts SET last_login = ?, updated = ? WHERE id = ?`,
		nowUTC(), nowUTC(), id,
	)
	return err
}

// ---------------------------------------------------------------
// patient_refresh_tokens
// ---------------------------------------------------------------

// savePatientRefreshJTI сохраняет jti при выдаче refresh.
func savePatientRefreshJTI(db *sql.DB, jti, patientID string, expiresAt time.Time) error {
	_, err := db.Exec(
		`INSERT INTO patient_refresh_tokens (jti, patient_id, issued_at, expires_at)
		 VALUES (?, ?, ?, ?)`,
		jti, patientID, nowUTC(), expiresAt.UTC().Format(time.RFC3339),
	)
	return err
}

// isPatientRefreshRevoked — true, если jti отозван или уже не существует.
// Возвращает (false, nil), если jti есть и revoked_at IS NULL.
func isPatientRefreshRevoked(db *sql.DB, jti string) (bool, error) {
	var revokedAt sql.NullString
	err := db.QueryRow(
		`SELECT revoked_at FROM patient_refresh_tokens WHERE jti = ?`, jti,
	).Scan(&revokedAt)
	if err == sql.ErrNoRows {
		// jti не найден — считаем «отозван», это защита от подделок.
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return revokedAt.Valid, nil
}

// revokePatientRefreshJTI помечает jti отозванным.
func revokePatientRefreshJTI(db *sql.DB, jti, replacedBy string) error {
	_, err := db.Exec(
		`UPDATE patient_refresh_tokens SET revoked_at = ?, replaced_by = ?
		 WHERE jti = ? AND revoked_at IS NULL`,
		nowUTC(), replacedBy, jti,
	)
	return err
}

// revokeAllPatientRefresh — каскадный отзыв (при replay-атаке).
func revokeAllPatientRefresh(db *sql.DB, patientID string) (int64, error) {
	res, err := db.Exec(
		`UPDATE patient_refresh_tokens SET revoked_at = ?
		 WHERE patient_id = ? AND revoked_at IS NULL`,
		nowUTC(), patientID,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------

type patientLoginReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type patientBrief struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Login string `json:"login"`
}

type patientLoginResp struct {
	AccessToken  string       `json:"accessToken"`
	RefreshToken string       `json:"refreshToken"`
	Patient      patientBrief `json:"patient"`
}

// normalizeLogin — trim + lower. Email/телефон к одному виду.
func normalizeLogin(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// handlePatientLogin — POST /api/patient-auth/login.
// Тело: {"login":"...","password":"..."}.
// 200: {"accessToken":"...","refreshToken":"...","patient":{...}}.
// 401: неверные креды (в т.ч. нет такого login).
// 400: битый JSON или пустые поля.
//
// Защита от user enumeration: bcrypt-проверка выполняется всегда,
// даже если login не найден (hash = "").
func handlePatientLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req patientLoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Login = normalizeLogin(req.Login)
	if req.Login == "" || req.Password == "" {
		http.Error(w, `{"error":"login and password required"}`, http.StatusBadRequest)
		return
	}

	acc, err := getPatientAccountByLogin(db, req.Login)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Warn("patient login: getPatientAccountByLogin", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Всегда вызываем CheckPassword — даже если acc == nil (тайминг).
	var hash string
	if acc != nil {
		hash = acc.PasswordHash
	}
	if err := CheckPassword(hash, req.Password); err != nil || acc == nil {
		slog.Info("patient login: invalid creds", "login", req.Login, "ip", clientIP(r))
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	if acc.Status != "active" {
		slog.Info("patient login: account not active", "id", acc.ID, "status", acc.Status)
		http.Error(w, `{"error":"account not active"}`, http.StatusForbidden)
		return
	}

	access, err := IssueAccessToken(acc.ID, RolePatient)
	if err != nil {
		slog.Warn("patient login: IssueAccessToken", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	refresh, jti, exp, err := IssueRefreshToken(acc.ID, RolePatient)
	if err != nil {
		slog.Warn("patient login: IssueRefreshToken", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if err := savePatientRefreshJTI(db, jti, acc.ID, exp); err != nil {
		slog.Warn("patient login: savePatientRefreshJTI", "err", err)
		// не фатально — access уже выдан
	}
	if err := touchPatientLastLogin(db, acc.ID); err != nil {
		slog.Warn("patient login: touchPatientLastLogin", "err", err)
	}

	// B30.6i: patient-login гасит doctor-сессию (конфликт ролей в одном браузере).
	clearAuthCookies(w)
	setPatientCookies(w, access, refresh)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(patientLoginResp{
		AccessToken:  access,
		RefreshToken: refresh,
		Patient: patientBrief{
			ID:    acc.ID,
			Name:  acc.PatientName,
			Login: acc.Login,
		},
	})
	slog.Info("patient login", "id", acc.ID, "login", acc.Login)
}

type patientRefreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

// handlePatientRefresh — POST /api/patient-auth/refresh.
// handlePatientDevHash — POST /api/patient-auth/dev-hash.
// Только для dev (IsProd=false). Возвращает bcrypt-хеш переданного пароля.
// Используется smoke-скриптом (smoke_b30_5.sh) для seed'а временного
// patient_accounts. В prod отдаёт 404.
// Тело: {"password":"..."} -> {"hash":"$2a$10$..."}.
func handlePatientDevHash(w http.ResponseWriter, r *http.Request) {
	if currentEnv == nil || currentEnv.IsProd {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Password == "" {
		http.Error(w, `{"error":"invalid json or empty password"}`, http.StatusBadRequest)
		return
	}
	h, err := HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"hash failed"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"hash": h})
}

func handlePatientRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req patientRefreshReq
	_ = json.NewDecoder(r.Body).Decode(&req) // best-effort: cookie-режим
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieNamePatientRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}
	if req.RefreshToken == "" {
		http.Error(w, `{"error":"refreshToken required"}`, http.StatusUnauthorized)
		return
	}

	claims, err := ParseToken(req.RefreshToken, "refresh")
	if err != nil {
		slog.Warn("patient refresh: invalid token", "err", err)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}
	if claims.Role != RolePatient {
		slog.Warn("patient refresh: wrong role", "role", claims.Role)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	revoked, err := isPatientRefreshRevoked(db, claims.ID)
	if err != nil {
		slog.Warn("patient refresh: isPatientRefreshRevoked", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if revoked {
		n, _ := revokeAllPatientRefresh(db, claims.DoctorID)
		slog.Error("patient refresh: REPLAY, cascade", "jti", claims.ID, "patient", claims.DoctorID, "revoked", n)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	acc, err := getPatientAccountByID(db, claims.DoctorID)
	if err != nil {
		slog.Warn("patient refresh: getPatientAccountByID", "id", claims.DoctorID, "err", err)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}
	if acc.Status != "active" {
		slog.Warn("patient refresh: account not active", "id", acc.ID, "status", acc.Status)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	newAccess, err := IssueAccessToken(acc.ID, RolePatient)
	if err != nil {
		slog.Warn("patient refresh: IssueAccessToken", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	newRefresh, newJTI, newExp, err := IssueRefreshToken(acc.ID, RolePatient)
	if err != nil {
		slog.Warn("patient refresh: IssueRefreshToken", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if err := savePatientRefreshJTI(db, newJTI, acc.ID, newExp); err != nil {
		slog.Warn("patient refresh: savePatientRefreshJTI", "err", err)
	}
	if err := revokePatientRefreshJTI(db, claims.ID, newJTI); err != nil {
		slog.Warn("patient refresh: revokePatientRefreshJTI", "jti", claims.ID, "err", err)
	}

	setPatientCookies(w, newAccess, newRefresh)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(patientLoginResp{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		Patient: patientBrief{
			ID:    acc.ID,
			Name:  acc.PatientName,
			Login: acc.Login,
		},
	})
	slog.Info("patient refresh", "id", acc.ID)
}

// handlePatientLogout — POST /api/patient-auth/logout.
// Идемпотентен: всегда 200.
func handlePatientLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req patientRefreshReq
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieNamePatientRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}
	if req.RefreshToken != "" {
		if claims, err := ParseToken(req.RefreshToken, "refresh"); err == nil {
			if err := revokePatientRefreshJTI(db, claims.ID, ""); err != nil {
				slog.Warn("patient logout: revoke", "jti", claims.ID, "err", err)
			}
		}
	}
	clearPatientCookies(w)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

// ---------------------------------------------------------------
// Middleware
// ---------------------------------------------------------------

// patientAuthWhitelist — пути, не требующие patient-токена.
func patientAuthWhitelist(path string) bool {
	switch path {
	case "/api/patient-auth/login",
		"/api/patient-auth/refresh",
		"/api/patient-auth/logout":
		return true
	}
	return false
}

// extractPatientAccessToken — берёт токен из cookie botmax_pat_access.
// (Bearer fallback — не поддерживаем: пациент всегда ходит через браузер.)
func extractPatientAccessToken(r *http.Request) string {
	if c, err := r.Cookie(cookieNamePatientAccess); err == nil {
		return c.Value
	}
	return ""
}

// patientAuthMiddleware — hard-mode. Soft-режим для пациента недопустим.
func patientAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if patientAuthWhitelist(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		token := extractPatientAccessToken(r)
		if token == "" {
			slog.Warn("patient auth: no token", "method", r.Method, "path", r.URL.Path)
			http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
			return
		}
		claims, err := ParseToken(token, "access")
		if err != nil {
			slog.Warn("patient auth: invalid token", "err", err)
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		if claims.Role != RolePatient {
			slog.Warn("patient auth: wrong role", "role", claims.Role)
			http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
			return
		}
		// Claims.DoctorID == patientAccountID (см. комментарий в шапке файла).
		ctx := withPatientID(r.Context(), claims.DoctorID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------------------------------------------------------------
// Routes
// ---------------------------------------------------------------

func registerPatientAuthRoutes() {
	http.HandleFunc("/api/patient-auth/login", handlePatientLogin)
	http.HandleFunc("/api/patient-auth/dev-hash", handlePatientDevHash)
	http.HandleFunc("/api/patient-auth/refresh", handlePatientRefresh)
	http.HandleFunc("/api/patient-auth/logout", handlePatientLogout)
	slog.Info("registerPatientAuthRoutes", "routes", "/login, /refresh, /logout")
}

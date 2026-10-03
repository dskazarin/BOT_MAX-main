package main

// auth.go — Шаг 6, подпатч 6.1: bcrypt + JWT-хелперы.
// Регистрация маршрутов /api/auth/* появится в подпатчах 6.3 и 6.6.
// authMiddleware и doctorIDFromCtx — в подпатче 6.4.

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------------
// Роли (Шаг 6.8)
// ---------------------------------------------------------------
//
// Роли — строковый enum. Значения совпадают с doctors.role в БД
// и с полем role в JWT-claims.
//
// Иерархия НЕ зашита в requireRole — вызывающий явно перечисляет
// допустимые роли.
type Role string

const (
	RoleSuperadmin Role = "superadmin"
	RoleAdmin      Role = "admin"
	RoleDoctor     Role = "doctor"
	// RoleRegistrar — медицинский регистратор (B27).
	// Используется модулем sick_leave для оформления направлений на ЭЛН.
	RoleRegistrar Role = "registrar"
)

// Valid — true, если роль известна.
func (r Role) Valid() bool {
	switch r {
	case RoleSuperadmin, RoleAdmin, RoleDoctor, RoleRegistrar:
		return true
	}
	return false
}

// ---------------------------------------------------------------
// Константы
// ---------------------------------------------------------------

const (
	// AccessTTL — время жизни access-токена.
	AccessTTL = 15 * time.Minute
	// RefreshTTL — время жизни refresh-токена.
	RefreshTTL = 7 * 24 * time.Hour
	// BcryptCost — стоимость bcrypt (дефолт 10).
	BcryptCost = bcrypt.DefaultCost
	// ctxKeyDoctorID — ключ для context.Context (unexported, чтобы никто
	// из других пакетов случайно не перезаписал).
	ctxKeyDoctorID ctxKey = "doctorID"
	// ctxKeyDoctorRole — ключ роли.
	ctxKeyDoctorRole ctxKey = "doctorRole"
	// ctxKeyActorID — ключ actor_id (B20, impersonation).
	// Пусто в обычной сессии, задан в impersonation.
	ctxKeyActorID ctxKey = "actorID"
	// ctxKeyActorRole — ключ роли actor'а (B20, impersonation).
	ctxKeyActorRole ctxKey = "actorRole"
	// ctxKeyTokenExp — Unix-время истечения access-токена (B21.2).
	// Нужно, чтобы UI мог показать countdown до конца impersonation.
	ctxKeyTokenExp ctxKey = "tokenExp"
)

type ctxKey string

// ---------------------------------------------------------------
// Пароли
// ---------------------------------------------------------------

// HashPassword возвращает bcrypt-хэш пароля.
func HashPassword(password string) (string, error) {
	if password == "" {
		return "", errors.New("empty password")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("bcrypt hash: %w", err)
	}
	return string(b), nil
}

// CheckPassword сравнивает пароль с bcrypt-хэшем.
// Возвращает nil, если пароль верный.
func CheckPassword(hash, password string) error {
	if hash == "" || password == "" {
		return errors.New("empty hash or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return fmt.Errorf("bcrypt compare: %w", err)
	}
	return nil
}

// IsBcryptHash — эвристика: строка похожа на bcrypt-хэш ($2a$/$2b$/$2y$).
// Нужна для идемпотентности seedDefaultDoctor.
func IsBcryptHash(s string) bool {
	return strings.HasPrefix(s, "$2a$") ||
		strings.HasPrefix(s, "$2b$") ||
		strings.HasPrefix(s, "$2y$")
}

// ---------------------------------------------------------------
// JWT
// ---------------------------------------------------------------

// Claims — полезная нагрузка токена.
//
// B20 (impersonation): при обычном логине поля ActorID/ActorRole/ActAs
// пустые. При impersonation-токене:
//   - DoctorID = целевой доктор (sub, "за кого" работаем)
//   - Role     = всегда RoleDoctor (запрет эскалации на уровне токена)
//   - ActorID  = кто выполняет impersonation (superadmin)
//   - ActorRole= роль actor'а на момент выдачи (нужна для /stop: перевыдать
//     actor'у его исходный access-токен)
//   - ActAs    = дублирует целевой doctorID (явный маркер impersonation)
type Claims struct {
	DoctorID  string `json:"sub"`
	Role      Role   `json:"role"`
	Type      string `json:"typ"` // "access" | "refresh"
	ActorID   string `json:"actor_id,omitempty"`
	ActorRole Role   `json:"actor_role,omitempty"`
	ActAs     string `json:"act_as,omitempty"`
	jwt.RegisteredClaims
}

// jwtSecret возвращает секрет из env BOTMAX_JWT_SECRET.
// Если не задан — dev-секрет + предупреждение (один раз).
var jwtSecretWarned bool

func jwtSecret() []byte {
	if currentEnv == nil {
		panic("jwtSecret: currentEnv == nil (validateEnv не вызван)")
	}
	if currentEnv.JWTSecret != "" {
		return []byte(currentEnv.JWTSecret)
	}
	if !jwtSecretWarned {
		slog.Warn("BOTMAX_JWT_SECRET не задан — dev-секрет, НЕ для прода")
		jwtSecretWarned = true
	}
	return []byte(devJWTSecret)
}

// newJTI — случайный идентификатор токена (jti).
func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// IssueAccessToken выдаёт access-токен для doctor.
func IssueAccessToken(doctorID string, role Role) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", fmt.Errorf("jti: %w", err)
	}
	now := time.Now()
	claims := Claims{
		DoctorID: doctorID,
		Role:     role,
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   doctorID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTTL)),
			Issuer:    "BOT_MAX",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(jwtSecret())
	if err != nil {
		return "", fmt.Errorf("sign access: %w", err)
	}
	return signed, nil
}

// impersonationTTL — TTL impersonation-access (B21).
// Читается из currentEnv.ImpersonationTTL; при currentEnv == nil
// (тесты без setupEnv) — fallback 15m.
func impersonationTTL() time.Duration {
	if currentEnv == nil || currentEnv.ImpersonationTTL <= 0 {
		return 15 * time.Minute
	}
	return currentEnv.ImpersonationTTL
}

// IssueImpersonationToken выдаёт access-токен «глазами доктора» (B20).
//
// Параметры:
//   - actorID       — кто выполняет impersonation (superadmin).
//   - actorRole     — роль actor'а (сохраняется в claim для /stop).
//   - targetDoctorID— кем прикидываемся.
//
// Возвращаемый токен:
//   - DoctorID  = targetDoctorID (sub) → весь существующий код, использующий
//     doctorIDFromCtx, продолжает работать без изменений.
//   - Role      = RoleDoctor (жёстко) → requireRole(RoleAdmin/Superadmin)
//     автоматически отсекает impersonated-сессию (запрет эскалации).
//   - ActorID/ActorRole/ActAs — заполнены; по ним authMiddleware/requireNonImpersonated
//     понимают, что сессия «чужая».
//   - Type      = "access", TTL = impersonationTTL().
//   - Refresh не выдаётся.
func IssueImpersonationToken(actorID string, actorRole Role, targetDoctorID string) (string, error) {
	if actorID == "" {
		return "", errors.New("impersonation: empty actorID")
	}
	if !actorRole.Valid() {
		return "", fmt.Errorf("impersonation: invalid actorRole %q", actorRole)
	}
	if targetDoctorID == "" {
		return "", errors.New("impersonation: empty targetDoctorID")
	}
	if actorID == targetDoctorID {
		return "", errors.New("impersonation: actorID == targetDoctorID")
	}

	jti, err := newJTI()
	if err != nil {
		return "", fmt.Errorf("jti: %w", err)
	}
	now := time.Now()
	claims := Claims{
		DoctorID:  targetDoctorID,
		Role:      RoleDoctor,
		Type:      "access",
		ActorID:   actorID,
		ActorRole: actorRole,
		ActAs:     targetDoctorID,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   targetDoctorID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(impersonationTTL())),
			Issuer:    "BOT_MAX",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(jwtSecret())
	if err != nil {
		return "", fmt.Errorf("sign impersonation: %w", err)
	}
	return signed, nil
}

// IssueRefreshToken выдаёт refresh-токен.
func IssueRefreshToken(doctorID string, role Role) (token string, jti string, expiresAt time.Time, err error) {
	jti, err = newJTI()
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("jti: %w", err)
	}
	now := time.Now()
	claims := Claims{
		DoctorID: doctorID,
		Role:     role,
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			Subject:   doctorID,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTTL)),
			Issuer:    "BOT_MAX",
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(jwtSecret())
	if err != nil {
		return "", "", time.Time{}, fmt.Errorf("sign refresh: %w", err)
	}
	return signed, jti, now.Add(RefreshTTL), nil
}

// ParseToken валидирует токен и возвращает claims.
// expectedType: "access" или "refresh"; если пусто — тип не проверяется.
func ParseToken(tokenString, expectedType string) (*Claims, error) {
	if tokenString == "" {
		return nil, errors.New("empty token")
	}
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	if !tok.Valid {
		return nil, errors.New("invalid token")
	}
	if expectedType != "" && claims.Type != expectedType {
		return nil, fmt.Errorf("wrong token type: got %q, want %q", claims.Type, expectedType)
	}
	if claims.DoctorID == "" {
		return nil, errors.New("empty sub in token")
	}
	return claims, nil
}

// ---------------------------------------------------------------
// Context helpers
// ---------------------------------------------------------------

// withDoctorID кладёт doctorID и role в context.
func withDoctorID(ctx context.Context, doctorID string, role Role) context.Context {
	ctx = context.WithValue(ctx, ctxKeyDoctorID, doctorID)
	ctx = context.WithValue(ctx, ctxKeyDoctorRole, role)
	return ctx
}

// withTokenExp кладёт Unix-время истечения access-токена в context (B21.2).
// unix <= 0 — no-op (нет данных, например в тестах).
func withTokenExp(ctx context.Context, unix int64) context.Context {
	if unix <= 0 {
		return ctx
	}
	return context.WithValue(ctx, ctxKeyTokenExp, unix)
}

// withActor кладёт actor_id/actor_role в context (B20, impersonation).
// no-op, если actorID == "" — чтобы не засорять обычные сессии.
func withActor(ctx context.Context, actorID string, actorRole Role) context.Context {
	if actorID == "" {
		return ctx
	}
	ctx = context.WithValue(ctx, ctxKeyActorID, actorID)
	ctx = context.WithValue(ctx, ctxKeyActorRole, actorRole)
	return ctx
}

// doctorIDFromCtx достаёт doctorID из context.
// Возвращает "" если не установлен.
func doctorIDFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyDoctorID).(string); ok {
		return v
	}
	return ""
}

// doctorRoleFromCtx достаёт role из context.
// Возвращает ("", false), если роль не установлена.
func doctorRoleFromCtx(r *http.Request) (Role, bool) {
	v, ok := r.Context().Value(ctxKeyDoctorRole).(Role)
	return v, ok
}

// actorIDFromCtx достаёт actor_id из context (B20, impersonation).
// В обычной сессии возвращает ("", false).
func actorIDFromCtx(r *http.Request) (string, bool) {
	v, ok := r.Context().Value(ctxKeyActorID).(string)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

// actorRoleFromCtx достаёт actor_role из context (B20, impersonation).
// Возвращает ("", false), если actor не задан.
func actorRoleFromCtx(r *http.Request) (Role, bool) {
	v, ok := r.Context().Value(ctxKeyActorRole).(Role)
	if !ok || !v.Valid() {
		return "", false
	}
	return v, true
}

// tokenExpFromCtx достаёт Unix-время истечения access-токена (B21.2).
// Возвращает (0, false), если не задано.
func tokenExpFromCtx(r *http.Request) (int64, bool) {
	v, ok := r.Context().Value(ctxKeyTokenExp).(int64)
	if !ok || v <= 0 {
		return 0, false
	}
	return v, true
}

// ---------------------------------------------------------------
// registerAuthRoutes — заглушка (маршруты появятся в 6.3 и 6.6).
// ---------------------------------------------------------------

// registerAuthRoutes регистрирует маршруты аутентификации на
// http.DefaultServeMux. Вызывается из main() рядом с
// registerPatientRoutes() — до создания http.Server.
func registerAuthRoutes() {
	http.HandleFunc("/api/auth/login", handleAuthLogin)
	http.HandleFunc("/api/auth/refresh", handleAuthRefresh)
	http.HandleFunc("/api/auth/logout", handleAuthLogout)
	// B27.3a: password reset.
	http.HandleFunc("/api/auth/password-reset/request", handlePasswordResetRequest)
	http.HandleFunc("/api/auth/password-reset/confirm", handlePasswordResetConfirm)

	slog.Info("registerAuthRoutes",
		"routes", "/login, /refresh, /logout, /password-reset/request, /password-reset/confirm")
}

// ---------------------------------------------------------------
// Шаг 6.3 — POST /api/auth/login
// ---------------------------------------------------------------

// loginReq — тело запроса POST /api/auth/login.
type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// doctorBrief — публичная часть врача (без password_hash) в ответе.
type doctorBrief struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// loginResp — ответ 200 на успешный логин.
type loginResp struct {
	AccessToken  string      `json:"accessToken"`
	RefreshToken string      `json:"refreshToken"`
	Doctor       doctorBrief `json:"doctor"`
}

// handleAuthLogin обрабатывает POST /api/auth/login.
// Тело: {"email":"...","password":"..."}.
// 200: {"accessToken":"...","refreshToken":"...","doctor":{...}}.
// 401: неверные креды (в т.ч. нет такого email).
// 400: битый JSON. 500: внутренняя ошибка.
//
// Защита от user enumeration: bcrypt-проверка выполняется всегда —
// даже если email не найден (hash = ""). Так по таймингу нельзя
// отличить «нет юзера» от «неверный пароль».
func handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		http.Error(w, `{"error":"email and password required"}`, http.StatusBadRequest)
		return
	}

	// --- B3.2: rate-limit до bcrypt ---
	ipKey := "ip:" + clientIP(r)
	emailKey := "email:" + req.Email
	if ok, retry := loginLimiterPerIP.Allow(ipKey); !ok {
		writeTooManyRequests(w, retry)
		return
	}
	if ok, retry := loginLimiterPerEmail.Allow(emailKey); !ok {
		writeTooManyRequests(w, retry)
		return
	}

	doc, err := getDoctorByEmail(db, req.Email)
	var storedHash string
	switch {
	case err == nil:
		storedHash = doc.PasswordHash
	case errors.Is(err, sql.ErrNoRows):
		// Не нашли — оставляем пустой hash, bcrypt всё равно отработает.
		doc = nil
	default:
		slog.Warn("login: getDoctorByEmail", "email", req.Email, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Всегда вызываем bcrypt (защита от timing-атак).
	if err := CheckPassword(storedHash, req.Password); err != nil || doc == nil || !doc.Active {
		auditLog(db, "", "auth.login.fail", "auth", "", clientIP(r), map[string]string{"email": req.Email, "reason": "invalid"}) // B9.2
		slog.Warn("login отклонено", "email", req.Email)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	access, err := IssueAccessToken(doc.ID, Role(doc.Role))
	if err != nil {
		slog.Warn("login: IssueAccessToken", "id", doc.ID, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	refresh, refreshJTI, refreshExp, err := IssueRefreshToken(doc.ID, Role(doc.Role))
	if err != nil {
		slog.Warn("login: IssueRefreshToken", "id", doc.ID, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if err := SaveRefreshJTI(db, refreshJTI, doc.ID, refreshExp); err != nil {
		slog.Warn("login: SaveRefreshJTI", "id", doc.ID, "err", err)
	}

	if err := touchLastLogin(db, doc.ID); err != nil {
		slog.Warn("login: touchLastLogin", "id", doc.ID, "err", err) // не критично
	}

	// B7.2: ставим access+refresh cookie (в дополнение к JSON)
	setAuthCookies(w, access, refresh)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResp{
		AccessToken:  access,
		RefreshToken: refresh,
		Doctor: doctorBrief{
			ID:   doc.ID,
			Name: doc.Name,
			Role: doc.Role,
		},
	})
	// --- B3.2: успешный логин — сбрасываем счётчики ---
	loginLimiterPerIP.Reset(ipKey)
	loginLimiterPerEmail.Reset(emailKey)
	slog.Info("login", "id", doc.ID, "email", doc.Email, "role", doc.Role)
	auditLog(db, doc.ID, "auth.login", "auth", "", clientIP(r), map[string]string{"email": doc.Email, "role": doc.Role}) // B9.2
}

// ---------------------------------------------------------------
// Шаг 6.4 — authMiddleware (soft-mode через BOTMAX_AUTH_ENFORCE)
// ---------------------------------------------------------------

// ---------------------------------------------------------------
// B27.3a — password reset (request + confirm)
// ---------------------------------------------------------------

// passwordResetRequestReq — тело POST /api/auth/password-reset/request.
type passwordResetRequestReq struct {
	Email string `json:"email"`
}

// passwordResetConfirmReq — тело POST /api/auth/password-reset/confirm.
type passwordResetConfirmReq struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// minPasswordLen — минимальная длина пароля (B27.3a).
const minPasswordLen = 8

// handlePasswordResetRequest — POST /api/auth/password-reset/request.
//
// Всегда возвращает 200, независимо от того, найден ли email
// (защита от user enumeration).
//
// В prod — токен уходит через email (в будущем).
// В dev (currentEnv.IsProd=false) — токен возвращается в JSON.
func handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req passwordResetRequestReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		http.Error(w, `{"error":"email required"}`, http.StatusBadRequest)
		return
	}

	if ok, retry := loginLimiterPerEmail.Allow("reset:" + req.Email); !ok {
		writeTooManyRequests(w, retry)
		return
	}

	resp := map[string]any{"ok": true}

	doc, err := getDoctorByEmail(db, req.Email)
	switch {
	case err == nil && doc != nil && doc.Active:
		plain, hash, err := GenerateResetToken()
		if err != nil {
			slog.Error("password-reset: GenerateResetToken", "err", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if err := CreatePasswordResetToken(db, doc.ID, hash); err != nil {
			slog.Error("password-reset: CreatePasswordResetToken", "id", doc.ID, "err", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		auditLog(db, doc.ID, "auth.password.reset_requested", "auth", "",
			clientIP(r), map[string]string{"email": doc.Email})
		slog.Info("password-reset: токен выпущен (dev-логирование)",
			"doctor_id", doc.ID, "email", doc.Email,
			"token_prefix", plain[:8])
		if currentEnv != nil && !currentEnv.IsProd {
			resp["token"] = plain
			resp["dev_note"] = "токен возвращён, потому что IsProd=false"
		}

	case errors.Is(err, sql.ErrNoRows):
		auditLog(db, "", "auth.password.reset_requested", "auth", "",
			clientIP(r), map[string]string{"email": req.Email, "found": "false"})
		slog.Info("password-reset: email не найден", "email", req.Email)

	default:
		slog.Warn("password-reset: getDoctorByEmail", "email", req.Email, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handlePasswordResetConfirm — POST /api/auth/password-reset/confirm.
func handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req passwordResetConfirmReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.Token = strings.TrimSpace(req.Token)
	if req.Token == "" || req.NewPassword == "" {
		http.Error(w, `{"error":"token and new_password required"}`, http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		http.Error(w, `{"error":"password too short"}`, http.StatusBadRequest)
		return
	}

	hash := hashResetToken(req.Token)
	row, err := GetPasswordResetToken(db, hash)
	if err != nil {
		if errors.Is(err, ErrPasswordResetTokenInvalid) {
			http.Error(w, `{"error":"invalid or expired token"}`, http.StatusBadRequest)
			return
		}
		slog.Error("password-reset: GetPasswordResetToken", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	newHash, err := HashPassword(req.NewPassword)
	if err != nil {
		slog.Error("password-reset: HashPassword", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if err := UpdatePasswordHash(db, row.DoctorID, newHash); err != nil {
		slog.Error("password-reset: UpdatePasswordHash", "id", row.DoctorID, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if err := ConsumePasswordResetToken(db, hash); err != nil {
		slog.Warn("password-reset: ConsumePasswordResetToken", "err", err)
	}

	if n, err := RevokeAllRefreshForDoctor(db, row.DoctorID); err != nil {
		slog.Warn("password-reset: RevokeAllRefreshForDoctor", "err", err)
	} else {
		slog.Info("password-reset: refresh-токены отозваны", "doctor_id", row.DoctorID, "revoked", n)
	}

	auditLog(db, row.DoctorID, "auth.password.reset_completed", "auth", "",
		clientIP(r), nil)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// authMiddleware — Шаг 6.4.
// Soft-mode (default): пропускает всех, но логирует запросы без токена.
// Enforce-mode (BOTMAX_AUTH_ENFORCE=true|1|yes|on): без валидного
// access-токена отдаёт 401, кроме whitelist-путей.
// Валидный токен кладёт doctorID и role в context запроса.
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Whitelist — не требуют авторизации.
		if isAuthWhitelist(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		token := extractAccessToken(r)
		enforce := authEnforceEnabled()

		if token == "" {
			if enforce {
				slog.Warn("auth: нет токена", "method", r.Method, "path", r.URL.Path)
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}
			slog.Warn("auth: soft-mode, нет токена", "method", r.Method, "path", r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		claims, err := ParseToken(token, "access")
		if err != nil {
			if enforce {
				slog.Warn("auth: невалидный токен", "method", r.Method, "path", r.URL.Path, "err", err)
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			slog.Warn("auth: soft-mode, невалидный токен", "method", r.Method, "path", r.URL.Path, "err", err)
			next.ServeHTTP(w, r)
			return
		}

		// Токен валиден — кладём в context и пропускаем.
		ctx := withDoctorID(r.Context(), claims.DoctorID, claims.Role)
		// B20: если это impersonation-токен (actor_id задан) — кладём actor.
		ctx = withActor(ctx, claims.ActorID, claims.ActorRole)
		// B21.2: кладём exp access-токена — UI использует для countdown.
		if claims.ExpiresAt != nil {
			ctx = withTokenExp(ctx, claims.ExpiresAt.Unix())
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ---------------------------------------------------------------
// requireRole — Шаг 6.8
// ---------------------------------------------------------------
//
// requireRole возвращает middleware, пропускающий запрос дальше
// только если роль в context входит в список allowed.
//
// Паникует при регистрации роутов, если передана неизвестная
// роль или пустой список.
//
// Коды:
//   - 401 Unauthorized — роли нет в context (authMiddleware
//     не отработал или soft-mode без токена).
//   - 403 Forbidden — роль есть, но не входит в allowed.
//
// Пример:
//
//	requireRole(RoleAdmin, RoleSuperadmin)
func requireRole(allowed ...Role) func(http.Handler) http.Handler {
	if len(allowed) == 0 {
		panic("requireRole: empty allowed roles")
	}
	for _, r := range allowed {
		if !r.Valid() {
			panic(fmt.Sprintf("requireRole: invalid role %q", r))
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := doctorRoleFromCtx(r)
			if !ok {
				writeUnauthorized(w)
				return
			}
			for _, a := range allowed {
				if role == a {
					next.ServeHTTP(w, r)
					return
				}
			}
			writeForbidden(w)
		})
	}
}

// requireNonImpersonated — 403, если текущая сессия является impersonation
// (B20, Q6 «запрет эскалации»). Ставится на роуты CRUD докторов, /api/audit
// и любые будущие «опасные» группы: impersonated-сессия не должна менять
// роли/учётки и читать журнал действий.
//
// В обычной сессии (actor_id не задан) — пропускает без изменений.
func requireNonImpersonated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := actorIDFromCtx(r); ok {
			slog.Warn("impersonation: доступ к защищённому роуту запрещён",
				"method", r.Method, "path", r.URL.Path)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"forbidden: impersonation mode"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// extractBearerToken достаёт токен из заголовка Authorization: Bearer <token>.
// Схема "Bearer" регистронезависима (RFC 6750).
func extractBearerToken(r *http.Request) string {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// extractAccessToken — access-токен из Authorization: Bearer или из cookie.
// Порядок: Bearer → cookie. Bearer имеет приоритет (обратная совместимость
// со старым фронтом на localStorage).
func extractAccessToken(r *http.Request) string {
	if t := extractBearerToken(r); t != "" {
		return t
	}
	if c, err := r.Cookie(cookieNameAccess); err == nil {
		return c.Value
	}
	return ""
}

// isAuthWhitelist — пути, не требующие авторизации.
// Для /api/* используем строгое сравнение (не префикс), чтобы случайно
// не открыть /api/auth/loginXXX. Статика (не /api/*) — открыта.
func isAuthWhitelist(path string) bool {
	switch path {
	case "/api/health", "/api/metrics", "/api/auth/login", "/api/auth/refresh",
		"/api/auth/password-reset/request", "/api/auth/password-reset/confirm":
		return true
	}
	if !strings.HasPrefix(path, "/api/") {
		return true
	}
	return false
}

// authEnforceEnabled читает BOTMAX_AUTH_ENFORCE один раз.
// "1","true","yes","on" → включено. Всё остальное → soft-mode.
var (
	authEnforceOnce sync.Once
	authEnforceVal  bool
)

func authEnforceEnabled() bool {
	authEnforceOnce.Do(func() {
		if currentEnv == nil {
			panic("authEnforceEnabled: currentEnv == nil (validateEnv не вызван)")
		}
		authEnforceVal = currentEnv.AuthEnforce
		if authEnforceVal {
			slog.Info("auth: enforce включён")
		} else {
			slog.Warn("auth: soft-mode (BOTMAX_AUTH_ENFORCE не установлен)")
		}
	})
	return authEnforceVal
}

// ---------------------------------------------------------------
// Шаг 6.5 — resolveDoctorID (context sub → doctorID)
// ---------------------------------------------------------------

// resolveDoctorID — Шаг 6.5.
// Возвращает doctorID из context (JWT sub).
//   - Если токен валиден → реальный sub.
//   - Если токена нет и enforce выключен → fallback на stubDoctorID.
//   - Если enforce включён и токена нет → ("", false).
//
// Второй результат ok == false → handler должен отдать 401.
func resolveDoctorID(r *http.Request) (string, bool) {
	if id := doctorIDFromCtx(r); id != "" {
		return id, true
	}
	if authEnforceEnabled() {
		return "", false
	}
	return stubDoctorID, true
}

// writeUnauthorized — единый ответ 401 для handlers.
func writeUnauthorized(w http.ResponseWriter) {
	http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
}

// writeForbidden — 403. Симметрично writeUnauthorized.
func writeForbidden(w http.ResponseWriter) {
	http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
}

// ---------------------------------------------------------------
// Шаг 6.6 — POST /api/auth/refresh, POST /api/auth/logout
// ---------------------------------------------------------------

// refreshReq — тело POST /api/auth/refresh.
type refreshReq struct {
	RefreshToken string `json:"refreshToken"`
}

// handleAuthRefresh — POST /api/auth/refresh.
// Принимает {"refreshToken":"..."}, валидирует его как refresh-токен,
// выдаёт новую пару access+refresh. Пользователь должен существовать
// и быть активным (иначе 401).
func handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	var req refreshReq
	// B8.1: Decode — best-effort: тело может быть пустым (cookie-режим).
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieNameRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}
	if req.RefreshToken == "" {
		http.Error(w, `{"error":"refreshToken required"}`, http.StatusBadRequest)
		return
	}

	claims, err := ParseToken(req.RefreshToken, "refresh")
	if err != nil {
		slog.Warn("refresh: невалидный токен", "err", err)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	revoked, rerr := IsRefreshRevoked(db, claims.ID)
	if rerr != nil {
		slog.Warn("refresh: IsRefreshRevoked", "jti", claims.ID, "err", rerr)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if revoked {
		n, _ := RevokeAllForDoctor(db, claims.DoctorID)
		slog.Error("refresh: REPLAY, каскад", "jti", claims.ID, "doctor", claims.DoctorID, "revoked", n)
		auditLog(db, claims.DoctorID, "auth.refresh.replay", "auth", "", clientIP(r), map[string]any{"jti": claims.ID, "revoked": n}) // B9.2
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	// Проверяем, что врач ещё существует и активен.
	doc, err := getDoctorByID(db, claims.DoctorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("refresh: врач не найден", "doctor", claims.DoctorID)
		} else {
			slog.Warn("refresh: getDoctorByID", "doctor", claims.DoctorID, "err", err)
		}
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}
	if !doc.Active {
		slog.Warn("refresh: врач деактивирован", "doctor", doc.ID)
		http.Error(w, `{"error":"invalid refresh token"}`, http.StatusUnauthorized)
		return
	}

	newAccess, err := IssueAccessToken(doc.ID, Role(doc.Role))
	if err != nil {
		slog.Warn("refresh: IssueAccessToken", "id", doc.ID, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	newRefresh, refreshJTI, refreshExp, err := IssueRefreshToken(doc.ID, Role(doc.Role))
	if err != nil {
		slog.Warn("refresh: IssueRefreshToken", "id", doc.ID, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if err := SaveRefreshJTI(db, refreshJTI, doc.ID, refreshExp); err != nil {
		slog.Warn("refresh: SaveRefreshJTI", "id", doc.ID, "err", err)
	}

	// Ротация: старый jti помечаем отозванным, указывая замену.
	if err := RevokeRefreshJTI(db, claims.ID, refreshJTI); err != nil {
		slog.Warn("refresh: RevokeRefreshJTI", "jti", claims.ID, "err", err)
	}

	// B7.2: ротация cookie (в дополнение к JSON)
	setAuthCookies(w, newAccess, newRefresh)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loginResp{
		AccessToken:  newAccess,
		RefreshToken: newRefresh,
		Doctor: doctorBrief{
			ID:   doc.ID,
			Name: doc.Name,
			Role: doc.Role,
		},
	})
	slog.Info("refresh", "id", doc.ID, "email", doc.Email, "role", doc.Role)
	auditLog(db, doc.ID, "auth.refresh", "auth", "", clientIP(r), nil) // B9.2
}

// handleAuthLogout — POST /api/auth/logout.
// Stateless JWT: серверной сессии нет, отзывать нечего. Фронт сам чистит
// localStorage. Middleware уже проверил access-токен (не в whitelist),
// поэтому сюда попадают только аутентифицированные запросы.
func handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Best-effort: читаем опциональный refreshToken из тела.
	// Ошибки parse/revoke не фатальны — logout всегда отвечает 200.
	var req refreshReq
	_ = json.NewDecoder(r.Body).Decode(&req) // B8.1: best-effort
	if req.RefreshToken == "" {
		if c, err := r.Cookie(cookieNameRefresh); err == nil {
			req.RefreshToken = c.Value
		}
	}
	if req.RefreshToken != "" {
		if claims, err := ParseToken(req.RefreshToken, "refresh"); err == nil {
			if err := RevokeRefreshJTI(db, claims.ID, ""); err != nil {
				slog.Warn("logout: RevokeRefreshJTI", "jti", claims.ID, "err", err)
			} else {
				slog.Info("logout: refresh отозван", "jti", claims.ID, "doctor", claims.DoctorID)
			}
		} else {
			slog.Info("logout: refresh невалиден", "err", err)
		}
	}

	docID := doctorIDFromCtx(r)
	if docID == "" {
		slog.Info("logout: анонимный")
	} else {
		slog.Info("logout", "doctor", docID)
	}
	// B7.2: снимаем cookie
	auditLog(db, docID, "auth.logout", "auth", "", clientIP(r), nil) // B9.2
	clearAuthCookies(w)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

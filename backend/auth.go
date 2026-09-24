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
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

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
)

type ctxKey string

// devJWTSecret — fallback, если env BOTMAX_JWT_SECRET не задан.
// НЕ использовать в проде. Логируется с громким warning.
const devJWTSecret = "BOT_MAX_DEV_SECRET_DO_NOT_USE_IN_PROD_change_me"

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
type Claims struct {
	DoctorID string `json:"sub"`
	Role     string `json:"role"`
	Type     string `json:"typ"` // "access" | "refresh"
	jwt.RegisteredClaims
}

// jwtSecret возвращает секрет из env BOTMAX_JWT_SECRET.
// Если не задан — dev-секрет + предупреждение (один раз).
var jwtSecretWarned bool

func jwtSecret() []byte {
	s := os.Getenv("BOTMAX_JWT_SECRET")
	if s != "" {
		return []byte(s)
	}
	if !jwtSecretWarned {
		log.Println("⚠️  BOTMAX_JWT_SECRET не задан — использую dev-секрет. НЕ для прода!")
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
func IssueAccessToken(doctorID, role string) (string, error) {
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

// IssueRefreshToken выдаёт refresh-токен.
func IssueRefreshToken(doctorID, role string) (string, error) {
	jti, err := newJTI()
	if err != nil {
		return "", fmt.Errorf("jti: %w", err)
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
		return "", fmt.Errorf("sign refresh: %w", err)
	}
	return signed, nil
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
func withDoctorID(ctx context.Context, doctorID, role string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyDoctorID, doctorID)
	ctx = context.WithValue(ctx, ctxKeyDoctorRole, role)
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
func doctorRoleFromCtx(r *http.Request) string {
	if v, ok := r.Context().Value(ctxKeyDoctorRole).(string); ok {
		return v
	}
	return ""
}

// ---------------------------------------------------------------
// registerAuthRoutes — заглушка (маршруты появятся в 6.3 и 6.6).
// ---------------------------------------------------------------

// registerAuthRoutes регистрирует маршруты аутентификации на
// http.DefaultServeMux. Вызывается из main() рядом с
// registerPatientRoutes() — до создания http.Server.
func registerAuthRoutes() {
	// Шаг 6.3
	http.HandleFunc("/api/auth/login", handleAuthLogin)

	// Шаг 6.6: POST /api/auth/refresh, POST /api/auth/logout
	log.Println("🔐 registerAuthRoutes: /api/auth/login зарегистрирован")
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

	doc, err := getDoctorByEmail(db, req.Email)
	var storedHash string
	switch {
	case err == nil:
		storedHash = doc.PasswordHash
	case errors.Is(err, sql.ErrNoRows):
		// Не нашли — оставляем пустой hash, bcrypt всё равно отработает.
		doc = nil
	default:
		log.Printf("⚠️  login: getDoctorByEmail(%q): %v", req.Email, err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Всегда вызываем bcrypt (защита от timing-атак).
	if err := CheckPassword(storedHash, req.Password); err != nil || doc == nil || !doc.Active {
		log.Printf("🔒 login: отклонено для %q", req.Email)
		http.Error(w, `{"error":"invalid credentials"}`, http.StatusUnauthorized)
		return
	}

	access, err := IssueAccessToken(doc.ID, doc.Role)
	if err != nil {
		log.Printf("⚠️  login: IssueAccessToken(%q): %v", doc.ID, err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	refresh, err := IssueRefreshToken(doc.ID, doc.Role)
	if err != nil {
		log.Printf("⚠️  login: IssueRefreshToken(%q): %v", doc.ID, err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	if err := touchLastLogin(db, doc.ID); err != nil {
		log.Printf("⚠️  login: touchLastLogin(%q): %v", doc.ID, err) // не критично
	}

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
	log.Printf("✅ login: %s (%s) роль=%s", doc.ID, doc.Email, doc.Role)
}

// ---------------------------------------------------------------
// Шаг 6.4 — authMiddleware (soft-mode через BOTMAX_AUTH_ENFORCE)
// ---------------------------------------------------------------

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

		token := extractBearerToken(r)
		enforce := authEnforceEnabled()

		if token == "" {
			if enforce {
				log.Printf("🔒 auth: нет токена %s %s → 401", r.Method, r.URL.Path)
				http.Error(w, `{"error":"authorization required"}`, http.StatusUnauthorized)
				return
			}
			log.Printf("⚠️  auth: soft-mode, нет токена %s %s (пропускаем)", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
			return
		}

		claims, err := ParseToken(token, "access")
		if err != nil {
			if enforce {
				log.Printf("🔒 auth: невалидный токен %s %s: %v", r.Method, r.URL.Path, err)
				http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
				return
			}
			log.Printf("⚠️  auth: soft-mode, невалидный токен %s %s: %v (пропускаем)",
				r.Method, r.URL.Path, err)
			next.ServeHTTP(w, r)
			return
		}

		// Токен валиден — кладём в context и пропускаем.
		ctx := withDoctorID(r.Context(), claims.DoctorID, claims.Role)
		next.ServeHTTP(w, r.WithContext(ctx))
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

// isAuthWhitelist — пути, не требующие авторизации.
// Для /api/* используем строгое сравнение (не префикс), чтобы случайно
// не открыть /api/auth/loginXXX. Статика (не /api/*) — открыта.
func isAuthWhitelist(path string) bool {
	switch path {
	case "/api/health", "/api/auth/login", "/api/auth/refresh":
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
		v := strings.ToLower(strings.TrimSpace(os.Getenv("BOTMAX_AUTH_ENFORCE")))
		switch v {
		case "1", "true", "yes", "on":
			authEnforceVal = true
			log.Println("🔐 auth: enforce ВКЛЮЧЁН (BOTMAX_AUTH_ENFORCE=true)")
		default:
			authEnforceVal = false
			log.Println("⚠️  auth: soft-mode (BOTMAX_AUTH_ENFORCE не установлен)")
		}
	})
	return authEnforceVal
}

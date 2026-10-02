package main

// env.go — B1 (6.10): fail-fast валидация окружения на старте.
//
// Контракт:
//   * BOTMAX_ENV: "" | dev | development | test  → dev
//                 prod | production              → prod
//                 иначе                          → ошибка
//   * BOTMAX_JWT_SECRET: в prod обязателен и >= 32 байт;
//                        в dev пустой/короткий → warn (см. main).
//   * BOTMAX_AUTH_ENFORCE: ""                       → false
//                          1|true|yes|on            → true
//                          0|false|no|off           → false
//                          иначе                    → ошибка
//   * BOTMAX_AUDIT_RETENTION_DAYS: "" → 0 (retention выключен)
//                                   N  → N (удалять записи старше N дней); N < 0 → ошибка
//   * BOTMAX_AUDIT_CLEANUP_INTERVAL: ""  → 24h
//                                    dur → dur (Go duration, > 0); иначе → ошибка
//
// validateEnv() НЕ логирует и НЕ фаталит — это делает main() (B1.3).
// Так тестируемо без os.Exit.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Env — разобранные и провалидированные параметры окружения.
type Env struct {
	Name        string // "dev" | "prod"
	IsProd      bool
	JWTSecret   string // "" в dev, если не задан
	AuthEnforce bool
	// CookieSecure — ставить Secure на cookie (HTTPS-only).
	// В prod по умолчанию true, в dev — false (чтобы работало на http://localhost).
	CookieSecure bool
	// DBPath — путь к SQLite-файлу. Пусто → main.go использует rootDir/bot_max.db.
	DBPath string
	// AuditRetentionDays — сколько дней хранить audit_log. 0 → retention выключен.
	AuditRetentionDays int
	// AuditCleanupInterval — период между прогонами очистки (B19).
	AuditCleanupInterval time.Duration
	// ImpersonationTTL — время жизни access-токена в режиме impersonation
	// (B21, было константой в auth.go). По умолчанию 15m.
	ImpersonationTTL time.Duration
}

// devJWTSecret — fallback для dev-режима.
// В prod НЕ используется: validateEnv вернёт ошибку при пустом секрете.
// (В auth.go пока остаётся дубликат — удалим в B1.2.)
const devJWTSecret = "BOT_MAX_DEV_SECRET_DO_NOT_USE_IN_PROD_change_me"

// minProdJWTSecretLen — минимальная длина секрета в prod.
const minProdJWTSecretLen = 32

// currentEnv — заполняется в main() после validateEnv() (B1.3).
// Нужен jwtSecret() в B1.2. Пока просто объявлен, никого не трогает.
var currentEnv *Env

// normalizeEnvName возвращает "dev"|"prod" или ошибку.
func normalizeEnvName(raw string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "dev", "development", "test":
		return "dev", nil
	case "prod", "production":
		return "prod", nil
	default:
		return "", fmt.Errorf("BOTMAX_ENV=%q: ожидается dev|prod (или пусто)", raw)
	}
}

// parseAuthEnforce строго парсит BOTMAX_AUTH_ENFORCE.
// Пусто → false. Мусор → ошибка (fail-fast).
func parseAuthEnforce(raw string) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, fmt.Errorf("BOTMAX_AUTH_ENFORCE=%q: ожидается 1|0|true|false|yes|no|on|off", raw)
	}
}

// parseCookieSecure строго парсит BOTMAX_COOKIE_SECURE.
//
//	"" → isProd (default: true в prod, false в dev)
//	1|true|yes|on  → true
//	0|false|no|off → false
//	иначе          → ошибка
func parseCookieSecure(raw string, isProd bool) (bool, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "":
		return isProd, nil
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("BOTMAX_COOKIE_SECURE=%q: ожидается 1|0|true|false|yes|no|on|off", raw)
	}
}

// parseNonNegativeInt строго парсит целое >= 0.
// Пусто → def. Мусор/отрицательное → ошибка.
func parseNonNegativeInt(name, raw string, def int) (int, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: ожидается целое >= 0", name, raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s=%q: ожидается целое >= 0", name, raw)
	}
	return n, nil
}

// parseDuration строго парсит time.Duration > 0.
// Пусто → def. Мусор/<= 0 → ошибка.
func parseDuration(name, raw string, def time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: ожидается Go duration (> 0), напр. 24h, 30m, 10s", name, raw)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s=%q: duration должен быть > 0", name, raw)
	}
	return d, nil
}

// validateEnv читает окружение и возвращает Env или ошибку.
// Никаких side-effect'ов, кроме чтения os.Getenv.
func validateEnv() (*Env, error) {
	name, err := normalizeEnvName(os.Getenv("BOTMAX_ENV"))
	if err != nil {
		return nil, err
	}
	isProd := name == "prod"

	secret := os.Getenv("BOTMAX_JWT_SECRET")

	if isProd {
		if secret == "" {
			return nil, fmt.Errorf("BOTMAX_ENV=prod: BOTMAX_JWT_SECRET обязателен")
		}
		if len(secret) < minProdJWTSecretLen {
			return nil, fmt.Errorf("BOTMAX_ENV=prod: BOTMAX_JWT_SECRET короче %d байт (got %d)",
				minProdJWTSecretLen, len(secret))
		}
	}

	enforce, err := parseAuthEnforce(os.Getenv("BOTMAX_AUTH_ENFORCE"))
	if err != nil {
		return nil, err
	}

	cookieSecure, err := parseCookieSecure(os.Getenv("BOTMAX_COOKIE_SECURE"), isProd)
	if err != nil {
		return nil, err
	}

	dbPath := strings.TrimSpace(os.Getenv("BOTMAX_DB_PATH"))

	retentionDays, err := parseNonNegativeInt(
		"BOTMAX_AUDIT_RETENTION_DAYS", os.Getenv("BOTMAX_AUDIT_RETENTION_DAYS"), 0)
	if err != nil {
		return nil, err
	}
	cleanupInterval, err := parseDuration(
		"BOTMAX_AUDIT_CLEANUP_INTERVAL", os.Getenv("BOTMAX_AUDIT_CLEANUP_INTERVAL"), 24*time.Hour)
	if err != nil {
		return nil, err
	}

	impersonationTTL, err := parseDuration(
		"BOTMAX_IMPERSONATION_TTL", os.Getenv("BOTMAX_IMPERSONATION_TTL"), 15*time.Minute)
	if err != nil {
		return nil, err
	}

	return &Env{
		Name:                 name,
		IsProd:               isProd,
		JWTSecret:            secret,
		AuthEnforce:          enforce,
		CookieSecure:         cookieSecure,
		DBPath:               dbPath,
		AuditRetentionDays:   retentionDays,
		AuditCleanupInterval: cleanupInterval,
		ImpersonationTTL:     impersonationTTL,
	}, nil
}

package main

// env_test.go — B1 (6.10, ч.4): табличные тесты validateEnv и jwtSecret.
//
// ВАЖНО: тесты НЕ параллельные (t.Parallel не используется) —
// они дёргают пакетные переменные currentEnv и jwtSecretWarned.
// Каждый тест восстанавливает их через t.Cleanup.

import (
	"os"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------
// Восстановление пакетных переменных
// ---------------------------------------------------------------

// snapshotGlobals сохраняет currentEnv/jwtSecretWarned и вешает
// t.Cleanup, который вернёт их к исходным значениям.
func snapshotGlobals(t *testing.T) {
	t.Helper()
	prevEnv := currentEnv
	prevWarned := jwtSecretWarned
	t.Cleanup(func() {
		currentEnv = prevEnv
		jwtSecretWarned = prevWarned
	})
}

// clearEnvVars отвязывает все три переменные окружения,
// чтобы тест не зависел от окружения запуска.
func clearEnvVars(t *testing.T) {
	t.Helper()
	t.Setenv("BOTMAX_ENV", "")
	t.Setenv("BOTMAX_JWT_SECRET", "")
	t.Setenv("BOTMAX_AUTH_ENFORCE", "")
	t.Setenv("BOTMAX_COOKIE_SECURE", "")
	t.Setenv("BOTMAX_AUDIT_RETENTION_DAYS", "")
	t.Setenv("BOTMAX_AUDIT_CLEANUP_INTERVAL", "")
	// t.Setenv("", "") не отвязывает переменную — Go выставит её в пустую строку,
	// что для нас эквивалентно «не задано» (normalize/parse трактуют "" как default).
}

// ---------------------------------------------------------------
// normalizeEnvName
// ---------------------------------------------------------------

func TestNormalizeEnvName(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "dev", false},
		{"dev", "dev", false},
		{"development", "dev", false},
		{"test", "dev", false},
		{"DEV", "dev", false},
		{"  prod  ", "prod", false},
		{"prod", "prod", false},
		{"production", "prod", false},
		{"Production", "prod", false},
		{"staging", "", true},
		{"qa", "", true},
		{"продакшен", "", true},
	}
	for _, c := range cases {
		got, err := normalizeEnvName(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("normalizeEnvName(%q): ожидали ошибку, получили %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("normalizeEnvName(%q): неожиданная ошибка: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("normalizeEnvName(%q) = %q, ожидали %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------
// parseAuthEnforce
// ---------------------------------------------------------------

func TestParseAuthEnforce(t *testing.T) {
	cases := []struct {
		in      string
		want    bool
		wantErr bool
	}{
		{"", false, false},
		{"0", false, false},
		{"false", false, false},
		{"no", false, false},
		{"off", false, false},
		{"FALSE", false, false},
		{"  OFF  ", false, false},
		{"1", true, false},
		{"true", true, false},
		{"yes", true, false},
		{"on", true, false},
		{"TRUE", true, false},
		{"  Yes ", true, false},
		{"maybe", false, true},
		{"2", false, true},
		{"enabled", false, true},
	}
	for _, c := range cases {
		got, err := parseAuthEnforce(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseAuthEnforce(%q): ожидали ошибку, получили %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseAuthEnforce(%q): неожиданная ошибка: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseAuthEnforce(%q) = %v, ожидали %v", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------
// parseNonNegativeInt
// ---------------------------------------------------------------

func TestParseNonNegativeInt(t *testing.T) {
	const def = 42
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", def, false},
		{"0", 0, false},
		{"30", 30, false},
		{"  7  ", 7, false},
		{"365", 365, false},
		{"-1", 0, true},
		{"-100", 0, true},
		{"abc", 0, true},
		{"3.5", 0, true},
		{"1 2", 0, true},
	}
	for _, c := range cases {
		got, err := parseNonNegativeInt("X", c.in, def)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseNonNegativeInt(%q): ожидали ошибку, получили %d", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNonNegativeInt(%q): неожиданная ошибка: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseNonNegativeInt(%q) = %d, ожидали %d", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------
// parseDuration
// ---------------------------------------------------------------

func TestParseDuration(t *testing.T) {
	def := 24 * time.Hour
	cases := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{"", def, false},
		{"24h", 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"10s", 10 * time.Second, false},
		{"1h30m", 90 * time.Minute, false},
		{"  2h  ", 2 * time.Hour, false},
		{"0s", 0, true},
		{"0", 0, true},
		{"-1h", 0, true},
		{"abc", 0, true},
		{"24", 0, true},
	}
	for _, c := range cases {
		got, err := parseDuration("X", c.in, def)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseDuration(%q): ожидали ошибку, получили %v", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseDuration(%q): неожиданная ошибка: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("parseDuration(%q) = %v, ожидали %v", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------
// validateEnv
// ---------------------------------------------------------------

func TestParseCookieSecure(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		isProd  bool
		want    bool
		wantErr bool
	}{
		{"empty-dev", "", false, false, false},
		{"empty-prod", "", true, true, false},
		{"one-dev", "1", false, true, false},
		{"true-dev", "true", false, true, false},
		{"yes-prod", "yes", true, true, false},
		{"on-dev", "on", false, true, false},
		{"zero-prod", "0", true, false, false},
		{"false-prod", "false", true, false, false},
		{"no-dev", "no", false, false, false},
		{"off-prod", "off", true, false, false},
		{"junk", "maybe", true, false, true},
		{"two", "2", false, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseCookieSecure(c.raw, c.isProd)
			if c.wantErr {
				if err == nil {
					t.Fatalf("ожидали ошибку для raw=%q isProd=%v", c.raw, c.isProd)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if got != c.want {
				t.Fatalf("parseCookieSecure(%q, %v) = %v, ожидали %v", c.raw, c.isProd, got, c.want)
			}
		})
	}
}

func TestValidateEnv(t *testing.T) {
	longSecret := strings.Repeat("x", 32)
	shortSecret := strings.Repeat("x", 31)

	cases := []struct {
		name            string
		envName         string
		envSecret       string
		envForce        string
		envCookieSecure string
		envDBPath       string
		envRetention    string
		envCleanup      string

		wantName         string
		wantIsProd       bool
		wantSecret       string
		wantEnforce      bool
		wantCookieSecure bool
		wantDBPath       string
		wantRetention    int
		wantCleanup      time.Duration
		wantErrSubstr    string
	}{
		{
			name:       "dev-пусто",
			envName:    "",
			wantName:   "dev",
			wantSecret: "",
		},
		{
			name:       "dev-явный",
			envName:    "dev",
			wantName:   "dev",
			wantSecret: "",
		},
		{
			name:       "dev-с-секретом",
			envName:    "development",
			envSecret:  "short-ok-in-dev",
			wantName:   "dev",
			wantSecret: "short-ok-in-dev",
		},
		{
			name:        "dev-enforce=true",
			envName:     "test",
			envForce:    "on",
			wantName:    "dev",
			wantEnforce: true,
		},
		{
			name:             "prod-32-ok",
			envName:          "prod",
			envSecret:        longSecret,
			wantName:         "prod",
			wantIsProd:       true,
			wantSecret:       longSecret,
			wantCookieSecure: true,
		},
		{
			name:             "prod-64-ok",
			envName:          "production",
			envSecret:        longSecret + longSecret,
			wantName:         "prod",
			wantIsProd:       true,
			wantSecret:       longSecret + longSecret,
			wantCookieSecure: true,
		},
		{
			name:          "prod-пусто-err",
			envName:       "prod",
			wantErrSubstr: "BOTMAX_JWT_SECRET обязателен",
		},
		{
			name:          "prod-31-err",
			envName:       "prod",
			envSecret:     shortSecret,
			wantErrSubstr: "короче 32 байт",
		},
		{
			name:          "env-мусор-err",
			envName:       "staging",
			wantErrSubstr: "BOTMAX_ENV",
		},
		{
			name:          "enforce-мусор-err",
			envName:       "dev",
			envForce:      "maybe",
			wantErrSubstr: "BOTMAX_AUTH_ENFORCE",
		},
		{
			name:             "dev-cookie-secure-true",
			envName:          "dev",
			envCookieSecure:  "1",
			wantName:         "dev",
			wantCookieSecure: true,
		},
		{
			name:             "prod-cookie-secure-false",
			envName:          "prod",
			envSecret:        longSecret,
			envCookieSecure:  "0",
			wantName:         "prod",
			wantIsProd:       true,
			wantSecret:       longSecret,
			wantCookieSecure: false,
		},
		{
			name:            "cookie-secure-junk",
			envName:         "dev",
			envCookieSecure: "maybe",
			wantErrSubstr:   "BOTMAX_COOKIE_SECURE",
		},
		{
			name:       "db-path-empty",
			envName:    "dev",
			wantName:   "dev",
			wantDBPath: "",
		},
		{
			name:       "db-path-custom",
			envName:    "dev",
			envDBPath:  "/tmp/foo.db",
			wantName:   "dev",
			wantDBPath: "/tmp/foo.db",
		},
		{
			name:          "retention-default",
			envName:       "dev",
			wantName:      "dev",
			wantRetention: 0,
			wantCleanup:   24 * time.Hour,
		},
		{
			name:          "retention-30",
			envName:       "dev",
			envRetention:  "30",
			wantName:      "dev",
			wantRetention: 30,
			wantCleanup:   24 * time.Hour,
		},
		{
			name:          "cleanup-1h",
			envName:       "dev",
			envCleanup:    "1h",
			wantName:      "dev",
			wantRetention: 0,
			wantCleanup:   time.Hour,
		},
		{
			name:          "retention-junk-err",
			envName:       "dev",
			envRetention:  "abc",
			wantErrSubstr: "BOTMAX_AUDIT_RETENTION_DAYS",
		},
		{
			name:          "retention-negative-err",
			envName:       "dev",
			envRetention:  "-1",
			wantErrSubstr: "BOTMAX_AUDIT_RETENTION_DAYS",
		},
		{
			name:          "cleanup-junk-err",
			envName:       "dev",
			envCleanup:    "maybe",
			wantErrSubstr: "BOTMAX_AUDIT_CLEANUP_INTERVAL",
		},
		{
			name:          "cleanup-zero-err",
			envName:       "dev",
			envCleanup:    "0s",
			wantErrSubstr: "BOTMAX_AUDIT_CLEANUP_INTERVAL",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("BOTMAX_ENV", c.envName)
			t.Setenv("BOTMAX_JWT_SECRET", c.envSecret)
			t.Setenv("BOTMAX_AUTH_ENFORCE", c.envForce)
			t.Setenv("BOTMAX_COOKIE_SECURE", c.envCookieSecure)
			t.Setenv("BOTMAX_DB_PATH", c.envDBPath)
			t.Setenv("BOTMAX_AUDIT_RETENTION_DAYS", c.envRetention)
			t.Setenv("BOTMAX_AUDIT_CLEANUP_INTERVAL", c.envCleanup)

			got, err := validateEnv()
			if c.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("ожидали ошибку, получили nil (env=%+v)", got)
				}
				if !strings.Contains(err.Error(), c.wantErrSubstr) {
					t.Fatalf("ошибка %q не содержит %q", err.Error(), c.wantErrSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if got.Name != c.wantName {
				t.Errorf("Name = %q, ожидали %q", got.Name, c.wantName)
			}
			if got.IsProd != c.wantIsProd {
				t.Errorf("IsProd = %v, ожидали %v", got.IsProd, c.wantIsProd)
			}
			if got.JWTSecret != c.wantSecret {
				t.Errorf("JWTSecret = %q, ожидали %q", got.JWTSecret, c.wantSecret)
			}
			if got.AuthEnforce != c.wantEnforce {
				t.Errorf("AuthEnforce = %v, ожидали %v", got.AuthEnforce, c.wantEnforce)
			}
			if got.CookieSecure != c.wantCookieSecure {
				t.Errorf("CookieSecure = %v, ожидали %v", got.CookieSecure, c.wantCookieSecure)
			}
			if got.DBPath != c.wantDBPath {
				t.Errorf("DBPath = %q, ожидали %q", got.DBPath, c.wantDBPath)
			}
			if got.AuditRetentionDays != c.wantRetention {
				t.Errorf("AuditRetentionDays = %d, ожидали %d", got.AuditRetentionDays, c.wantRetention)
			}
			// wantCleanup == 0 означает «не проверяем»: у старых кейсов
			// (до B19.1) поле wantCleanup не задано, а validateEnv вернёт дефолт 24h.
			if c.wantCleanup != 0 && got.AuditCleanupInterval != c.wantCleanup {
				t.Errorf("AuditCleanupInterval = %v, ожидали %v", got.AuditCleanupInterval, c.wantCleanup)
			}
		})
	}
}

// ---------------------------------------------------------------
// jwtSecret
// ---------------------------------------------------------------

func TestJwtSecret_PanicWithoutCurrentEnv(t *testing.T) {
	snapshotGlobals(t)
	currentEnv = nil

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("ожидали panic при currentEnv == nil")
		}
		if !strings.Contains(fmt_Sprint(r), "currentEnv == nil") {
			t.Fatalf("panic %v не содержит 'currentEnv == nil'", r)
		}
	}()

	_ = jwtSecret()
}

func TestJwtSecret_UsesCurrentEnv(t *testing.T) {
	snapshotGlobals(t)
	currentEnv = &Env{Name: "prod", IsProd: true, JWTSecret: "abc-32-bytes-secret-abcdefghijkl"}
	jwtSecretWarned = false

	got := string(jwtSecret())
	if got != currentEnv.JWTSecret {
		t.Fatalf("jwtSecret() = %q, ожидали %q", got, currentEnv.JWTSecret)
	}
	if jwtSecretWarned {
		t.Fatalf("jwtSecretWarned не должен был переключиться (секрет задан)")
	}
}

func TestJwtSecret_DevFallback(t *testing.T) {
	snapshotGlobals(t)
	currentEnv = &Env{Name: "dev", JWTSecret: ""}
	jwtSecretWarned = false

	got := string(jwtSecret())
	if got != devJWTSecret {
		t.Fatalf("jwtSecret() в dev с пустым секретом = %q, ожидали devJWTSecret", got)
	}
	if !jwtSecretWarned {
		t.Fatalf("jwtSecretWarned должен был переключиться (dev-fallback)")
	}
}

// ---------------------------------------------------------------
// вспомогательное (без import "fmt")
// ---------------------------------------------------------------

// fmt_Sprint — грубая замена fmt.Sprint, чтобы не тащить лишний импорт.
// Нам достаточно, чтобы panic-значение привелось к строке.
func fmt_Sprint(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case error:
		return s.Error()
	default:
		return ""
	}
}

// Гарантия, что os используется (в snapshotGlobals/clearEnvVars может
// показаться, что os не нужен — но мы его используем неявно через t.Setenv;
// тем не менее — явная проверка для теста отсутствия "unused import":
var _ = os.Getenv

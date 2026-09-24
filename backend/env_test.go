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
// validateEnv
// ---------------------------------------------------------------

func TestValidateEnv(t *testing.T) {
	longSecret := strings.Repeat("x", 32)
	shortSecret := strings.Repeat("x", 31)

	cases := []struct {
		name      string
		envName   string
		envSecret string
		envForce  string

		wantName      string
		wantIsProd    bool
		wantSecret    string
		wantEnforce   bool
		wantErrSubstr string
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
			name:       "dev-enforce=true",
			envName:    "test",
			envForce:   "on",
			wantName:   "dev",
			wantEnforce: true,
		},
		{
			name:       "prod-32-ok",
			envName:    "prod",
			envSecret:  longSecret,
			wantName:   "prod",
			wantIsProd: true,
			wantSecret: longSecret,
		},
		{
			name:       "prod-64-ok",
			envName:    "production",
			envSecret:  longSecret + longSecret,
			wantName:   "prod",
			wantIsProd: true,
			wantSecret: longSecret + longSecret,
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
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("BOTMAX_ENV", c.envName)
			t.Setenv("BOTMAX_JWT_SECRET", c.envSecret)
			t.Setenv("BOTMAX_AUTH_ENFORCE", c.envForce)

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

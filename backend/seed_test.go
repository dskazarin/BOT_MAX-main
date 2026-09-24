package main

// seed_test.go — подпатч 6.11 (B2.2): тесты ShouldSeedDevAccounts.
//
// Чистые функции, без БД и без сети. Тесты НЕ параллельные —
// держим единый стиль с env_test.go (пакетные переменные не трогаем,
// но избегаем сюрпризов при будущих правках).

import "testing"

func TestShouldSeedDevAccounts(t *testing.T) {
	cases := []struct {
		name string
		env  *Env
		want bool
	}{
		{
			name: "nil-env",
			env:  nil,
			want: false,
		},
		{
			name: "dev",
			env:  &Env{Name: "dev", IsProd: false},
			want: true,
		},
		{
			name: "dev-zero-value",
			env:  &Env{Name: "dev"}, // IsProd zero-value = false
			want: true,
		},
		{
			name: "prod",
			env:  &Env{Name: "prod", IsProd: true},
			want: false,
		},
		{
			name: "prod-name-only", // не должно случаться после validateEnv, но защищаемся
			env:  &Env{Name: "prod", IsProd: false},
			want: true, // смотрим только на IsProd — единственный источник истины
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ShouldSeedDevAccounts(c.env)
			if got != c.want {
				t.Fatalf("ShouldSeedDevAccounts(%+v) = %v, ожидали %v", c.env, got, c.want)
			}
		})
	}
}

func TestEnvNameOrUnknown(t *testing.T) {
	cases := []struct {
		name string
		env  *Env
		want string
	}{
		{"nil", nil, "unknown"},
		{"dev", &Env{Name: "dev"}, "dev"},
		{"prod", &Env{Name: "prod"}, "prod"},
		{"empty-name", &Env{Name: ""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := envNameOrUnknown(c.env)
			if got != c.want {
				t.Fatalf("envNameOrUnknown(%+v) = %q, ожидали %q", c.env, got, c.want)
			}
		})
	}
}

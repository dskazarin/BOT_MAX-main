package main

// seed.go — подпатч 6.11 (B2): dev-аккаунты не создаются в prod.
//
// Раньше seedDefaultDoctor и seedAdminRoles вызывались из main()
// безусловно — тестовые учётки (DOC-001, ADMIN-001, SUPER-001)
// оказывались в prod-базе. Теперь всё гейтится через ShouldSeedDevAccounts.

import (
	"database/sql"
	"log/slog"
	"os"
)

// ShouldSeedDevAccounts — сидим ли dev-тестовые аккаунты.
//
// В prod — никогда: тестовые учётки в проде = дыра.
// В dev — да.
//
// env == nil → false (безопасный дефолт: без валидного env не сидим).
func ShouldSeedDevAccounts(env *Env) bool {
	if env == nil {
		return false
	}
	return !env.IsProd
}

// runDevSeeds выполняет dev-seed'ы, если это разрешено окружением.
// В prod пишет явный лог о пропуске и ничего не делает.
// При ошибке seed — slog.Error + os.Exit(1).
func runDevSeeds(db *sql.DB, env *Env) {
	if !ShouldSeedDevAccounts(env) {
		slog.Info("seed: пропущен (prod)", "env", envNameOrUnknown(env))
		return
	}
	if err := seedDefaultDoctor(db); err != nil {
		slog.Error("seedDefaultDoctor", "err", err); os.Exit(1)
	}
	if err := seedAdminRoles(db); err != nil {
		slog.Error("seedAdminRoles", "err", err); os.Exit(1)
	}
}

// envNameOrUnknown — безопасно достаёт Name из Env (для лога).
func envNameOrUnknown(env *Env) string {
	if env == nil {
		return "unknown"
	}
	return env.Name
}

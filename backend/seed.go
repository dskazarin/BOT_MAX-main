package main

// seed.go — подпатч 6.11 (B2): dev-аккаунты не создаются в prod.
//
// Раньше seedDefaultDoctor и seedAdminRoles вызывались из main()
// безусловно — тестовые учётки (DOC-001, ADMIN-001, SUPER-001)
// оказывались в prod-базе. Теперь всё гейтится через ShouldSeedDevAccounts.

import (
	"database/sql"
	"log"
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
// При ошибке seed — log.Fatalf (как было в main).
func runDevSeeds(db *sql.DB, env *Env) {
	if !ShouldSeedDevAccounts(env) {
		log.Printf("🌱 seed: пропущен (env=%s, dev-аккаунты в prod не создаются)", envNameOrUnknown(env))
		return
	}
	if err := seedDefaultDoctor(db); err != nil {
		log.Fatalf("❌ seedDefaultDoctor: %v", err)
	}
	if err := seedAdminRoles(db); err != nil {
		log.Fatalf("❌ seedAdminRoles: %v", err)
	}
}

// envNameOrUnknown — безопасно достаёт Name из Env (для лога).
func envNameOrUnknown(env *Env) string {
	if env == nil {
		return "unknown"
	}
	return env.Name
}

//go:build smoke

package main

import (
	"os"
	"testing"
)

// TestSmoke_MigrateRealDB — смоук миграции на КОПИИ реальной БД.
// Запуск: SMOKE_DB=/tmp/bot_max.smoke.db go test -tags smoke -run TestSmoke -v
// Работать только с копией — оригинал bot_max.db не трогать.
func TestSmoke_MigrateRealDB(t *testing.T) {
	path := os.Getenv("SMOKE_DB")
	if path == "" {
		t.Skip("SMOKE_DB not set")
	}

	db, err := InitDB(path)
	if err != nil {
		t.Fatalf("InitDB(%s): %v", path, err)
	}
	defer db.Close()

	var notnull int
	if err := db.QueryRow(`SELECT "notnull" FROM pragma_table_info('patients') WHERE name = 'doctor_id'`).Scan(&notnull); err != nil {
		t.Fatalf("read notnull: %v", err)
	}
	if notnull != 0 {
		t.Errorf("doctor_id.notnull = %d, ожидалось 0", notnull)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM patients`).Scan(&count); err != nil {
		t.Fatalf("count patients: %v", err)
	}
	t.Logf("patients after migration: %d", count)

	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("fk_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Errorf("foreign_key_check: обнаружены нарушения")
	}

	idx := map[string]bool{}
	ir, err := db.Query(`SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='patients'`)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	defer ir.Close()
	for ir.Next() {
		var n string
		_ = ir.Scan(&n)
		idx[n] = true
	}
	for _, want := range []string{"idx_patients_doctor", "idx_patients_clinic"} {
		if !idx[want] {
			t.Errorf("индекс %s отсутствует", want)
		}
	}

	t.Logf("smoke migration OK on %s", path)
}

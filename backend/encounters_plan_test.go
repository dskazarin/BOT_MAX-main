package main

// encounters_plan_test.go — B29.1.5: EXPLAIN QUERY PLAN.
//
// Проверяем, что list-запросы (offset и keyset) опираются на idx_enc_patient_kind,
// а не делают полный скан encounters.

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

// explainPlan возвращает объединённый "detail" всех строк EXPLAIN QUERY PLAN.
func explainPlan(t *testing.T, db *sql.DB, query string, args ...any) string {
	t.Helper()
	rows, err := db.Query("EXPLAIN QUERY PLAN "+query, args...)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
	}
	defer rows.Close()

	var details []string
	for rows.Next() {
		var (
			id, parent, notused int
			detail              string
		)
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan row: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("plan rows: %v", err)
	}
	return strings.Join(details, " | ")
}

// seedPlanEncounters — 500 приёмов у P1: kind циклится по 5 значениям,
// started_at — по часам от 2026-01-01T00:00Z. Также ANALYZE, чтобы планировщик
// опирался на статистику.
func seedPlanEncounters(t *testing.T, db *sql.DB) {
	t.Helper()
	kinds := []string{"consultation", "study", "procedure", "lab", "followup"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	stmt, err := tx.Prepare(`INSERT INTO encounters
		(id, patient_id, doctor_id, clinic_id, specialty, title, kind,
		 started_at, status, common_json, specialty_json, created_at, updated_at)
		VALUES (?, 'P1', 'D1', NULL, '', '', ?, ?, 'draft', '{}', '{}', ?, ?)`)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("prepare: %v", err)
	}
	defer stmt.Close()

	for i := 0; i < 500; i++ {
		ts := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		id := "ENC-PLAN-" + strings.Repeat("0", 4-len(itoa(i))) + itoa(i)
		if _, err := stmt.Exec(id, kinds[i%len(kinds)], ts, ts, ts); err != nil {
			_ = tx.Rollback()
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if _, err := db.Exec("ANALYZE"); err != nil {
		t.Fatalf("analyze: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}

// --- 1. Offset-путь: WHERE patient_id=? AND kind=? ORDER BY started_at DESC, id DESC LIMIT ? ---
func TestEncountersPlan_UsesIdxEncPatientKind(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedPlanEncounters(t, db)

	plan := explainPlan(t, db,
		`SELECT id FROM encounters
		 WHERE patient_id = ? AND kind = ?
		 ORDER BY started_at DESC, id DESC LIMIT ?`,
		"P1", "consultation", 20)

	if !strings.Contains(plan, "idx_enc_patient_kind") {
		t.Errorf("plan doesn't use idx_enc_patient_kind:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN encounters") {
		t.Errorf("plan does full SCAN encounters:\n%s", plan)
	}
}

// --- 2. Keyset-путь: тот же индекс, добавочное условие (started_at,id) < (?,?) ---
func TestEncountersPlan_KeysetUsesIdxEncPatientKind(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedPlanEncounters(t, db)

	plan := explainPlan(t, db,
		`SELECT id FROM encounters
		 WHERE patient_id = ? AND kind = ?
		   AND (started_at < ? OR (started_at = ? AND id < ?))
		 ORDER BY started_at DESC, id DESC LIMIT ?`,
		"P1", "consultation",
		"2026-01-15T00:00:00Z", "2026-01-15T00:00:00Z", "ENC-X",
		20)

	if !strings.Contains(plan, "idx_enc_patient_kind") {
		t.Errorf("keyset plan doesn't use idx_enc_patient_kind:\n%s", plan)
	}
	if strings.Contains(plan, "SCAN encounters") {
		t.Errorf("keyset plan does full SCAN encounters:\n%s", plan)
	}
}

// --- 3. Санити: где kind отсутствует — используется idx_enc_patient ---
func TestEncountersPlan_NoKindUsesIdxEncPatient(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedPlanEncounters(t, db)

	plan := explainPlan(t, db,
		`SELECT id FROM encounters
		 WHERE patient_id = ?
		 ORDER BY started_at DESC, id DESC LIMIT ?`,
		"P1", 20)

	if !strings.Contains(plan, "idx_enc_patient") {
		t.Errorf("plan doesn't use idx_enc_patient:\n%s", plan)
	}
}

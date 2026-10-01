package main

// audit_test.go — подпатч 6.18 (B9.6): тесты audit-store.
//
// Переиспользует newTestDB(t) из refresh_test.go — :memory: SQLite
// с полной схемой через InitDB.
//
// HTTP-хендлер handleAuditList покрыт end-to-end smoke в B9.2–B9.4.
// Здесь — только store: auditLog + auditCleanup + прямой SELECT.

import (
	"database/sql"
	"strings"
	"testing"
	"time"
)

// fetchAuditRow — читает одну строку audit_log по id.
func fetchAuditRow(t *testing.T, db *sql.DB, id int64) (ts, doctorID, action, entity, entityID, ip, detail string) {
	t.Helper()
	var (
		dID, ent, entID, ipA, det sql.NullString
	)
	err := db.QueryRow(
		`SELECT ts, doctor_id, action, entity, entity_id, ip, detail
		 FROM audit_log WHERE id = ?`, id,
	).Scan(&ts, &dID, &action, &ent, &entID, &ipA, &det)
	if err != nil {
		t.Fatalf("fetch audit row %d: %v", id, err)
	}
	if dID.Valid {
		doctorID = dID.String
	}
	if ent.Valid {
		entity = ent.String
	}
	if entID.Valid {
		entityID = entID.String
	}
	if ipA.Valid {
		ip = ipA.String
	}
	if det.Valid {
		detail = det.String
	}
	return
}

// lastAuditID — id последней записи.
func lastAuditID(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(`SELECT id FROM audit_log ORDER BY id DESC LIMIT 1`).Scan(&id); err != nil {
		t.Fatalf("last audit id: %v", err)
	}
	return id
}

// ---------------------------------------------------------------
// 1. Insert + fetch
// ---------------------------------------------------------------

func TestAudit_InsertAndFetch(t *testing.T) {
	db := newTestDB(t)

	auditLog(db, "DOC-1", "auth.login", "auth", "DOC-1", "1.2.3.4", nil)

	id := lastAuditID(t, db)
	ts, doctorID, action, entity, entityID, ip, _ := fetchAuditRow(t, db, id)

	if action != "auth.login" {
		t.Fatalf("action = %q, ожидали auth.login", action)
	}
	if doctorID != "DOC-1" {
		t.Fatalf("doctor_id = %q, ожидали DOC-1", doctorID)
	}
	if entity != "auth" {
		t.Fatalf("entity = %q, ожидали auth", entity)
	}
	if entityID != "DOC-1" {
		t.Fatalf("entity_id = %q, ожидали DOC-1", entityID)
	}
	if ip != "1.2.3.4" {
		t.Fatalf("ip = %q, ожидали 1.2.3.4", ip)
	}
	if ts == "" {
		t.Fatalf("ts пусто")
	}
	// ts формат: "2006-01-02 15:04:05"
	if len(ts) != 19 || ts[10] != ' ' {
		t.Fatalf("ts формат неверен: %q", ts)
	}
}

// ---------------------------------------------------------------
// 2. Detail — JSON и nil
// ---------------------------------------------------------------

func TestAudit_DetailJSON(t *testing.T) {
	db := newTestDB(t)

	// С detail
	auditLog(db, "DOC-2", "doctor.create", "doctor", "DOC-3", "5.6.7.8",
		map[string]string{"email": "x@y", "role": "doctor"})
	id := lastAuditID(t, db)
	_, _, _, _, _, _, detail := fetchAuditRow(t, db, id)

	if !strings.Contains(detail, `"email":"x@y"`) {
		t.Fatalf("detail не содержит email: %q", detail)
	}
	if !strings.Contains(detail, `"role":"doctor"`) {
		t.Fatalf("detail не содержит role: %q", detail)
	}

	// Без detail — NULL
	auditLog(db, "DOC-2", "auth.logout", "auth", "", "5.6.7.8", nil)
	id2 := lastAuditID(t, db)
	_, _, _, _, _, _, detail2 := fetchAuditRow(t, db, id2)
	if detail2 != "" {
		t.Fatalf("detail должен быть пустым для nil, got %q", detail2)
	}
}

// ---------------------------------------------------------------
// 3. Nil DB — не паникует
// ---------------------------------------------------------------

func TestAudit_NilDB(t *testing.T) {
	// auditLog(nil, ...) — должен молча выйти
	auditLog(nil, "DOC-1", "auth.login", "auth", "", "1.2.3.4", nil)
	// Если не паниковало — ок
}

// ---------------------------------------------------------------
// 4. Пустой action — не пишет
// ---------------------------------------------------------------

func TestAudit_EmptyAction(t *testing.T) {
	db := newTestDB(t)

	auditLog(db, "DOC-1", "", "auth", "", "1.2.3.4", nil)

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("пустой action не должен писать, count=%d", count)
	}
}

// ---------------------------------------------------------------
// 5. Cleanup — удаляет старые
// ---------------------------------------------------------------

func TestAudit_Cleanup(t *testing.T) {
	db := newTestDB(t)

	// Свежая запись — через auditLog
	auditLog(db, "D1", "auth.login", "auth", "", "1.1.1.1", nil)

	// Старая запись — вручную (ts = 40 дней назад)
	old := time.Now().UTC().AddDate(0, 0, -40).Format("2006-01-02 15:04:05")
	if _, err := db.Exec(
		`INSERT INTO audit_log (ts, doctor_id, action, entity, entity_id, ip, detail)
		 VALUES (?, ?, ?, ?, ?, ?, NULL)`,
		old, "D2", "auth.logout", "auth", "", "2.2.2.2",
	); err != nil {
		t.Fatalf("insert old: %v", err)
	}

	// Cleanup 30 дней — старая удалится, свежая останется
	n, err := auditCleanup(db, 30)
	if err != nil {
		t.Fatalf("auditCleanup: %v", err)
	}
	if n != 1 {
		t.Fatalf("auditCleanup удалил %d, ожидали 1", n)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("после cleanup count=%d, ожидали 1 (свежая)", count)
	}

	// Невалидный days
	if _, err := auditCleanup(db, 0); err == nil {
		t.Fatalf("auditCleanup(0) должен вернуть ошибку")
	}
	if _, err := auditCleanup(db, -5); err == nil {
		t.Fatalf("auditCleanup(-5) должен вернуть ошибку")
	}
}

// ---------------------------------------------------------------
// 5b. auditActionClause — точное/префикс + экранирование
// ---------------------------------------------------------------

func TestAudit_ActionClause(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantClause string
		wantArg    string
	}{
		{"empty", "", "", ""},
		{"exact", "doctor.create", "action = ?", "doctor.create"},
		{"prefix", "doctor.*", `action LIKE ? ESCAPE '\'`, "doctor.%"},
		{"prefix2", "auth.*", `action LIKE ? ESCAPE '\'`, "auth.%"},
		{"escape_pct", "doc%.*", `action LIKE ? ESCAPE '\'`, `doc\%.%`},
		{"escape_us", "doc_.*", `action LIKE ? ESCAPE '\'`, `doc\_.%`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotClause, gotArgs := auditActionClause(c.in)
			if gotClause != c.wantClause {
				t.Fatalf("clause = %q, ожидали %q", gotClause, c.wantClause)
			}
			if c.wantClause == "" {
				if len(gotArgs) != 0 {
					t.Fatalf("args = %v, ожидали пусто", gotArgs)
				}
				return
			}
			if len(gotArgs) != 1 {
				t.Fatalf("len(args) = %d, ожидали 1", len(gotArgs))
			}
			if got := gotArgs[0].(string); got != c.wantArg {
				t.Fatalf("arg = %q, ожидали %q", got, c.wantArg)
			}
		})
	}
}

// B17_1_TESTS_APPLIED

// TestAudit_EntityClause — зеркало TestAudit_ActionClause для entity (B17.1).
func TestAudit_EntityClause(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantClause string
		wantArg    string
	}{
		{"empty", "", "", ""},
		{"exact", "doctor", "entity = ?", "doctor"},
		{"prefix", "doctor.*", `entity LIKE ? ESCAPE '\'`, "doctor.%"},
		{"prefix2", "auth.*", `entity LIKE ? ESCAPE '\'`, "auth.%"},
		{"escape_pct", "doc%.*", `entity LIKE ? ESCAPE '\'`, `doc\%.%`},
		{"escape_us", "doc_.*", `entity LIKE ? ESCAPE '\'`, `doc\_.%`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotClause, gotArgs := auditEntityClause(c.in)
			if gotClause != c.wantClause {
				t.Fatalf("clause = %q, ожидали %q", gotClause, c.wantClause)
			}
			if c.wantClause == "" {
				if len(gotArgs) != 0 {
					t.Fatalf("args = %v, ожидали пусто", gotArgs)
				}
				return
			}
			if len(gotArgs) != 1 {
				t.Fatalf("len(args) = %d, ожидали 1", len(gotArgs))
			}
			if got := gotArgs[0].(string); got != c.wantArg {
				t.Fatalf("arg = %q, ожидали %q", got, c.wantArg)
			}
		})
	}
}

// TestAudit_EntityPrefix_Filter — интеграция: auditEntityClause("doctor.*")
// отбирает только записи entity='doctor' (B17.1).
func TestAudit_EntityPrefix_Filter(t *testing.T) {
	db := newTestDB(t)

	// Entity-значения с точкой — реалистичный сценарий для префиксного
	// фильтра (entity=doctor.*). В текущей системе entity бывают и
	// однословными (doctor, auth, patient), но фильтр '*' — это LIKE-prefix,
	// поэтому для проверки нужны составные значения.
	auditLog(db, "D1", "doctor.create", "doctor.profile", "DOC-1", "1.1.1.1", nil)
	auditLog(db, "D1", "doctor.update", "doctor.schedule", "DOC-1", "1.1.1.1", nil)
	auditLog(db, "D1", "patient.view", "patient.card", "PAT-1", "1.1.1.1", nil)
	auditLog(db, "D1", "auth.login", "auth.session", "", "1.1.1.1", nil)

	clause, args := auditEntityClause("doctor.*")
	if clause == "" {
		t.Fatal("пустой clause для doctor.*")
	}
	q := `SELECT entity FROM audit_log WHERE ` + clause
	rows, err := db.Query(q, args...)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, e)
	}
	if len(got) != 2 || got[0] != "doctor.profile" || got[1] != "doctor.schedule" {
		t.Fatalf("получено %v, ожидали [doctor.profile doctor.schedule]", got)
	}
}

// ---------------------------------------------------------------
// 6. Порядок — свежая первая
// ---------------------------------------------------------------

func TestAudit_Order(t *testing.T) {
	db := newTestDB(t)

	auditLog(db, "D1", "auth.login", "auth", "", "1.1.1.1", nil)
	auditLog(db, "D1", "doctor.create", "doctor", "DOC-9", "1.1.1.1", nil)
	auditLog(db, "D1", "auth.logout", "auth", "", "1.1.1.1", nil)

	rows, err := db.Query(`SELECT action FROM audit_log ORDER BY id DESC LIMIT 3`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, a)
	}
	if len(got) != 3 {
		t.Fatalf("получено %d, ожидали 3", len(got))
	}
	if got[0] != "auth.logout" || got[1] != "doctor.create" || got[2] != "auth.login" {
		t.Fatalf("порядок неверен: %v", got)
	}
}

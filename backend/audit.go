package main

// audit.go — подпатч 6.18 (B9): журнал действий (audit-log).
//
// Best-effort: ошибки записи только логируются, никогда не влияют
// на HTTP-ответ. Вызывающий код продолжает работу.
//
// Схема: id, ts, doctor_id, action, entity, entity_id, ip, detail (JSON).
// ts формируется в Go (time.Now().UTC()) — так избегаем зависимости
// от SQLite datetime('now') и легко тестируем.

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// auditLog пишет запись в audit_log (best-effort).
//   - doctorID — кто (может быть пусто для анонимных действий);
//   - action — тип действия (auth.login, doctor.delete, ...);
//   - entity — сущность (auth, doctor, patient, ...);
//   - entityID — ID объекта;
//   - ip — IP клиента (clientIP(r));
//   - detail — любая структура (сериализуется в JSON) или nil.
func auditLog(db *sql.DB, doctorID, action, entity, entityID, ip string, detail any) {
	if db == nil || action == "" {
		return
	}
	var detailJSON sql.NullString
	if detail != nil {
		if b, err := json.Marshal(detail); err == nil {
			detailJSON.String = string(b)
			detailJSON.Valid = true
		}
	}
	_, err := db.Exec(
		`INSERT INTO audit_log (ts, doctor_id, action, entity, entity_id, ip, detail)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now().UTC().Format("2006-01-02 15:04:05"),
		nullIfEmpty(doctorID), action, nullIfEmpty(entity), nullIfEmpty(entityID),
		nullIfEmpty(ip), detailJSON,
	)
	if err != nil {
		slog.Warn("audit: insert", "action", action, "entityId", entityID, "err", err)
	}
}

// auditRow — запись из audit_log для JSON-ответа.
type auditRow struct {
	ID       int64  `json:"id"`
	Ts       string `json:"ts"`
	DoctorID string `json:"doctorId,omitempty"`
	Action   string `json:"action"`
	Entity   string `json:"entity,omitempty"`
	EntityID string `json:"entityId,omitempty"`
	IP       string `json:"ip,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// registerAuditRoutes — GET /api/audit (только admin/superadmin).
func registerAuditRoutes() {
	guard := requireRole(RoleAdmin, RoleSuperadmin)
	http.Handle("/api/audit", guard(http.HandlerFunc(handleAuditList)))
	slog.Info("registerAuditRoutes", "route", "/api/audit")
}

// auditActionClause строит SQL-условие для фильтра action.
//
// B16 (6.25): поддержка префиксного поиска через '*'.
//   - "doctor.create"   → action = ?
//   - "doctor.*"        → action LIKE ? ESCAPE '\'  (prefix = "doctor.%")
//
// Знаки '%', '_' и '\' в prefix экранируются, чтобы не трактовались
// как wildcards.
func auditActionClause(action string) (string, []any) {
	action = strings.TrimSpace(action)
	if action == "" {
		return "", nil
	}
	if !strings.HasSuffix(action, "*") {
		return "action = ?", []any{action}
	}
	prefix := strings.TrimSuffix(action, "*")
	repl := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	pattern := repl.Replace(prefix) + "%"
	return `action LIKE ? ESCAPE '\'`, []any{pattern}
}
// handleAuditList — GET /api/audit.
// Query:
//   - limit=N            (default 100, max 1000)
//   - doctor_id=...      фильтр по врачу
//   - action=...         фильтр по типу действия
//   - entity=...         фильтр по сущности
//   - since=YYYY-MM-DD   ts >= since
//   - until=YYYY-MM-DD   ts <= until
func handleAuditList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()

	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	where := []string{}
	args := []any{}

	if v := strings.TrimSpace(q.Get("doctor_id")); v != "" {
		where = append(where, "doctor_id = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("action")); v != "" {
		clause, cargs := auditActionClause(v)
		if clause != "" {
			where = append(where, clause)
			args = append(args, cargs...)
		}
	}
	if v := strings.TrimSpace(q.Get("entity")); v != "" {
		where = append(where, "entity = ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("since")); v != "" {
		where = append(where, "ts >= ?")
		args = append(args, v)
	}
	if v := strings.TrimSpace(q.Get("until")); v != "" {
		where = append(where, "ts <= ?")
		args = append(args, v)
	}

	sqlQ := `SELECT id, ts, doctor_id, action, entity, entity_id, ip, detail FROM audit_log`
	if len(where) > 0 {
		sqlQ += " WHERE " + strings.Join(where, " AND ")
	}
	sqlQ += " ORDER BY id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(sqlQ, args...)
	if err != nil {
		slog.Warn("GET /api/audit", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	out := []auditRow{}
	for rows.Next() {
		var (
			row      auditRow
			docID    sql.NullString
			entity   sql.NullString
			entityID sql.NullString
			ip       sql.NullString
			detail   sql.NullString
		)
		if err := rows.Scan(&row.ID, &row.Ts, &docID, &row.Action, &entity, &entityID, &ip, &detail); err != nil {
			slog.Warn("GET /api/audit scan", "err", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
			return
		}
		if docID.Valid {
			row.DoctorID = docID.String
		}
		if entity.Valid {
			row.Entity = entity.String
		}
		if entityID.Valid {
			row.EntityID = entityID.String
		}
		if ip.Valid {
			row.IP = ip.String
		}
		if detail.Valid {
			row.Detail = detail.String
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		slog.Warn("GET /api/audit rows", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("GET /api/audit encode", "err", err)
	}
}

// auditCleanup — задел на retention (не вызывается в B9).
func auditCleanup(db *sql.DB, days int) (int64, error) {
	if days <= 0 {
		return 0, fmt.Errorf("days must be > 0")
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05")
	res, err := db.Exec(`DELETE FROM audit_log WHERE ts < ?`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("audit cleanup: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

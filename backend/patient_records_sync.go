package main

// patient_records_sync.go — B30.17 Шаг 3a: batch-sync doctor-write для
// symptoms/allergies/vaccinations/diagnoses (PENDING queue на клиенте).
//
// Контракт:
//
//   POST /api/patients/{id}/records/sync   (doctor-only)
//   {"ops":[{"client_op_id":"...","kind":"symptoms","action":"create|update","payload":{...}}]}
//
//   200 {"results":[{"client_op_id":"...","status":"ok|dup|conflict|invalid","record_id":"..."}]}
//   403 {"error":"doctor only"}         — нет doctor-контекста
//   403 {"error":"consent_required"}    — нет approved <kind>_write хотя бы для одного kind
//   400 {"error":"invalid json"}        — битый JSON / пустой ops
//   400 {"error":"batch_too_large"}     — > maxSyncOps
//   405 {"error":"method not allowed"}
//
// Идемпотентность: UNIQUE(patient_id, client_op_id) per table (partial, миграция в db.go).
// Конфликты: last-write-wins по updated_at (RFC3339 UTC, строковое сравнение корректно).
// Невалидные op не валят батч — помечаются "invalid" в results (частичный успех).
// Consent проверяется атомарно по множеству kind'ов до транзакции.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const maxSyncOps = 100

// syncKinds — kind'ы, участвующие в sync (только *_write из Шага 1).
var syncKinds = map[string]bool{
	"symptoms":     true,
	"allergies":    true,
	"vaccinations": true,
	"diagnoses":    true,
}

// syncOp — одна операция из батча.
type syncOp struct {
	ClientOpID string         `json:"client_op_id"`
	Kind       string         `json:"kind"`
	Action     string         `json:"action"` // create|update
	Payload    map[string]any `json:"payload"`
}

// syncOpIndexed — валидная op с индексом в исходном массиве.
type syncOpIndexed struct {
	idx int
	op  syncOp
}

// syncResult — per-op результат.
type syncResult struct {
	ClientOpID string `json:"client_op_id"`
	Status     string `json:"status"` // ok|dup|conflict|invalid
	RecordID   string `json:"record_id,omitempty"`
}

// syncRequest — тело запроса.
type syncRequest struct {
	Ops []syncOp `json:"ops"`
}

// handlePatientRecordsSync — POST /api/patients/{id}/records/sync.
func handlePatientRecordsSync(w http.ResponseWriter, r *http.Request, patientID string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	doctorID := doctorIDFromCtx(r)
	if doctorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusForbidden)
		return
	}
	if patientID == "" {
		http.Error(w, `{"error":"patient id required"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return
	}
	var req syncRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if len(req.Ops) == 0 {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if len(req.Ops) > maxSyncOps {
		http.Error(w, `{"error":"batch_too_large"}`, http.StatusBadRequest)
		return
	}

	// 1. Валидация + сбор kind'ов (для consent).
	results, valids := validateSyncOps(req.Ops)
	kindSet := map[string]bool{}
	for _, v := range valids {
		kindSet[v.op.Kind] = true
	}

	// 2. Consent атомарно: для каждого kind из валидных op нужен approved <kind>_write.
	if len(valids) > 0 {
		for kind := range kindSet {
			_, _, err := findApprovedConsent(patientID, doctorID, kind+"_write")
			if errors.Is(err, sql.ErrNoRows) {
				consentAudit(db, "", patientID, doctorID, "write_denied", "doctor", doctorID, clientIP(r))
				http.Error(w, `{"error":"consent_required"}`, http.StatusForbidden)
				return
			}
			if err != nil {
				slog.Warn("records.sync: findApprovedConsent", "kind", kind, "err", err)
				http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
				return
			}
		}
	}

	// 3. Применяем батч общей функцией (та же логика для self и doctor).
	applySyncBatch(w, patientID, valids, results)

	auditLog(db, doctorID, "records.sync", "patient", patientID, clientIP(r), nil)
}

// handlePatientRecordsSyncSelf — POST /api/patient/records/sync (self).
// Та же логика, что doctor-путь, но без consent: пациент пишет свои записи.
func handlePatientRecordsSyncSelf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	pid, err := resolvePatientIDForSelf(r)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"patient account not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	if err != nil {
		http.Error(w, `{"error":"read body"}`, http.StatusBadRequest)
		return
	}
	var req syncRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if len(req.Ops) == 0 {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if len(req.Ops) > maxSyncOps {
		http.Error(w, `{"error":"batch_too_large"}`, http.StatusBadRequest)
		return
	}

	results, valids := validateSyncOps(req.Ops)
	applySyncBatch(w, pid, valids, results)
}

// applySyncBatch — общая часть: одна tx, per-op результаты, ответ.
// valids уже отфильтрованы, results — слоты под все op (invalid заполнены).
func applySyncBatch(w http.ResponseWriter, patientID string, valids []syncOpIndexed, results []syncResult) {
	tx, err := db.Begin()
	if err != nil {
		slog.Warn("records.sync: begin", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	for _, v := range valids {
		res := applySyncOp(tx, patientID, v.op)
		results[v.idx] = res
	}

	if err := tx.Commit(); err != nil {
		slog.Warn("records.sync: commit", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"results": results})
}

// validateSyncOps — общая валидация: невалидные -> results как "invalid",
// валидные -> valids (с idx в исходном массиве).
func validateSyncOps(ops []syncOp) (results []syncResult, valids []syncOpIndexed) {
	results = make([]syncResult, 0, len(ops))
	valids = make([]syncOpIndexed, 0, len(ops))
	for i, op := range ops {
		if !validSyncOp(op) {
			results = append(results, syncResult{ClientOpID: op.ClientOpID, Status: "invalid"})
			continue
		}
		valids = append(valids, syncOpIndexed{idx: i, op: op})
		results = append(results, syncResult{ClientOpID: op.ClientOpID, Status: ""})
	}
	return results, valids
}

// validSyncOp — базовая валидация op до записи.
func validSyncOp(op syncOp) bool {
	if strings.TrimSpace(op.ClientOpID) == "" {
		return false
	}
	if !syncKinds[op.Kind] {
		return false
	}
	if op.Action != "create" && op.Action != "update" && op.Action != "delete" {
		return false
	}
	if op.Payload == nil {
		return false
	}
	if rid, _ := op.Payload["id"].(string); strings.TrimSpace(rid) == "" {
		return false
	}
	return true
}

// applySyncOp — применяет одну op в tx. Возвращает результат.
//
//	create: INSERT ... ON CONFLICT(patient_id, client_op_id) DO NOTHING RETURNING id;
//	        пусто -> dup (достаём существующий id по индексу).
//	update: LWW UPDATE ... WHERE updated_at < ? RETURNING id;
//	        пусто -> conflict (серверная новее или запись не найдена).
func applySyncOp(tx *sql.Tx, patientID string, op syncOp) syncResult {
	rid := op.Payload["id"].(string)
	now := time.Now().UTC().Format(time.RFC3339)

	switch op.Action {
	case "create":
		id, dup, err := insertSyncCreate(tx, patientID, op.Kind, rid, op.ClientOpID, op.Payload, now)
		if err != nil {
			slog.Warn("records.sync: create", "kind", op.Kind, "err", err)
			return syncResult{ClientOpID: op.ClientOpID, Status: "invalid"}
		}
		if dup {
			return syncResult{ClientOpID: op.ClientOpID, Status: "dup", RecordID: id}
		}
		return syncResult{ClientOpID: op.ClientOpID, Status: "ok", RecordID: id}

	case "update":
		id, ok, err := updateSyncLWW(tx, patientID, op.Kind, rid, op.Payload, now)
		if err != nil {
			slog.Warn("records.sync: update", "kind", op.Kind, "err", err)
			return syncResult{ClientOpID: op.ClientOpID, Status: "invalid"}
		}
		if !ok {
			return syncResult{ClientOpID: op.ClientOpID, Status: "conflict", RecordID: rid}
		}
		return syncResult{ClientOpID: op.ClientOpID, Status: "ok", RecordID: id}

	case "delete":
		id, ok, err := softDeleteLWW(tx, patientID, op.Kind, rid, now)
		if err != nil {
			slog.Warn("records.sync: delete", "kind", op.Kind, "err", err)
			return syncResult{ClientOpID: op.ClientOpID, Status: "invalid"}
		}
		if !ok {
			return syncResult{ClientOpID: op.ClientOpID, Status: "conflict", RecordID: rid}
		}
		return syncResult{ClientOpID: op.ClientOpID, Status: "ok", RecordID: id}
	}
	return syncResult{ClientOpID: op.ClientOpID, Status: "invalid"}
}

// insertSyncCreate — INSERT с ON CONFLICT на partial UNIQUE(patient_id, client_op_id).
// Возвращает (recordID, dup, err). dup=true — запись уже была (replay).
func insertSyncCreate(tx *sql.Tx, patientID, kind, rid, clientOpID string, payload map[string]any, now string) (string, bool, error) {
	clinic, err := clinicIDForPatientTx(tx, patientID)
	if err != nil && err != sql.ErrNoRows {
		return "", false, err
	}

	cols := []string{"id", "patient_id", "clinic_id", "client_op_id", "created_at", "updated_at"}
	vals := []any{rid, patientID, clinic, clientOpID, now, now}
	phs := []string{"?", "?", "?", "?", "?", "?"}

	for _, f := range recordFields[kind] {
		v, ok := payload[f]
		if !ok {
			continue
		}
		cols = append(cols, f)
		vals = append(vals, v)
		phs = append(phs, "?")
	}

	q := `INSERT INTO ` + kind + ` (` + strings.Join(cols, ",") + `) VALUES (` +
		strings.Join(phs, ",") + `)
		ON CONFLICT(patient_id, client_op_id) WHERE client_op_id IS NOT NULL DO NOTHING
		RETURNING id`

	var outID string
	err = tx.QueryRow(q, vals...).Scan(&outID)
	if err == nil {
		return outID, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", false, err
	}
	// Конфликт: запись уже есть (replay). Достаём id по индексу.
	err = tx.QueryRow(`SELECT id FROM `+kind+` WHERE patient_id=? AND client_op_id=?`,
		patientID, clientOpID).Scan(&outID)
	if err != nil {
		return "", false, err
	}
	return outID, true, nil
}

// updateSyncLWW — LWW-update: применяем только если серверная updated_at < now.
// Возвращает (recordID, applied, err). applied=false — конфликт (серверная новее/нет записи).
func updateSyncLWW(tx *sql.Tx, patientID, kind, rid string, payload map[string]any, now string) (string, bool, error) {
	sets := []string{"updated_at=?"}
	vals := []any{now}

	for _, f := range recordFields[kind] {
		v, ok := payload[f]
		if !ok {
			continue
		}
		sets = append(sets, f+"=?")
		vals = append(vals, v)
	}

	q := `UPDATE ` + kind + ` SET ` + strings.Join(sets, ",") +
		` WHERE id=? AND patient_id=? AND deleted_at IS NULL AND updated_at < ?
		RETURNING id`
	vals = append(vals, rid, patientID, now)

	var outID string
	err := tx.QueryRow(q, vals...).Scan(&outID)
	if err == nil {
		return outID, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return rid, false, nil // конфликт: серверная новее или записи нет
	}
	return "", false, err
}

// clinicIDForPatientTx — clinic_id пациента внутри tx (nil-safe).
// softDeleteLWW — LWW soft-delete: ставит deleted_at только если серверная
// updated_at < now и запись ещё жива. Возвращает (recordID, applied, err).
// applied=false — конфликт (уже удалено / серверная новее / нет записи).
func softDeleteLWW(tx *sql.Tx, patientID, kind, rid, now string) (string, bool, error) {
	q := `UPDATE ` + kind + ` SET deleted_at=?, updated_at=?
		WHERE id=? AND patient_id=? AND deleted_at IS NULL AND updated_at < ?
		RETURNING id`
	var outID string
	err := tx.QueryRow(q, now, now, rid, patientID, now).Scan(&outID)
	if err == nil {
		return outID, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return rid, false, nil
	}
	return "", false, err
}

func clinicIDForPatientTx(tx *sql.Tx, pid string) (any, error) {
	var c sql.NullString
	err := tx.QueryRow(`SELECT clinic_id FROM patients WHERE id=?`, pid).Scan(&c)
	if err != nil {
		return nil, err
	}
	if c.Valid {
		return c.String, nil
	}
	return nil, nil
}

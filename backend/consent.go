package main

// consent.go — B30.3 (вариант B): consent-контур patient <-> doctor.
//
// Пациент даёт/отзывает согласие на чтение vitals, врач запрашивает.
// Статусы:  pending | approved | denied | revoked
// Действия: request | approve | deny | revoke
//
// Роуты:
//   POST /api/consents/request        — врач создаёт pending
//   POST /api/consents/{id}/approve   — пациент: pending -> approved
//   POST /api/consents/{id}/deny      — пациент: pending -> denied
//   POST /api/consents/{id}/revoke    — обе стороны: approved -> revoked
//   GET  /api/consents                — список (свои)

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ErrConsentConflict — целевой статус не совпал с текущим (гонка или
// недопустимый переход). Хендлеры отдают на это 409.
var ErrConsentConflict = errors.New("consent status conflict")

// ---------------------------------------------------------------
// Модель
// ---------------------------------------------------------------

type Consent struct {
	ID          string `json:"id"`
	PatientID   string `json:"patientId"`
	DoctorID    string `json:"doctorId"`
	Scope       string `json:"scope"`
	Status      string `json:"status"`
	RequestedAt string `json:"requestedAt"`
	ApprovedAt  string `json:"approvedAt,omitempty"`
	RevokedAt   string `json:"revokedAt,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
}

// ---------------------------------------------------------------
// Аудит (append-only consents_audit)
// ---------------------------------------------------------------

// consentAudit пишет запись в consents_audit (best-effort).
// По образцу auditLog: ошибки только логируются, никогда не влияют
// на HTTP-ответ.
//
//	action:    request | approve | deny | revoke
//	actorType: patient | doctor | admin
func consentAudit(db *sql.DB, consentID, patientID, doctorID, action, actorType, actorID, ip string) {
	if db == nil || consentID == "" || action == "" {
		return
	}
	auditID, err := newID("CAUD")
	if err != nil {
		slog.Warn("consent audit: newID", "err", err)
		return
	}
	_, err = db.Exec(
		`INSERT INTO consents_audit
		   (id, consent_id, patient_id, doctor_id, action, actor_type, actor_id, ip, created)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		auditID,
		consentID, patientID, doctorID, action, actorType,
		nullIfEmpty(actorID), nullIfEmpty(ip),
		time.Now().UTC().Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		slog.Warn("consent audit: insert", "action", action, "consentId", consentID, "err", err)
	}
}

// ---------------------------------------------------------------
// Регистратор
// ---------------------------------------------------------------

// registerConsentRoutes регистрирует роуты для consents.
func registerConsentRoutes() {
	http.HandleFunc("/api/consents", handleConsents)
	http.HandleFunc("/api/consents/", handleConsentByID)
}

// ---------------------------------------------------------------
// Диспетчеры
// ---------------------------------------------------------------

// handleConsents обрабатывает /api/consents:
//
//	GET — список (свои: пациент -> свои, врач -> свои).
func handleConsents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleConsentsList(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleConsentByID разбирает /api/consents/{id}/{action}.
// Паттерн 1:1 с handlePatientByID (patients.go).
func handleConsentByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/consents/")
	if path == "" {
		http.Error(w, `{"error":"id is required"}`, http.StatusBadRequest)
		return
	}

	parts := strings.Split(path, "/")
	id := parts[0]

	// /api/consents/request — action без id.
	if id == "request" && len(parts) == 1 {
		handleConsentRequest(w, r)
		return
	}

	// /api/consents/{id}/{action}
	if len(parts) == 2 {
		switch parts[1] {
		case "approve":
			handleConsentApprove(w, r, id)
			return
		case "deny":
			handleConsentDeny(w, r, id)
			return
		case "revoke":
			handleConsentRevoke(w, r, id)
			return
		}
	}

	http.NotFound(w, r)
}

// ---------------------------------------------------------------
// Хендлеры
// ---------------------------------------------------------------

// roleOf возвращает роль запроса для логов/метрик: patient|doctor|anonymous.
func roleOf(r *http.Request) string {
	if patientIDFromCtx(r) != "" {
		return "patient"
	}
	if doctorIDFromCtx(r) != "" {
		return "doctor"
	}
	return "anonymous"
}

// handleConsentsList — GET /api/consents.
// Пациент получает свои согласия, врач — свои. Роль из контекста.
func handleConsentsList(w http.ResponseWriter, r *http.Request) {
	role := roleOf(r)
	slog.Info("consents list", "role", role)

	var (
		list []Consent
		err  error
	)
	switch role {
	case "patient":
		list, err = listConsentsForPatient(db, patientIDFromCtx(r))
	case "doctor":
		list, err = listConsentsForDoctor(db, doctorIDFromCtx(r))
	default:
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}
	if err != nil {
		slog.Warn("consents list: query", "role", role, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// handleConsentRequest — POST /api/consents/request.
// Тело: {"patientId":"...","scope":"vitals_read"} (doctor-only).
func handleConsentRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	doctorID := doctorIDFromCtx(r)
	if doctorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusForbidden)
		return
	}
	var req struct {
		PatientID string `json:"patientId"`
		Scope     string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.PatientID == "" {
		http.Error(w, `{"error":"patientId required"}`, http.StatusBadRequest)
		return
	}
	if req.Scope == "" {
		req.Scope = "vitals_read"
	}
	// B-b: проверяем, что пациент существует (иначе FK уронит в 500).
	if _, err := getPatientByID(db, req.PatientID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
			return
		}
		slog.Warn("consent request: getPatientByID", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	id, err := upsertConsentRequest(db, req.PatientID, doctorID, req.Scope)
	if err != nil {
		slog.Warn("consent request: upsert", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	consentAudit(db, id, req.PatientID, doctorID, "request", "doctor", doctorID, clientIP(r))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "pending"})
	slog.Info("consent request", "id", id, "patientId", req.PatientID, "doctorId", doctorID)
}

// handleConsentApprove — POST /api/consents/{id}/approve (patient-only).
func handleConsentApprove(w http.ResponseWriter, r *http.Request, id string) {
	handleConsentTransition(w, r, id, "approve", "pending", "approved", "patient")
}

// handleConsentDeny — POST /api/consents/{id}/deny (patient-only).
func handleConsentDeny(w http.ResponseWriter, r *http.Request, id string) {
	handleConsentTransition(w, r, id, "deny", "pending", "denied", "patient")
}

// handleConsentRevoke — POST /api/consents/{id}/revoke (patient или doctor).
func handleConsentRevoke(w http.ResponseWriter, r *http.Request, id string) {
	handleConsentTransition(w, r, id, "revoke", "approved", "revoked", "")
}

// handleConsentTransition — общая логика переходов approve/deny/revoke.
//
//	requiredRole == "" — любая аутентифицированная сторона (patient/doctor).
func handleConsentTransition(w http.ResponseWriter, r *http.Request, id, action, fromStatus, toStatus, requiredRole string) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var actorType, actorID string
	pid := patientIDFromCtx(r)
	did := doctorIDFromCtx(r)
	switch requiredRole {
	case "patient":
		if pid == "" {
			http.Error(w, `{"error":"patient only"}`, http.StatusForbidden)
			return
		}
		actorType, actorID = "patient", pid
	default: // revoke: обе стороны
		if pid != "" {
			actorType, actorID = "patient", pid
		} else if did != "" {
			actorType, actorID = "doctor", did
		} else {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
	}

	c, err := getConsent(db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		slog.Warn("consent transition: get", "id", id, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if actorType == "patient" && c.PatientID != actorID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if actorType == "doctor" && c.DoctorID != actorID {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	if err := transitionConsent(db, id, fromStatus, toStatus); err != nil {
		if errors.Is(err, ErrConsentConflict) {
			http.Error(w, `{"error":"status conflict"}`, http.StatusConflict)
			return
		}
		slog.Warn("consent transition: update", "id", id, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	consentAudit(db, id, c.PatientID, c.DoctorID, action, actorType, actorID, clientIP(r))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id, "status": toStatus})
	slog.Info("consent "+action, "id", id, "actor", actorType)
}

// ---------------------------------------------------------------
// DB-операции
// ---------------------------------------------------------------

// upsertConsentRequest — INSERT ON CONFLICT (patient_id, doctor_id, scope).
// Решение 1a: повторный request (в т.ч. после denied) → pending,
// approved_at/revoked_at обнуляются.
func upsertConsentRequest(db *sql.DB, patientID, doctorID, scope string) (string, error) {
	id, err := newID("CONS")
	if err != nil {
		return "", fmt.Errorf("newID: %w", err)
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = db.Exec(
		`INSERT INTO consents
		   (id, patient_id, doctor_id, scope, status, requested_at)
		 VALUES (?, ?, ?, ?, 'pending', ?)
		 ON CONFLICT(patient_id, doctor_id, scope) DO UPDATE SET
		   status       = 'pending',
		   requested_at = excluded.requested_at,
		   approved_at  = NULL,
		   revoked_at   = NULL`,
		id, patientID, doctorID, scope, now,
	)
	if err != nil {
		return "", fmt.Errorf("upsert consents: %w", err)
	}
	var actualID string
	if err := db.QueryRow(
		`SELECT id FROM consents WHERE patient_id=? AND doctor_id=? AND scope=?`,
		patientID, doctorID, scope,
	).Scan(&actualID); err != nil {
		return "", fmt.Errorf("select id after upsert: %w", err)
	}
	return actualID, nil
}

// transitionConsent — явные UPDATE по целевому статусу (A-b).
// ErrConsentConflict — если 0 rows affected (гонка или неверный from).
func transitionConsent(db *sql.DB, id, fromStatus, toStatus string) error {
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	var (
		res sql.Result
		err error
	)
	switch toStatus {
	case "approved":
		res, err = db.Exec(
			`UPDATE consents SET status='approved', approved_at=? WHERE id=? AND status=?`,
			now, id, fromStatus,
		)
	case "denied":
		res, err = db.Exec(
			`UPDATE consents SET status='denied', approved_at=NULL WHERE id=? AND status=?`,
			id, fromStatus,
		)
	case "revoked":
		res, err = db.Exec(
			`UPDATE consents SET status='revoked', revoked_at=? WHERE id=? AND status=?`,
			now, id, fromStatus,
		)
	default:
		return fmt.Errorf("transitionConsent: unknown toStatus %q", toStatus)
	}
	if err != nil {
		return fmt.Errorf("update consents: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrConsentConflict
	}
	return nil
}

// getConsent загружает один consent.
func getConsent(db *sql.DB, id string) (*Consent, error) {
	var (
		c          Consent
		approvedAt sql.NullString
		revokedAt  sql.NullString
		expiresAt  sql.NullString
	)
	err := db.QueryRow(
		`SELECT id, patient_id, doctor_id, scope, status, requested_at,
		        approved_at, revoked_at, expires_at
		   FROM consents WHERE id=?`, id,
	).Scan(&c.ID, &c.PatientID, &c.DoctorID, &c.Scope, &c.Status, &c.RequestedAt,
		&approvedAt, &revokedAt, &expiresAt)
	if err != nil {
		return nil, err
	}
	c.ApprovedAt = approvedAt.String
	c.RevokedAt = revokedAt.String
	c.ExpiresAt = expiresAt.String
	return &c, nil
}

// listConsentsForDoctor — список согласий врача (свежие первыми).
func listConsentsForDoctor(db *sql.DB, doctorID string) ([]Consent, error) {
	return queryConsents(db,
		`SELECT id, patient_id, doctor_id, scope, status, requested_at,
		        approved_at, revoked_at, expires_at
		   FROM consents WHERE doctor_id=? ORDER BY requested_at DESC`, doctorID)
}

// listConsentsForPatient — список согласий пациента (свежие первыми).
func listConsentsForPatient(db *sql.DB, patientID string) ([]Consent, error) {
	return queryConsents(db,
		`SELECT id, patient_id, doctor_id, scope, status, requested_at,
		        approved_at, revoked_at, expires_at
		   FROM consents WHERE patient_id=? ORDER BY requested_at DESC`, patientID)
}

// queryConsents — общий помощник для list-функций.
func queryConsents(db *sql.DB, query string, arg string) ([]Consent, error) {
	rows, err := db.Query(query, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Consent
	for rows.Next() {
		var (
			c          Consent
			approvedAt sql.NullString
			revokedAt  sql.NullString
			expiresAt  sql.NullString
		)
		if err := rows.Scan(&c.ID, &c.PatientID, &c.DoctorID, &c.Scope, &c.Status,
			&c.RequestedAt, &approvedAt, &revokedAt, &expiresAt); err != nil {
			return nil, err
		}
		c.ApprovedAt = approvedAt.String
		c.RevokedAt = revokedAt.String
		c.ExpiresAt = expiresAt.String
		out = append(out, c)
	}
	return out, rows.Err()
}

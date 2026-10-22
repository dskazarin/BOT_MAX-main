package main

// patient_records.go — B30.9c: self-records пациента (этап 2 §6.58).
//
// Сущности: symptoms, allergies, vaccinations, diagnoses.
// Модель — как patient_vitals.go:
//   patientIDFromCtx(r)   → patient_accounts.id (accountID)
//   resolvePatientIDForSelf → patients.id (pid)
//   clinic_id             → patients.clinic_id (снимок на момент записи)
//
// API (все под patientAuthMiddleware):
//   GET    /api/patient/records/{kind}       — список живых своих
//   POST   /api/patient/records/{kind}       — upsert по id (server-wins)
//   DELETE /api/patient/records/{kind}/{id}  — soft delete
//
// kind ∈ {symptoms, allergies, vaccinations, diagnoses}.
// Upsert по id: id — TEXT (клиентский Date.now()), created_at не меняется,
// updated_at = now. Soft delete: deleted_at = now, запись остаётся.

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

var recordTables = map[string]bool{
	"symptoms":     true,
	"allergies":    true,
	"vaccinations": true,
	"diagnoses":    true,
}

// recordKindFromPath — /api/patient/records/{kind}[/{id}] → kind, id.
func recordKindFromPath(p string) (kind, id string, ok bool) {
	const prefix = "/api/patient/records/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	if rest == "" {
		return "", "", false
	}
	parts := strings.SplitN(rest, "/", 2)
	kind = parts[0]
	if !recordTables[kind] {
		return "", "", false
	}
	if len(parts) == 2 {
		id = parts[1]
	}
	return kind, id, true
}

// clinicIDForPatient — patients.clinic_id (может быть NULL → "").
func clinicIDForPatient(pid string) (sql.NullString, error) {
	var c sql.NullString
	err := db.QueryRow(`SELECT clinic_id FROM patients WHERE id=?`, pid).Scan(&c)
	return c, err
}

// handlePatientRecords — общий диспатчер для 4 сущностей.
func handlePatientRecords(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := recordKindFromPath(r.URL.Path)
	if !ok {
		http.Error(w, `{"error":"unknown record kind"}`, http.StatusNotFound)
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

	switch r.Method {
	case http.MethodGet:
		if id != "" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		list, err := listPatientRecords(kind, pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)

	case http.MethodPost:
		if id != "" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		rid, err := upsertPatientRecord(kind, pid, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": rid})

	case http.MethodDelete:
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		n, err := softDeletePatientRecord(kind, pid, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if n == 0 {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// recordFields — белый список колонок по kind (кроме id/patient_id/clinic_id/
// created_at/updated_at/deleted_at — их ставим сами).
var recordFields = map[string][]string{
	"symptoms":     {"name", "severity", "onset", "notes"},
	"allergies":    {"allergen", "reaction", "severity", "notes"},
	"vaccinations": {"vaccine", "dose", "administered", "notes"},
	"diagnoses":    {"code", "title", "diagnosed_at", "status", "notes"},
}

func listPatientRecords(kind, pid string) ([]map[string]any, error) {
	cols := append([]string{"id"}, recordFields[kind]...)
	cols = append(cols, "created_at", "updated_at")
	q := `SELECT ` + strings.Join(cols, ",") + ` FROM ` + kind +
		` WHERE patient_id=? AND deleted_at IS NULL ORDER BY updated_at DESC`
	rows, err := db.Query(q, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func upsertPatientRecord(kind, pid string, payload map[string]any) (string, error) {
	rid, _ := payload["id"].(string)
	if strings.TrimSpace(rid) == "" {
		return "", errors.New("id required")
	}

	clinic, err := clinicIDForPatient(pid)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	fields := recordFields[kind]
	// Собираем значения по бе白ому списку; отсутствующие — nil.
	vals := make([]any, 0, len(fields)+5)
	vals = append(vals, rid, pid)
	var clinicVal any
	if clinic.Valid {
		clinicVal = clinic.String
	} else {
		clinicVal = nil
	}
	vals = append(vals, clinicVal)

	cols := []string{"id", "patient_id", "clinic_id"}
	placeholders := []string{"?", "?", "?"}
	updates := []string{}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range fields {
		v := payload[f]
		cols = append(cols, f)
		placeholders = append(placeholders, "?")
		vals = append(vals, v)
		updates = append(updates, f+"=excluded."+f)
	}
	cols = append(cols, "created_at", "updated_at")
	placeholders = append(placeholders, "?", "?")
	vals = append(vals, now, now)
	updates = append(updates, "updated_at=excluded.updated_at")

	q := `INSERT INTO ` + kind + ` (` + strings.Join(cols, ",") + `) VALUES (` +
		strings.Join(placeholders, ",") + `) ON CONFLICT(id) DO UPDATE SET ` +
		strings.Join(updates, ", ") +
		` WHERE ` + kind + `.patient_id = excluded.patient_id`
	if _, err := db.Exec(q, vals...); err != nil {
		return "", err
	}
	return rid, nil
}

func softDeletePatientRecord(kind, pid, id string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`UPDATE `+kind+` SET deleted_at=?, updated_at=? WHERE id=? AND patient_id=? AND deleted_at IS NULL`,
		now, now, id, pid)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// registerPatientRecordsRoutes — все 4 сущности под patientAuthMiddleware.
func registerPatientRecordsRoutes() {
	http.Handle("/api/patient/records/",
		patientAuthMiddleware(http.HandlerFunc(handlePatientRecords)))
	slog.Info("registerPatientRecordsRoutes",
		"routes", "/api/patient/records/{symptoms,allergies,vaccinations,diagnoses}")
}

// handleDoctorPatientRecords — GET /api/patients/{id}/{kind} для врача.
// Проверка consent: findApprovedConsent(pid, doctorID, kind+"_read").
//   - нет doctorID в ctx          → 401
//   - нет approved consent        → 403 consent_required
//   - consent истёк (expires_at)  → 403 consent_expired
//   - иначе                       → 200 + список живых (deleted_at IS NULL)
func handleDoctorPatientRecords(w http.ResponseWriter, r *http.Request, pid, kind string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doctorID := doctorIDFromCtx(r)
	if doctorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusUnauthorized)
		return
	}

	// Пациент существует?
	if _, err := getPatientByID(db, pid); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scope := kind + "_read"
	_, expiresAt, err := findApprovedConsent(pid, doctorID, scope)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"consent_required"}`, http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if expired, bad := checkConsentExpired(expiresAt); bad {
		http.Error(w, `{"error":"consent_bad_expires"}`, http.StatusInternalServerError)
		return
	} else if expired {
		http.Error(w, `{"error":"consent_expired"}`, http.StatusForbidden)
		return
	}

	list, err := listPatientRecords(kind, pid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

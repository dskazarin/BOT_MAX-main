package main

// patient_vitals.go — B30.5: self-vitals пациента.
//
// Модель:
//   Пациент ведёт свои vitals сам. Резолв:
//     patientIDFromCtx(r)   → patient_accounts.id (patientAccountID)
//     getPatientAccountByID → acc.PatientID (patients.id)
//   Дальше — переиспользуем врачебные CRUD из db.go:
//     getVitals / upsertVitals / appendVitalsHistory / getVitalsHistory.
//
// Формат времени — RFC3339 (как пишет db.go:upsertVitals/appendVitalsHistory).
// tsLayout из B30.4 сюда НЕ применяется — он про consents.expires_at.
//
// Middleware: patientAuthMiddleware (hard-mode) в registerPatientVitalsRoutes.

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
)

// resolvePatientIDForSelf — patientAccountID (из ctx) → patients.id.
// sql.ErrNoRows, если аккаунт не найден или ctx пуст.
func resolvePatientIDForSelf(r *http.Request) (string, error) {
	accID := patientIDFromCtx(r)
	if accID == "" {
		return "", sql.ErrNoRows
	}
	acc, err := getPatientAccountByID(db, accID)
	if err != nil {
		return "", err
	}
	return acc.PatientID, nil
}

// handlePatientVitals — GET / PUT /api/patient/vitals.
func handlePatientVitals(w http.ResponseWriter, r *http.Request) {
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
		bp, temp, spo2, updated, err := getVitals(db, pid)
		if err == sql.ErrNoRows {
			// Нет записи — отдаём пустой объект (не 404): фронт
			// сам решит, показывать ли fallback на localStorage.
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"bp": "", "temp": 0, "spo2": 0, "updated": "",
			})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"bp": bp, "temp": temp, "spo2": spo2, "updated": updated,
		})

	case http.MethodPut:
		var body vitalsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := upsertVitals(db, pid, body.BP, body.Temp, body.Spo2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := appendVitalsHistory(db, pid, body.BP, body.Temp, body.Spo2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePatientVitalsHistory — GET /api/patient/vitals/history?limit=N.
// limit default 50, cap 500 (симметрия с B30.4).
func handlePatientVitalsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 500 {
		limit = 500
	}

	records, err := getVitalsHistory(db, pid, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// registerPatientVitalsRoutes — self-vitals за patientAuthMiddleware.
func registerPatientVitalsRoutes() {
	http.Handle("/api/patient/vitals",
		patientAuthMiddleware(http.HandlerFunc(handlePatientVitals)))
	http.Handle("/api/patient/vitals/history",
		patientAuthMiddleware(http.HandlerFunc(handlePatientVitalsHistory)))
	slog.Info("registerPatientVitalsRoutes",
		"routes", "/api/patient/vitals, /api/patient/vitals/history")
}

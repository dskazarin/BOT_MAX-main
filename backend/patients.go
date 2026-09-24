package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Patient — структура пациента для API и SQLite.
// Соответствует полям схемы БД (без Symptoms и AIResult — добавим в шаг 4).
type Patient struct {
	ID         string          `json:"id"`
	DoctorID   string          `json:"doctorId"`
	ClinicID   string          `json:"clinicId,omitempty"`
	Name       string          `json:"name"`
	Age        int             `json:"age"`
	AgeMonths  int             `json:"ageMonths,omitempty"`
	AgeDisplay string          `json:"ageDisplay,omitempty"`
	BirthDate  string          `json:"birthDate,omitempty"`
	Gender     string          `json:"gender,omitempty"`
	Height     float64         `json:"height,omitempty"`
	Weight     float64         `json:"weight,omitempty"`
	Phone      string          `json:"phone,omitempty"`
	Email      string          `json:"email,omitempty"`
	Status     string          `json:"status,omitempty"`
	Diagnosis  string          `json:"diagnosis,omitempty"`
	AIChecked  bool            `json:"aiChecked,omitempty"`
	Symptoms   json.RawMessage `json:"symptoms,omitempty"`
	AIResult   json.RawMessage `json:"aiResult,omitempty"`
	Created    string          `json:"created,omitempty"`
	LastSync   string          `json:"lastSync,omitempty"`
}

// stubDoctorID — заглушка вместо авторизации (шаг 6 — JWT).
const stubDoctorID = "DOC-001"

// registerPatientRoutes регистрирует роуты для patients.
// Два роута: /api/patients (GET list, POST create) и /api/patients/{id} (GET one).
func registerPatientRoutes() {
	http.HandleFunc("/api/patients", handlePatients)
	http.HandleFunc("/api/patients/", handlePatientByID)
}

// handlePatients обрабатывает GET (список пациентов) и POST (создание).
func handlePatients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handlePatientsList(w, r)
	case http.MethodPost:
		handlePatientCreate(w, r)
	default:
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
	}
}

// handlePatientsList возвращает список пациентов текущего врача.
func handlePatientsList(w http.ResponseWriter, r *http.Request) {
	docID, ok := resolveDoctorID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	patients, err := getPatientsByDoctor(db, docID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(patients)
}

// handlePatientCreate создаёт нового пациента.
func handlePatientCreate(w http.ResponseWriter, r *http.Request) {
	var p Patient
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if p.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if p.Gender != "" && p.Gender != "male" && p.Gender != "female" && p.Gender != "other" {
		http.Error(w, "gender must be male, female or other", http.StatusBadRequest)
		return
	}
	docID, ok := resolveDoctorID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	p.DoctorID = docID
	if p.ID == "" {
		p.ID = "PAT-" + time.Now().Format("20060102-150405")
	}
	if p.Created == "" {
		p.Created = time.Now().UTC().Format(time.RFC3339)
	}
	if err := createPatient(db, &p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(p)
}

// handlePatientByID возвращает одного пациента по ID из URL.
func handlePatientByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/patients/")
	if path == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}

	// Разбор подпутей: /{id}/vitals, /{id}/vitals/history, /{id}/cards
	parts := strings.Split(path, "/")
	id := parts[0]

	if len(parts) >= 2 {
		switch parts[1] {
		case "vitals":
			if len(parts) == 2 {
				handleVitalsByID(w, r, id)
				return
			}
			if len(parts) == 3 && parts[2] == "history" {
				handleVitalsHistory(w, r, id)
				return
			}
		case "cards":
			if len(parts) == 2 {
				handleCardByID(w, r, id)
				return
			}
		}
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handlePatientGet(w, r, id)
	case http.MethodPut:
		handlePatientUpdate(w, r, id)
	case http.MethodDelete:
		// Шаг 6.8: удаление пациента — только admin/superadmin.
		requireRole(RoleAdmin, RoleSuperadmin)(http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				handlePatientDelete(w, r, id)
			},
		)).ServeHTTP(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePatientGet возвращает одного пациента по id.
func handlePatientGet(w http.ResponseWriter, r *http.Request, id string) {
	p, err := getPatientByID(db, id)
	if err == sql.ErrNoRows {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// handlePatientUpdate — PUT /api/patients/{id}.
// Принимает полный объект пациента, перезаписывает запись.
func handlePatientUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var p Patient
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	// id из URL имеет приоритет, но если клиент прислал другой — это ошибка.
	if p.ID != "" && p.ID != id {
		http.Error(w, "id mismatch", http.StatusBadRequest)
		return
	}
	p.ID = id
	docID, ok := resolveDoctorID(r)
	if !ok {
		writeUnauthorized(w)
		return
	}
	p.DoctorID = docID

	if p.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if p.Gender != "" && p.Gender != "male" && p.Gender != "female" && p.Gender != "other" {
		http.Error(w, "gender must be male, female or other", http.StatusBadRequest)
		return
	}

	if err := updatePatient(db, &p); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

// handlePatientDelete — DELETE /api/patients/{id}.
// Шаг 6.8: admin-only. Гейтинг — requireRole(RoleAdmin, RoleSuperadmin)
// в handlePatientByID. Admin/superadmin удаляют любого пациента.
func handlePatientDelete(w http.ResponseWriter, r *http.Request, id string) {
	if err := deletePatientByID(db, id); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// === Vitals / Cards handlers (Шаг 5, B3) ===

// vitalsBody — тело PUT /api/patients/{id}/vitals.
type vitalsBody struct {
	BP   string  `json:"bp"`
	Temp float64 `json:"temp"`
	Spo2 int     `json:"spo2"`
}

// cardBody — тело PUT /api/patients/{id}/cards.
// Data — произвольный JSON, бэк его не парсит.
type cardBody struct {
	Data json.RawMessage `json:"data"`
}

// handleVitalsByID — GET / PUT /api/patients/{id}/vitals.
func handleVitalsByID(w http.ResponseWriter, r *http.Request, id string) {
	// Проверка существования пациента → 404 + FK-защита.
	if _, err := getPatientByID(db, id); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		bp, temp, spo2, updated, err := getVitals(db, id)
		if err == sql.ErrNoRows {
			http.Error(w, "vitals not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"bp":      bp,
			"temp":    temp,
			"spo2":    spo2,
			"updated": updated,
		})

	case http.MethodPut:
		var body vitalsBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := upsertVitals(db, id, body.BP, body.Temp, body.Spo2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := appendVitalsHistory(db, id, body.BP, body.Temp, body.Spo2); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleVitalsHistory — GET /api/patients/{id}/vitals/history?limit=N.
func handleVitalsHistory(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := getPatientByID(db, id); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	limit := 50
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}

	records, err := getVitalsHistory(db, id, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(records)
}

// handleCardByID — GET / PUT /api/patients/{id}/cards.
func handleCardByID(w http.ResponseWriter, r *http.Request, id string) {
	if _, err := getPatientByID(db, id); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		data, updated, err := getCard(db, id)
		if err == sql.ErrNoRows {
			http.Error(w, "card not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"data":    json.RawMessage(data),
			"updated": updated,
		})

	case http.MethodPut:
		var body cardBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		if len(body.Data) == 0 {
			http.Error(w, "data is required", http.StatusBadRequest)
			return
		}
		if err := upsertCard(db, id, string(body.Data)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

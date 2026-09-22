package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// Patient — структура пациента для API и SQLite.
// Соответствует полям схемы БД (без Symptoms и AIResult — добавим в шаг 4).
type Patient struct {
	ID         string  `json:"id"`
	DoctorID   string  `json:"doctorId"`
	ClinicID   string  `json:"clinicId,omitempty"`
	Name       string  `json:"name"`
	Age        int     `json:"age"`
	AgeMonths  int     `json:"ageMonths,omitempty"`
	AgeDisplay string  `json:"ageDisplay,omitempty"`
	BirthDate  string  `json:"birthDate,omitempty"`
	Gender     string  `json:"gender,omitempty"`
	Height     float64 `json:"height,omitempty"`
	Weight     float64 `json:"weight,omitempty"`
	Phone      string  `json:"phone,omitempty"`
	Email      string  `json:"email,omitempty"`
	Status     string  `json:"status,omitempty"`
	Diagnosis  string  `json:"diagnosis,omitempty"`
	AIChecked  bool    `json:"aiChecked,omitempty"`
	Created    string  `json:"created,omitempty"`
	LastSync   string  `json:"lastSync,omitempty"`
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

// handlePatientsList возвращает список пациентов для stubDoctorID.
func handlePatientsList(w http.ResponseWriter, r *http.Request) {
	patients, err := getPatientsByDoctor(db, stubDoctorID)
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
	p.DoctorID = stubDoctorID
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
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/patients/")
	if id == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
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

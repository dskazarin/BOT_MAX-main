package main

// encounters.go — подпатч 6.38 (B28.2): store-слой для encounters.
//
// encounters — центральная сущность МИС: каждый приём = отдельный эпизод
// с временем, врачом, специальностью, полями протокола и JSON-полями
// для общих/специфичных данных.
//
// Схема — в db.go (B28.1).

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Допустимые значения kind.
var validEncounterKinds = map[string]bool{
	"consultation": true,
	"study":        true,
	"procedure":    true,
	"lab":          true,
	"followup":     true,
}

// Допустимые значения status.
var validEncounterStatuses = map[string]bool{
	"draft":    true,
	"finished": true,
}

// Encounter — эпизод приёма.
type Encounter struct {
	ID              string `json:"id"`
	PatientID       string `json:"patient_id"`
	DoctorID        string `json:"doctor_id"`
	ClinicID        string `json:"clinic_id,omitempty"`
	Specialty       string `json:"specialty"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	StartedAt       string `json:"started_at"`
	FinishedAt      string `json:"finished_at,omitempty"`
	Status          string `json:"status"`
	Complaint       string `json:"complaint"`
	Anamnesis       string `json:"anamnesis"`
	Examination     string `json:"examination"`
	DiagnosisText   string `json:"diagnosis_text"`
	DiagnosisICD10  string `json:"diagnosis_icd10"`
	Recommendations string `json:"recommendations"`
	CommonJSON      string `json:"common_json"`
	SpecialtyJSON   string `json:"specialty_json"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

// CreateEncounterInput — вход для createEncounter.
type CreateEncounterInput struct {
	PatientID       string
	DoctorID        string
	ClinicID        string
	Specialty       string
	Title           string
	Kind            string
	StartedAt       string
	Complaint       string
	Anamnesis       string
	Examination     string
	DiagnosisText   string
	DiagnosisICD10  string
	Recommendations string
	CommonJSON      string
	SpecialtyJSON   string
}

// UpdateEncounterInput — вход для updateEncounter (partial).
type UpdateEncounterInput struct {
	Specialty       *string
	Title           *string
	Kind            *string
	Status          *string
	FinishedAt      *string
	Complaint       *string
	Anamnesis       *string
	Examination     *string
	DiagnosisText   *string
	DiagnosisICD10  *string
	Recommendations *string
	CommonJSON      *string
	SpecialtyJSON   *string
}

// validateEncounterKind — проверка допустимого kind.
func validateEncounterKind(kind string) error {
	if kind == "" {
		return nil
	}
	if !validEncounterKinds[kind] {
		return fmt.Errorf("invalid kind: %q", kind)
	}
	return nil
}

// validateEncounterStatus — проверка допустимого status.
func validateEncounterStatus(status string) error {
	if status == "" {
		return nil
	}
	if !validEncounterStatuses[status] {
		return fmt.Errorf("invalid status: %q", status)
	}
	return nil
}

// createEncounter вставляет новый encounter.
func createEncounter(db *sql.DB, in CreateEncounterInput) (*Encounter, error) {
	if in.PatientID == "" || in.DoctorID == "" {
		return nil, fmt.Errorf("patient_id and doctor_id required")
	}
	if in.StartedAt == "" {
		in.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if err := validateEncounterKind(in.Kind); err != nil {
		return nil, err
	}
	if in.Kind == "" {
		in.Kind = "consultation"
	}
	if in.CommonJSON == "" {
		in.CommonJSON = "{}"
	}
	if in.SpecialtyJSON == "" {
		in.SpecialtyJSON = "{}"
	}

	id, err := newID("ENC")
	if err != nil {
		return nil, fmt.Errorf("newID: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	_, err = db.Exec(
		`INSERT INTO encounters (
			id, patient_id, doctor_id, clinic_id, specialty, title, kind,
			started_at, finished_at, status,
			complaint, anamnesis, examination, diagnosis_text, diagnosis_icd10,
			recommendations, common_json, specialty_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULL, 'draft', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.PatientID, in.DoctorID, nullIfEmpty(in.ClinicID),
		in.Specialty, in.Title, in.Kind,
		in.StartedAt,
		in.Complaint, in.Anamnesis, in.Examination, in.DiagnosisText, in.DiagnosisICD10,
		in.Recommendations, in.CommonJSON, in.SpecialtyJSON, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert encounter: %w", err)
	}

	return getEncounterByID(db, id)
}

// getEncounterByID — читает один encounter.
func getEncounterByID(db *sql.DB, id string) (*Encounter, error) {
	if id == "" {
		return nil, fmt.Errorf("empty encounter id")
	}
	const q = `SELECT id, patient_id, doctor_id, COALESCE(clinic_id, ''),
		specialty, title, kind, started_at, COALESCE(finished_at, ''),
		status, complaint, anamnesis, examination, diagnosis_text, diagnosis_icd10,
		recommendations, common_json, specialty_json, created_at, updated_at
		FROM encounters WHERE id = ?`

	var e Encounter
	err := db.QueryRow(q, id).Scan(
		&e.ID, &e.PatientID, &e.DoctorID, &e.ClinicID,
		&e.Specialty, &e.Title, &e.Kind, &e.StartedAt, &e.FinishedAt,
		&e.Status, &e.Complaint, &e.Anamnesis, &e.Examination,
		&e.DiagnosisText, &e.DiagnosisICD10,
		&e.Recommendations, &e.CommonJSON, &e.SpecialtyJSON,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, fmt.Errorf("select encounter: %w", err)
	}
	return &e, nil
}

// listEncountersByPatient — список encounter'ов пациента.
func listEncountersByPatient(db *sql.DB, patientID string, limit, offset int) ([]*Encounter, error) {
	return listEncounters(db,
		`WHERE patient_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		patientID, limit, offset)
}

// listEncountersByDoctor — список encounter'ов врача.
func listEncountersByDoctor(db *sql.DB, doctorID string, limit, offset int) ([]*Encounter, error) {
	return listEncounters(db,
		`WHERE doctor_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`,
		doctorID, limit, offset)
}

// listEncounters — общий внутренний SELECT.
func listEncounters(db *sql.DB, where string, args ...any) ([]*Encounter, error) {
	if len(args) >= 3 {
		if lim, ok := args[len(args)-2].(int); ok && (lim <= 0 || lim > 1000) {
			args[len(args)-2] = 100
		}
		if off, ok := args[len(args)-1].(int); ok && off < 0 {
			args[len(args)-1] = 0
		}
	}
	q := `SELECT id, patient_id, doctor_id, COALESCE(clinic_id, ''),
		specialty, title, kind, started_at, COALESCE(finished_at, ''),
		status, complaint, anamnesis, examination, diagnosis_text, diagnosis_icd10,
		recommendations, common_json, specialty_json, created_at, updated_at
		FROM encounters ` + where

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("list encounters: %w", err)
	}
	defer rows.Close()

	out := []*Encounter{}
	for rows.Next() {
		var e Encounter
		if err := rows.Scan(
			&e.ID, &e.PatientID, &e.DoctorID, &e.ClinicID,
			&e.Specialty, &e.Title, &e.Kind, &e.StartedAt, &e.FinishedAt,
			&e.Status, &e.Complaint, &e.Anamnesis, &e.Examination,
			&e.DiagnosisText, &e.DiagnosisICD10,
			&e.Recommendations, &e.CommonJSON, &e.SpecialtyJSON,
			&e.CreatedAt, &e.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan encounter: %w", err)
		}
		out = append(out, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows.Err: %w", err)
	}
	return out, nil
}

// updateEncounter — частичное обновление.
func updateEncounter(db *sql.DB, id string, in UpdateEncounterInput) error {
	if id == "" {
		return fmt.Errorf("empty encounter id")
	}

	sets := []string{}
	args := []any{}

	addStr := func(col string, v *string) {
		if v != nil {
			sets = append(sets, col+" = ?")
			args = append(args, strings.TrimSpace(*v))
		}
	}
	addStr("specialty", in.Specialty)
	addStr("title", in.Title)
	addStr("complaint", in.Complaint)
	addStr("anamnesis", in.Anamnesis)
	addStr("examination", in.Examination)
	addStr("diagnosis_text", in.DiagnosisText)
	addStr("diagnosis_icd10", in.DiagnosisICD10)
	addStr("recommendations", in.Recommendations)
	addStr("common_json", in.CommonJSON)
	addStr("specialty_json", in.SpecialtyJSON)

	if in.Kind != nil {
		if err := validateEncounterKind(*in.Kind); err != nil {
			return err
		}
		sets = append(sets, "kind = ?")
		args = append(args, *in.Kind)
	}
	if in.Status != nil {
		if err := validateEncounterStatus(*in.Status); err != nil {
			return err
		}
		sets = append(sets, "status = ?")
		args = append(args, *in.Status)
	}
	if in.FinishedAt != nil {
		sets = append(sets, "finished_at = ?")
		args = append(args, nullIfEmpty(*in.FinishedAt))
	}

	if _, err := getEncounterByID(db, id); err != nil {
		return err
	}

	if len(sets) == 0 {
		return nil
	}

	sets = append(sets, "updated_at = ?")
	args = append(args, time.Now().UTC().Format(time.RFC3339))
	args = append(args, id)

	q := "UPDATE encounters SET " + strings.Join(sets, ", ") + " WHERE id = ?"
	if _, err := db.Exec(q, args...); err != nil {
		return fmt.Errorf("update encounter: %w", err)
	}
	return nil
}

// finishEncounter — завершает encounter.
func finishEncounter(db *sql.DB, id string) error {
	if id == "" {
		return fmt.Errorf("empty encounter id")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`UPDATE encounters SET status = 'finished', finished_at = ?, updated_at = ?
		 WHERE id = ?`, now, now, id,
	)
	if err != nil {
		return fmt.Errorf("finish encounter: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// deleteEncounter — hard delete.
func deleteEncounter(db *sql.DB, id string) error {
	if id == "" {
		return fmt.Errorf("empty encounter id")
	}
	res, err := db.Exec(`DELETE FROM encounters WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete encounter: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// countEncountersByPatient — количество приёмов пациента.
func countEncountersByPatient(db *sql.DB, patientID string) (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM encounters WHERE patient_id = ?`, patientID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count encounters: %w", err)
	}
	return n, nil
}

// encounterExistsForDoctorAndPatient — true, если у врача есть encounter с пациентом.
func encounterExistsForDoctorAndPatient(db *sql.DB, doctorID, patientID string) (bool, error) {
	var exists int
	err := db.QueryRow(
		`SELECT 1 FROM encounters WHERE doctor_id = ? AND patient_id = ? LIMIT 1`,
		doctorID, patientID,
	).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists check: %w", err)
	}
	return true, nil
}

// MarshalJSONValue — сериализует value в JSON-строку.
func MarshalJSONValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// UnmarshalJSONValue — десериализует JSON-строку в map.
func UnmarshalJSONValue(s string) (map[string]any, error) {
	if strings.TrimSpace(s) == "" {
		return map[string]any{}, nil
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, fmt.Errorf("unmarshal json: %w", err)
	}
	return m, nil
}

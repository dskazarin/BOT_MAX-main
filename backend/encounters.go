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
	"log/slog"
	"net/http"
	"strconv"
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
	PatientID       string `json:"patient_id"`
	DoctorID        string `json:"doctor_id"`
	ClinicID        string `json:"clinic_id"`
	Specialty       string `json:"specialty"`
	Title           string `json:"title"`
	Kind            string `json:"kind"`
	StartedAt       string `json:"started_at"`
	Complaint       string `json:"complaint"`
	Anamnesis       string `json:"anamnesis"`
	Examination     string `json:"examination"`
	DiagnosisText   string `json:"diagnosis_text"`
	DiagnosisICD10  string `json:"diagnosis_icd10"`
	Recommendations string `json:"recommendations"`
	CommonJSON      string `json:"common_json"`
	SpecialtyJSON   string `json:"specialty_json"`
}

// UpdateEncounterInput — вход для updateEncounter (partial).
type UpdateEncounterInput struct {
	Specialty       *string `json:"specialty"`
	Title           *string `json:"title"`
	Kind            *string `json:"kind"`
	Status          *string `json:"status"`
	FinishedAt      *string `json:"finished_at"`
	Complaint       *string `json:"complaint"`
	Anamnesis       *string `json:"anamnesis"`
	Examination     *string `json:"examination"`
	DiagnosisText   *string `json:"diagnosis_text"`
	DiagnosisICD10  *string `json:"diagnosis_icd10"`
	Recommendations *string `json:"recommendations"`
	CommonJSON      *string `json:"common_json"`
	SpecialtyJSON   *string `json:"specialty_json"`
}

// PrefillInput — вход для prefill.
type PrefillInput struct {
	PatientID string `json:"patient_id"`
}

// PrefillOutput — ответ prefill (клинические поля последнего приёма).
type PrefillOutput struct {
	Found           bool   `json:"found"`
	EncounterID     string `json:"encounter_id,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	Specialty       string `json:"specialty"`
	Complaint       string `json:"complaint"`
	Anamnesis       string `json:"anamnesis"`
	Examination     string `json:"examination"`
	DiagnosisText   string `json:"diagnosis_text"`
	DiagnosisICD10  string `json:"diagnosis_icd10"`
	Recommendations string `json:"recommendations"`
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

// ---------------------------------------------------------------
// B28.3 — HTTP-слой
// ---------------------------------------------------------------

// registerEncounterRoutes — регистрирует /api/encounters.
// Все роуты доступны doctor/admin/superadmin.
func registerEncounterRoutes() {
	guard := requireRole(RoleDoctor, RoleAdmin, RoleSuperadmin)
	http.Handle("/api/encounters", guard(http.HandlerFunc(handleEncounters)))
	// B28.5: prefill — регистрируем ДО /api/encounters/, чтобы не попал в {id}.
	http.Handle("/api/encounters/prefill", guard(http.HandlerFunc(handleEncounterPrefill)))
	http.Handle("/api/encounters/", guard(http.HandlerFunc(handleEncounterByID)))

	slog.Info("registerEncounterRoutes", "routes",
		"/api/encounters (GET, POST), /api/encounters/prefill (POST), /api/encounters/{id} (GET, PATCH, DELETE, /finish)")
}

// handleEncounters — /api/encounters (без id).
func handleEncounters(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleEncountersList(w, r)
	case http.MethodPost:
		handleEncounterCreate(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// getLatestEncounterForPrefill — последний приём пациента для prefill.
// Возвращает nil, ErrNoRows, если приёмов нет.
func getLatestEncounterForPrefill(db *sql.DB, patientID string) (*Encounter, error) {
	var enc Encounter
	err := db.QueryRow(
		`SELECT id, patient_id, doctor_id, COALESCE(clinic_id,''), specialty, title, kind,
		        started_at, COALESCE(finished_at,''), status, complaint, anamnesis,
		        examination, diagnosis_text, diagnosis_icd10, recommendations,
		        common_json, specialty_json, created_at, updated_at
		 FROM encounters
		 WHERE patient_id = ?
		 ORDER BY started_at DESC
		 LIMIT 1`,
		patientID,
	).Scan(
		&enc.ID, &enc.PatientID, &enc.DoctorID, &enc.ClinicID, &enc.Specialty, &enc.Title,
		&enc.Kind, &enc.StartedAt, &enc.FinishedAt, &enc.Status, &enc.Complaint, &enc.Anamnesis,
		&enc.Examination, &enc.DiagnosisText, &enc.DiagnosisICD10, &enc.Recommendations,
		&enc.CommonJSON, &enc.SpecialtyJSON, &enc.CreatedAt, &enc.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &enc, nil
}

// handleEncounterPrefill — POST /api/encounters/prefill.
// Body: {"patient_id": "PAT-..."}.
// Возвращает клинические поля последнего приёма пациента (для автозаполнения
// формы нового приёма). Если приёмов нет — found=false, все поля пустые.
func handleEncounterPrefill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req PrefillInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.PatientID == "" {
		http.Error(w, `{"error":"patient_id required"}`, http.StatusBadRequest)
		return
	}

	// Пациент должен существовать.
	if _, err := getPatientByID(db, req.PatientID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Последний приём пациента.
	enc, err := getLatestEncounterForPrefill(db, req.PatientID)
	if err != nil {
		slog.Warn("encounter.prefill", "err", err, "patient_id", req.PatientID)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if enc == nil {
		_ = json.NewEncoder(w).Encode(PrefillOutput{Found: false})
		return
	}

	out := PrefillOutput{
		Found:           true,
		EncounterID:     enc.ID,
		StartedAt:       enc.StartedAt,
		Specialty:       enc.Specialty,
		Complaint:       enc.Complaint,
		Anamnesis:       enc.Anamnesis,
		Examination:     enc.Examination,
		DiagnosisText:   enc.DiagnosisText,
		DiagnosisICD10:  enc.DiagnosisICD10,
		Recommendations: enc.Recommendations,
	}
	_ = json.NewEncoder(w).Encode(out)
}

// handleEncounterByID — /api/encounters/{id} и подпути.
func handleEncounterByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/encounters/")
	if path == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	parts := strings.Split(path, "/")
	id := parts[0]

	// /{id}/finish
	if len(parts) == 2 && parts[1] == "finish" {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		handleEncounterFinish(w, r, id)
		return
	}
	if len(parts) > 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleEncounterGet(w, r, id)
	case http.MethodPatch:
		handleEncounterUpdate(w, r, id)
	case http.MethodDelete:
		handleEncounterDelete(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// canReadEncounter — проверка: имеет ли право актор читать encounter.
//
//	doctor     — если encounter.doctor_id == he, ИЛИ у него есть encounter
//	             с тем же пациентом (см. §6.37);
//	admin      — если encounter.clinic_id == его clinic_id;
//	superadmin — всегда.
func canReadEncounter(r *http.Request, enc *Encounter) bool {
	role, ok := doctorRoleFromCtx(r)
	if !ok {
		return false
	}
	me := doctorIDFromCtx(r)

	switch role {
	case RoleSuperadmin:
		return true
	case RoleAdmin:
		// Сверяем clinic_id admin'а и encounter'а.
		adminDoc, err := getDoctorRowByID(db, me)
		if err != nil {
			return false
		}
		return enc.ClinicID != "" && enc.ClinicID == adminDoc.ClinicID
	case RoleDoctor:
		if enc.DoctorID == me {
			return true
		}
		// Врач видит историю пациентов, с которыми есть encounter.
		ok, err := encounterExistsForDoctorAndPatient(db, me, enc.PatientID)
		if err != nil {
			slog.Warn("canReadEncounter: exists check", "err", err)
			return false
		}
		return ok
	}
	return false
}

// canMutateEncounter — true, если актор может менять encounter.
//
//	doctor     — только свои;
//	admin      — НЕ может (медицинские данные);
//	superadmin — может.
func canMutateEncounter(r *http.Request, enc *Encounter) bool {
	role, _ := doctorRoleFromCtx(r)
	me := doctorIDFromCtx(r)
	switch role {
	case RoleSuperadmin:
		return true
	case RoleDoctor:
		return enc.DoctorID == me
	}
	return false
}

// handleEncounterCreate — POST /api/encounters.
func handleEncounterCreate(w http.ResponseWriter, r *http.Request) {
	var req CreateEncounterInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if req.PatientID == "" {
		http.Error(w, `{"error":"patient_id required"}`, http.StatusBadRequest)
		return
	}

	// DoctorID: врач — принудительно свой; admin/superadmin — могут задать,
	// по умолчанию берём из ctx.
	me := doctorIDFromCtx(r)
	role, _ := doctorRoleFromCtx(r)
	if role == RoleDoctor {
		req.DoctorID = me
	} else if req.DoctorID == "" {
		req.DoctorID = me
	}
	if req.DoctorID == "" {
		http.Error(w, `{"error":"doctor_id required"}`, http.StatusBadRequest)
		return
	}

	// Проверка, что patient существует.
	if _, err := getPatientByID(db, req.PatientID); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	enc, err := createEncounter(db, req)
	if err != nil {
		slog.Warn("encounter.create", "err", err)
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	auditLog(db, me, "encounter.created", "encounter", enc.ID, clientIP(r),
		map[string]string{"patient_id": enc.PatientID, "kind": enc.Kind})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(enc)
}

// handleEncountersList — GET /api/encounters.
// Обязательно: ?patient_id= или ?doctor_id= (иначе 400).
// Для admin/superadmin: можно ?clinic_id=.
func handleEncountersList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	patientID := strings.TrimSpace(q.Get("patient_id"))
	doctorID := strings.TrimSpace(q.Get("doctor_id"))
	clinicID := strings.TrimSpace(q.Get("clinic_id"))

	limit := 100
	offset := 0
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	role, _ := doctorRoleFromCtx(r)
	me := doctorIDFromCtx(r)

	// Права на list:
	//   doctor  — только свои (doctor_id=me) или по пациенту, с которым есть encounter;
	//   admin   — только своя клиника;
	//   superadmin — любые фильтры.
	var (
		list []*Encounter
		err  error
	)
	switch {
	case patientID != "":
		// B28.4.4: врач видит timeline пациента так же, как его карточку.
		// В рамках одной клиники — это норма. Consent-модель для сторонних
		// врачей (из других клиник) — в B30 (личный кабинет пациента).
		// См. также §6.38 checkpoint.
		if role == RoleAdmin {
			// Проверим, что patient в его клинике (через encounters).
			// Упрощённо: если clinicID не задан — запретим.
			if clinicID == "" {
				adminDoc, _ := getDoctorRowByID(db, me)
				if adminDoc != nil {
					clinicID = adminDoc.ClinicID
				}
			}
			list, err = listEncounters(db,
				`WHERE patient_id = ? AND clinic_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`,
				patientID, clinicID, limit, offset)
		} else {
			list, err = listEncountersByPatient(db, patientID, limit, offset)
		}
	case doctorID != "":
		// Врачу — только свой doctor_id.
		if role == RoleDoctor && doctorID != me {
			http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
			return
		}
		list, err = listEncountersByDoctor(db, doctorID, limit, offset)
	case clinicID != "" && (role == RoleAdmin || role == RoleSuperadmin):
		list, err = listEncounters(db,
			`WHERE clinic_id = ? ORDER BY started_at DESC LIMIT ? OFFSET ?`,
			clinicID, limit, offset)
	default:
		http.Error(w, `{"error":"patient_id or doctor_id or clinic_id required"}`,
			http.StatusBadRequest)
		return
	}
	if err != nil {
		slog.Warn("encounters.list", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// Считаем count для пагинации (упрощённо — len от limit).
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items":  list,
		"limit":  limit,
		"offset": offset,
		"count":  len(list),
	})
}

// handleEncounterGet — GET /api/encounters/{id}.
func handleEncounterGet(w http.ResponseWriter, r *http.Request, id string) {
	enc, err := getEncounterByID(db, id)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Warn("encounter.get", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !canReadEncounter(r, enc) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(enc)
}

// handleEncounterUpdate — PATCH /api/encounters/{id}.
func handleEncounterUpdate(w http.ResponseWriter, r *http.Request, id string) {
	enc, err := getEncounterByID(db, id)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !canMutateEncounter(r, enc) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	// Завершённый encounter менять нельзя (медицинская неизменяемость).
	if enc.Status == "finished" {
		http.Error(w, `{"error":"encounter is finished; use amendment (not implemented)"}`,
			http.StatusForbidden)
		return
	}

	var req UpdateEncounterInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	if err := updateEncounter(db, id, req); err != nil {
		slog.Warn("encounter.update", "err", err, "id", id)
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	auditLog(db, doctorIDFromCtx(r), "encounter.updated", "encounter", id, clientIP(r), nil)

	// Вернём обновлённый.
	updated, _ := getEncounterByID(db, id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

// handleEncounterFinish — POST /api/encounters/{id}/finish.
func handleEncounterFinish(w http.ResponseWriter, r *http.Request, id string) {
	enc, err := getEncounterByID(db, id)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !canMutateEncounter(r, enc) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if enc.Status == "finished" {
		http.Error(w, `{"error":"already finished"}`, http.StatusBadRequest)
		return
	}
	if err := finishEncounter(db, id); err != nil {
		slog.Warn("encounter.finish", "err", err, "id", id)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	auditLog(db, doctorIDFromCtx(r), "encounter.finished", "encounter", id, clientIP(r), nil)
	updated, _ := getEncounterByID(db, id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updated)
}

// handleEncounterDelete — DELETE /api/encounters/{id}.
// Разрешено только для status='draft' (медицинская неизменяемость).
func handleEncounterDelete(w http.ResponseWriter, r *http.Request, id string) {
	enc, err := getEncounterByID(db, id)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if !canMutateEncounter(r, enc) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if enc.Status != "draft" {
		http.Error(w, `{"error":"only draft encounters can be deleted"}`,
			http.StatusForbidden)
		return
	}
	if err := deleteEncounter(db, id); err != nil {
		slog.Warn("encounter.delete", "err", err, "id", id)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	auditLog(db, doctorIDFromCtx(r), "encounter.deleted", "encounter", id, clientIP(r),
		map[string]string{"patient_id": enc.PatientID})
	w.WriteHeader(http.StatusNoContent)
}

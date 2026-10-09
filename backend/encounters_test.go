package main

// encounters_test.go — подпатч 6.38 (B28.2): тесты store-слоя encounters.
//
// Переиспользует newTestDB(t), seedDoctorForRefresh(t, db, id) из refresh_test.go.

import (
	"database/sql"
	"errors"
	"testing"
	"time"
)

// seedPatientForEncounter — создаёт пациента с заданным doctor_id.
func seedPatientForEncounter(t *testing.T, db *sql.DB, patientID, doctorID string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO patients (id, doctor_id, name, gender, created)
		 VALUES (?, ?, 'Test Patient', 'male', datetime('now'))`,
		patientID, doctorID,
	)
	if err != nil {
		t.Fatalf("insert patient %s: %v", patientID, err)
	}
}

// TestEncounter_CreateAndGet — happy path.
func TestEncounter_CreateAndGet(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1",
		DoctorID:  "D1",
		Specialty: "lor",
		Title:     "Первичный осмотр ЛОР",
		Kind:      "consultation",
		Complaint: "заложенность носа",
	})
	if err != nil {
		t.Fatalf("createEncounter: %v", err)
	}
	if enc.ID == "" {
		t.Fatalf("ID пустой")
	}
	if enc.Status != "draft" {
		t.Errorf("Status: got %q, want draft", enc.Status)
	}
	if enc.Kind != "consultation" {
		t.Errorf("Kind: got %q", enc.Kind)
	}

	// Round-trip.
	got, err := getEncounterByID(db, enc.ID)
	if err != nil {
		t.Fatalf("getEncounterByID: %v", err)
	}
	if got.Complaint != "заложенность носа" {
		t.Errorf("Complaint: got %q", got.Complaint)
	}
}

// TestEncounter_KindDefault — пустой kind → 'consultation'.
func TestEncounter_KindDefault(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1",
	})
	if err != nil {
		t.Fatalf("createEncounter: %v", err)
	}
	if enc.Kind != "consultation" {
		t.Fatalf("Kind: got %q, want consultation", enc.Kind)
	}
}

// TestEncounter_InvalidKind — невалидный kind.
func TestEncounter_InvalidKind(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	_, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1", Kind: "bogus",
	})
	if err == nil {
		t.Fatalf("ожидали ошибку при невалидном kind")
	}
}

// TestEncounter_ListByPatient — сортировка по started_at DESC.
func TestEncounter_ListByPatient(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	// 3 приёма в разное время.
	times := []string{
		"2026-01-01T10:00:00Z",
		"2026-03-15T10:00:00Z",
		"2026-02-01T10:00:00Z",
	}
	for i, ts := range times {
		_, err := createEncounter(db, CreateEncounterInput{
			PatientID: "P1", DoctorID: "D1",
			Title:     "enc " + string(rune('A'+i)),
			StartedAt: ts,
		})
		if err != nil {
			t.Fatalf("createEncounter %d: %v", i, err)
		}
	}

	list, err := listEncountersByPatient(db, "P1", 10, 0)
	if err != nil {
		t.Fatalf("listEncountersByPatient: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("got %d, want 3", len(list))
	}
	// Первый — самый свежий (2026-03-15).
	if list[0].StartedAt != "2026-03-15T10:00:00Z" {
		t.Errorf("list[0].StartedAt = %q, want 2026-03-15", list[0].StartedAt)
	}
	if list[2].StartedAt != "2026-01-01T10:00:00Z" {
		t.Errorf("list[2].StartedAt = %q, want 2026-01-01", list[2].StartedAt)
	}
}

// TestEncounter_ListByDoctor.
func TestEncounter_ListByDoctor(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedDoctorForRefresh(t, db, "D2")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedPatientForEncounter(t, db, "P2", "D2")

	_, _ = createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})
	_, _ = createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})
	_, _ = createEncounter(db, CreateEncounterInput{PatientID: "P2", DoctorID: "D2"})

	list1, _ := listEncountersByDoctor(db, "D1", 10, 0)
	list2, _ := listEncountersByDoctor(db, "D2", 10, 0)
	if len(list1) != 2 {
		t.Errorf("D1: got %d, want 2", len(list1))
	}
	if len(list2) != 1 {
		t.Errorf("D2: got %d, want 1", len(list2))
	}
}

// TestEncounter_UpdatePartial — меняем только указанные поля.
func TestEncounter_UpdatePartial(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, _ := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1",
		Complaint: "изначально",
		Anamnesis: "не менять",
	})

	newComplaint := "обновлено"
	if err := updateEncounter(db, enc.ID, UpdateEncounterInput{
		Complaint: &newComplaint,
	}); err != nil {
		t.Fatalf("updateEncounter: %v", err)
	}

	got, _ := getEncounterByID(db, enc.ID)
	if got.Complaint != "обновлено" {
		t.Errorf("Complaint: got %q", got.Complaint)
	}
	if got.Anamnesis != "не менять" {
		t.Errorf("Anamnesis не должен меняться: got %q", got.Anamnesis)
	}
}

// TestEncounter_UpdateInvalidStatus — невалидный статус.
func TestEncounter_UpdateInvalidStatus(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, _ := createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})

	bogus := "bogus"
	if err := updateEncounter(db, enc.ID, UpdateEncounterInput{Status: &bogus}); err == nil {
		t.Fatalf("ожидали ошибку при невалидном status")
	}
}

// TestEncounter_Finish — status→finished + finished_at.
func TestEncounter_Finish(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, _ := createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})

	if err := finishEncounter(db, enc.ID); err != nil {
		t.Fatalf("finishEncounter: %v", err)
	}

	got, _ := getEncounterByID(db, enc.ID)
	if got.Status != "finished" {
		t.Errorf("Status: got %q, want finished", got.Status)
	}
	if got.FinishedAt == "" {
		t.Errorf("FinishedAt пустой")
	}
}

// TestEncounter_FinishNotFound — несуществующий ID.
func TestEncounter_FinishNotFound(t *testing.T) {
	db := newTestDB(t)
	err := finishEncounter(db, "NEVER")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ожидали sql.ErrNoRows, got %v", err)
	}
}

// TestEncounter_GetNotFound — несуществующий ID.
func TestEncounter_GetNotFound(t *testing.T) {
	db := newTestDB(t)
	_, err := getEncounterByID(db, "NEVER")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("ожидали sql.ErrNoRows, got %v", err)
	}
}

// TestEncounter_ExistsForDoctorAndPatient — проверка прав.
func TestEncounter_ExistsForDoctorAndPatient(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	// До создания encounter — false.
	ok, err := encounterExistsForDoctorAndPatient(db, "D1", "P1")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if ok {
		t.Fatalf("до encounter должно быть false")
	}

	_, _ = createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})

	ok, _ = encounterExistsForDoctorAndPatient(db, "D1", "P1")
	if !ok {
		t.Fatalf("после encounter должно быть true")
	}
}

// TestEncounter_Delete — hard delete.
func TestEncounter_Delete(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, _ := createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})
	if err := deleteEncounter(db, enc.ID); err != nil {
		t.Fatalf("deleteEncounter: %v", err)
	}
	if _, err := getEncounterByID(db, enc.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("после удаления ожидали ErrNoRows, got %v", err)
	}
}

// TestEncounter_CountByPatient.
func TestEncounter_CountByPatient(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	for i := 0; i < 5; i++ {
		_, _ = createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})
	}
	n, err := countEncountersByPatient(db, "P1")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 5 {
		t.Fatalf("got %d, want 5", n)
	}
}

// TestMarshalUnmarshalJSONValue — утилиты для common_json.
func TestMarshalUnmarshalJSONValue(t *testing.T) {
	src := map[string]any{"allergies": "пенициллин", "height": 178}
	js := MarshalJSONValue(src)
	if js == "" || js == "{}" {
		t.Fatalf("MarshalJSONValue вернул %q", js)
	}
	back, err := UnmarshalJSONValue(js)
	if err != nil {
		t.Fatalf("UnmarshalJSONValue: %v", err)
	}
	if back["allergies"] != "пенициллин" {
		t.Errorf("allergies: got %v", back["allergies"])
	}

	// Пустая строка → пустой map.
	m, err := UnmarshalJSONValue("")
	if err != nil || len(m) != 0 {
		t.Fatalf("пустая строка: got %v, %v", m, err)
	}
}

// TestUnmarshalJSONValue_Invalid — мусор.
func TestUnmarshalJSONValue_Invalid(t *testing.T) {
	_, err := UnmarshalJSONValue("not-json")
	if err == nil {
		t.Fatalf("ожидали ошибку при мусоре")
	}
}

// TestEncounter_UpdatedAtChanged — updated_at меняется.
func TestEncounter_UpdatedAtChanged(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	enc, _ := createEncounter(db, CreateEncounterInput{PatientID: "P1", DoctorID: "D1"})
	initial := enc.UpdatedAt

	time.Sleep(1100 * time.Millisecond) // чтобы время успело измениться (секундная точность)

	newComplaint := "обновлено"
	_ = updateEncounter(db, enc.ID, UpdateEncounterInput{Complaint: &newComplaint})

	got, _ := getEncounterByID(db, enc.ID)
	if got.UpdatedAt == initial {
		t.Errorf("UpdatedAt не изменился: %q", got.UpdatedAt)
	}
}

// ---------- B28.5.1b: тесты prefill ----------

// TestPrefill_GetLatest_Found — есть 2 приёма, возвращается последний.
func TestPrefill_GetLatest_Found(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	// Старый приём
	_, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1", Specialty: "lor",
		Title: "Первый", Kind: "consultation", Complaint: "старая жалоба",
	})
	if err != nil {
		t.Fatalf("create 1: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // гарантируем, что started_at отличается

	// Новый приём
	newEnc, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1", Specialty: "therapy",
		Title: "Второй", Kind: "followup", Complaint: "новая жалоба",
	})
	if err != nil {
		t.Fatalf("create 2: %v", err)
	}

	got, err := getLatestEncounterForPrefill(db, "P1")
	if err != nil {
		t.Fatalf("getLatestEncounterForPrefill: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil, want encounter")
	}
	if got.ID != newEnc.ID {
		t.Errorf("ID: got %q, want %q", got.ID, newEnc.ID)
	}
	if got.Specialty != "therapy" {
		t.Errorf("Specialty: got %q, want therapy", got.Specialty)
	}
	if got.Complaint != "новая жалоба" {
		t.Errorf("Complaint: got %q", got.Complaint)
	}
}

// TestPrefill_GetLatest_NoHistory — нет приёмов → nil.
func TestPrefill_GetLatest_NoHistory(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	got, err := getLatestEncounterForPrefill(db, "P1")
	if err != nil {
		t.Fatalf("getLatestEncounterForPrefill: %v", err)
	}
	if got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

// TestPrefill_GetLatest_AllClinicalFields — все клинические поля копируются.
func TestPrefill_GetLatest_AllClinicalFields(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")

	_, err := createEncounter(db, CreateEncounterInput{
		PatientID:       "P1",
		DoctorID:        "D1",
		Specialty:       "lor",
		Title:           "Осмотр",
		Kind:            "consultation",
		Complaint:       "жалоба-тест",
		Anamnesis:       "анамнез-тест",
		Examination:     "осмотр-тест",
		DiagnosisText:   "диагноз-тест",
		DiagnosisICD10:  "J06.9",
		Recommendations: "рекомендации-тест",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := getLatestEncounterForPrefill(db, "P1")
	if err != nil {
		t.Fatalf("getLatest: %v", err)
	}
	if got == nil {
		t.Fatalf("got nil")
	}

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"Specialty", got.Specialty, "lor"},
		{"Complaint", got.Complaint, "жалоба-тест"},
		{"Anamnesis", got.Anamnesis, "анамнез-тест"},
		{"Examination", got.Examination, "осмотр-тест"},
		{"DiagnosisText", got.DiagnosisText, "диагноз-тест"},
		{"DiagnosisICD10", got.DiagnosisICD10, "J06.9"},
		{"Recommendations", got.Recommendations, "рекомендации-тест"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestPrefill_DifferentPatients — изоляция: приёмы другого пациента не подтягиваются.
func TestPrefill_DifferentPatients(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedPatientForEncounter(t, db, "P2", "D1")

	// Приём только у P1
	_, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1", Specialty: "lor",
		Title: "P1 enc", Kind: "consultation", Complaint: "жалоба P1",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Для P2 — nil
	got, err := getLatestEncounterForPrefill(db, "P2")
	if err != nil {
		t.Fatalf("P2: %v", err)
	}
	if got != nil {
		t.Errorf("P2 got %+v, want nil", got)
	}

	// Для P1 — есть
	got1, err := getLatestEncounterForPrefill(db, "P1")
	if err != nil {
		t.Fatalf("P1: %v", err)
	}
	if got1 == nil {
		t.Fatalf("P1 got nil, want encounter")
	}
	if got1.Complaint != "жалоба P1" {
		t.Errorf("P1 Complaint: got %q", got1.Complaint)
	}
}

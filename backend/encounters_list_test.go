package main

// encounters_list_test.go — B29.1.4: repository-тесты list (фильтры, пагинация, keyset).
//
// Тестируем listEncountersFiltered/countEncounters как основной путь
// handleEncountersList. HTTP-слой (RBAC/400) — отдельный подпатч B29.1.6.

import (
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// seedEncounterList создаёт n приёмов у (patientID, doctorID) с детерминированным
// started_at (шаг 1 час от base), циклично варьируя kind/status/specialty.
//
//	i%5 → kind:     consultation, study, procedure, lab, followup
//	i%2 → status:   finished, draft
//	i%3 → specialty: cardio, neuro, ortho
func seedEncounterList(t *testing.T, db *sql.DB, patientID, doctorID string, n int) {
	t.Helper()
	kinds := []string{"consultation", "study", "procedure", "lab", "followup"}
	specs := []string{"cardio", "neuro", "ortho"}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339)
		kind := kinds[i%len(kinds)]
		spec := specs[i%len(specs)]
		in := CreateEncounterInput{
			PatientID: patientID,
			DoctorID:  doctorID,
			Kind:      kind,
			StartedAt: ts,
			Title:     fmt.Sprintf("enc-%02d", i),
			Specialty: spec,
		}
		enc, err := createEncounter(db, in)
		if err != nil {
			t.Fatalf("createEncounter %d: %v", i, err)
		}
		// i%2==0 → finished, иначе оставляем draft.
		if i%2 == 0 {
			if err := finishEncounter(db, enc.ID); err != nil {
				t.Fatalf("finishEncounter %d: %v", i, err)
			}
		}
	}
}

func mustList(t *testing.T, db *sql.DB, f *EncounterFilter) ([]*Encounter, string) {
	t.Helper()
	items, next, err := listEncountersFiltered(db, f)
	if err != nil {
		t.Fatalf("listEncountersFiltered: %v", err)
	}
	return items, next
}

// --- 1. Фильтр по kind ---
func TestListEncounters_FilterByKind(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10) // 5 kinds × по 2

	got, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Kind: "consultation", Limit: 50})
	if len(got) != 2 {
		t.Fatalf("consultation: got %d, want 2", len(got))
	}
	for _, e := range got {
		if e.Kind != "consultation" {
			t.Errorf("kind = %q, want consultation", e.Kind)
		}
	}
}

// --- 2. Фильтр по status ---
func TestListEncounters_FilterByStatus(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10) // 5 finished (i%2==0), 5 draft

	fin, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Status: "finished", Limit: 50})
	if len(fin) != 5 {
		t.Errorf("finished: got %d, want 5", len(fin))
	}
	draft, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Status: "draft", Limit: 50})
	if len(draft) != 5 {
		t.Errorf("draft: got %d, want 5", len(draft))
	}
}

// --- 3. Фильтр по specialty ---
func TestListEncounters_FilterBySpecialty(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 9) // 3 cardio, 3 neuro, 3 ortho

	got, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Specialty: "neuro", Limit: 50})
	if len(got) != 3 {
		t.Fatalf("neuro: got %d, want 3", len(got))
	}
	for _, e := range got {
		if e.Specialty != "neuro" {
			t.Errorf("specialty = %q, want neuro", e.Specialty)
		}
	}
}

// --- 4. Фильтр по from/to (границы включительно) ---
func TestListEncounters_FilterByFromTo(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10) // 00:00..09:00 UTC 2026-01-01

	from := time.Date(2026, 1, 1, 2, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 5, 0, 0, 0, time.UTC)
	got, _ := mustList(t, db, &EncounterFilter{
		PatientID: "P1", From: &from, To: &to, Limit: 50,
	})
	if len(got) != 4 {
		t.Fatalf("from..to: got %d, want 4", len(got))
	}
	// Проверим, что самая ранняя — 02:00, самая поздняя — 05:00 (включительно).
	if got[len(got)-1].StartedAt != "2026-01-01T02:00:00Z" {
		t.Errorf("min started_at = %q, want 02:00", got[len(got)-1].StartedAt)
	}
	if got[0].StartedAt != "2026-01-01T05:00:00Z" {
		t.Errorf("max started_at = %q, want 05:00", got[0].StartedAt)
	}
}

// --- 5. Комбинированный фильтр ---
func TestListEncounters_FilterCombined(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10)

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	got, _ := mustList(t, db, &EncounterFilter{
		PatientID: "P1", Kind: "consultation", Status: "finished",
		From: &from, To: &to, Limit: 50,
	})
	// consultation × finished × весь диапазон: i%5==0 && i%2==0 → i ∈ {0} → 1.
	if len(got) != 1 {
		t.Fatalf("combined: got %d, want 1", len(got))
	}
	if got[0].Kind != "consultation" || got[0].Status != "finished" {
		t.Errorf("combined mismatch: kind=%q status=%q", got[0].Kind, got[0].Status)
	}
}

// --- 6. Offset-пагинация ---
func TestListEncounters_OffsetPagination(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10)

	var all []string
	for off := 0; off < 10; off += 3 {
		items, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 3, Offset: off})
		for _, e := range items {
			all = append(all, e.ID)
		}
	}
	if len(all) != 10 {
		t.Fatalf("total collected = %d, want 10", len(all))
	}
	// Никаких дублей.
	seen := map[string]bool{}
	for _, id := range all {
		if seen[id] {
			t.Errorf("duplicate id %q", id)
		}
		seen[id] = true
	}
}

// --- 7. Keyset-пагинация: собираем все страницы ---
func TestListEncounters_KeysetPagination(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 10)

	// Полный набор (для сравнения).
	full, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 50})
	if len(full) != 10 {
		t.Fatalf("full = %d, want 10", len(full))
	}

	// Обходим keyset по 3.
	var got []string
	cursor := ""
	pages := 0
	for {
		f := &EncounterFilter{PatientID: "P1", Limit: 3, Cursor: cursor}
		items, next := mustList(t, db, f)
		for _, e := range items {
			got = append(got, e.ID)
		}
		pages++
		if next == "" {
			break
		}
		cursor = next
		if pages > 10 {
			t.Fatalf("too many pages — likely cursor loop")
		}
	}
	if len(got) != 10 {
		t.Fatalf("keyset collected = %d, want 10", len(got))
	}
	// Порядок должен совпадать с offset-версией.
	for i := range full {
		if full[i].ID != got[i] {
			t.Errorf("order mismatch at %d: offset=%q keyset=%q", i, full[i].ID, got[i])
		}
	}
}

// --- 8. На последней странице next == "" ---
func TestListEncounters_NextCursor_EmptyOnLastPage(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 4)

	// limit=4 при 4 элементах — всё на одной странице.
	items, next := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 4})
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	if next != "" {
		t.Errorf("next = %q, want empty", next)
	}
}

// --- 9. countEncounters согласован с len при большом limit ---
func TestListEncounters_CountEncounters(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 7)

	n, err := countEncounters(db, &EncounterFilter{PatientID: "P1"})
	if err != nil {
		t.Fatalf("countEncounters: %v", err)
	}
	if n != 7 {
		t.Errorf("count = %d, want 7", n)
	}

	items, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 50})
	if len(items) != n {
		t.Errorf("len(items)=%d, count=%d", len(items), n)
	}
}

// --- 10. Keyset стабилен при вставке новой строки между страницами ---
func TestListEncounters_KeysetStableUnderInsert(t *testing.T) {
	db := newTestDB(t)
	seedDoctorForRefresh(t, db, "D1")
	seedPatientForEncounter(t, db, "P1", "D1")
	seedEncounterList(t, db, "P1", "D1", 5)

	// Первая страница по 2.
	page1, next := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 2})
	if len(page1) != 2 || next == "" {
		t.Fatalf("page1: len=%d next=%q", len(page1), next)
	}

	// Вставляем НОВЫЙ encounter с "будущим" started_at — он придёт в начало,
	// но keyset должен продолжить без дублей и без пропусков уже отданных.
	_, err := createEncounter(db, CreateEncounterInput{
		PatientID: "P1", DoctorID: "D1",
		StartedAt: "2027-01-01T00:00:00Z", Title: "later",
	})
	if err != nil {
		t.Fatalf("insert later: %v", err)
	}

	// Вторая страница по курсору.
	page2, _ := mustList(t, db, &EncounterFilter{PatientID: "P1", Limit: 2, Cursor: next})
	for _, e := range page2 {
		if e.StartedAt == "2027-01-01T00:00:00Z" {
			t.Errorf("keyset должен не отдавать новый элемент (он выше курсора): %+v", e)
		}
	}
	// Проверяем отсутствие пересечения с page1.
	ids1 := map[string]bool{}
	for _, e := range page1 {
		ids1[e.ID] = true
	}
	for _, e := range page2 {
		if ids1[e.ID] {
			t.Errorf("page2 содержит id из page1: %q", e.ID)
		}
	}
}

package main

// pdf_export_test.go — B30.16 шаг 1: базовая проверка рендера.

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRenderPatientCardPDF_Header(t *testing.T) {
	p := Patient{ID: "PAT-1", Name: "Иванов Иван", BirthDate: "1980-01-02", AgeDisplay: "45 лет", Gender: "м", Diagnosis: "ОРВИ"}
	b, err := renderPatientCardPDF(p, `{"vitals":{"bp":"120/80"}}`, "2026-11-12T10:00:00Z", nil, nil)
	if err != nil {
		t.Fatalf("renderPatientCardPDF: %v", err)
	}
	if len(b) < 200 {
		t.Fatalf("PDF слишком мал: %d байт", len(b))
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatalf("нет сигнатуры %%PDF- в начале")
	}
}

func TestRenderPatientCardPDF_EmptyCard(t *testing.T) {
	b, err := renderPatientCardPDF(Patient{ID: "PAT-2"}, "", "", nil, nil)
	if err != nil {
		t.Fatalf("renderPatientCardPDF: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("нет сигнатуры %PDF-")
	}
}

func TestRenderPatientCardPDF_InvalidJSON(t *testing.T) {
	if _, err := renderPatientCardPDF(Patient{ID: "P"}, "not-json{{", "", nil, nil); err != nil {
		t.Fatalf("невалидный JSON не должен ронять рендер: %v", err)
	}
}

// --- B30.16 step2: GET /api/patients/{id}/cards.pdf ---

func reqCardPDF(doctorID, pid string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodGet, "/api/patients/"+pid+"/cards.pdf", nil).WithContext(ctx)
}

func TestPatientCardPDF_401_NoDoctor(t *testing.T) {
	setupConsentDB(t)
	seedPatient(t, db, "PAT-1", "Test Patient")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/patients/PAT-1/cards.pdf", nil)
	handlePatientCardPDF(rr, req, "PAT-1")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", rr.Code)
	}
}

func TestPatientCardPDF_404_NoPatient(t *testing.T) {
	setupConsentDB(t)

	rr := httptest.NewRecorder()
	handlePatientCardPDF(rr, reqCardPDF("DOC-1", "NOPE"), "NOPE")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("code = %d, want 404", rr.Code)
	}
}

func TestPatientCardPDF_403_NoConsent(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")

	rr := httptest.NewRecorder()
	handlePatientCardPDF(rr, reqCardPDF("DOC-1", "PAT-1"), "PAT-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("code = %d, want 403", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "consent_required") {
		t.Errorf("body = %s, want consent_required", rr.Body.String())
	}
}

func TestPatientCardPDF_200_OK(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")
	approveConsent(t, "PAT-1", "DOC-1", "card_read")
	if err := upsertCard(db, "PAT-1", `{"note":"hello"}`); err != nil {
		t.Fatalf("upsertCard: %v", err)
	}

	rr := httptest.NewRecorder()
	handlePatientCardPDF(rr, reqCardPDF("DOC-1", "PAT-1"), "PAT-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, `patient_PAT-1_card.pdf`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	body := rr.Body.String()
	if len(body) < 5 || body[:5] != "%PDF-" {
		t.Errorf("body does not start with %%PDF-, got %q", body)
	}
}

func TestPatientCardPDF_405_Method(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")

	ctx := withDoctorID(context.Background(), "DOC-1", RoleDoctor)
	req := httptest.NewRequest(http.MethodPost, "/api/patients/PAT-1/cards.pdf", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handlePatientCardPDF(rr, req, "PAT-1")
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code = %d, want 405", rr.Code)
	}
}

// --- B30.16 step4: разделы symptoms/allergies/vaccinations/files ---

// --- B30.16 step3/4: честные тесты через *Lines (UTF-8, без PDF-байтов) ---

func TestCardLines_Sections(t *testing.T) {
	card := `{"complaints":"болит голова","anamnesis":"2 дня","objective":"норма",` +
		`"diagnosis_clinical":"мигрень","mkb":"G43","recommendations":"покой","allergy":"пенициллин"}`
	joined := strings.Join(cardLines(card), "\n")
	for _, want := range []string{"болит голова", "2 дня", "норма", "мигрень", "G43", "покой", "пенициллин"} {
		if !strings.Contains(joined, want) {
			t.Errorf("cardLines не содержит %q", want)
		}
	}
}

func TestCardLines_UnknownKeys_ToOther(t *testing.T) {
	joined := strings.Join(cardLines(`{"vitals":{"bp":"120/80"},"custom":"x"}`), "\n")
	if !strings.Contains(joined, "Прочее") {
		t.Errorf("нет раздела Прочее")
	}
	if !strings.Contains(joined, "120/80") && !strings.Contains(joined, "custom") {
		t.Errorf("неизвестные ключи потерялись")
	}
}

func TestCardLines_InvalidJSON_Fallback(t *testing.T) {
	joined := strings.Join(cardLines(`{not json`), "\n")
	if !strings.Contains(joined, "Данные карты") {
		t.Errorf("невалидный JSON не ушёл в fallback")
	}
}

func TestVitalsLines(t *testing.T) {
	joined := strings.Join(vitalsLines(VitalsData{BP: "120/80", Temp: 36.7, SpO2: 98, Updated: "2026-11-12T10:00:00Z"}), "\n")
	for _, want := range []string{"120/80", "36.7", "98"} {
		if !strings.Contains(joined, want) {
			t.Errorf("vitalsLines не содержит %q", want)
		}
	}
}

func TestRecordsLines_AllKinds(t *testing.T) {
	cases := []struct {
		kind string
		rows []map[string]any
		want []string
		bad  []string
	}{
		{"symptoms", []map[string]any{{"name": "кашель", "severity": "средне", "onset": "2026-11-10", "notes": "сухой"}},
			[]string{"Симптомы", "кашель", "средне", "2026-11-10", "сухой"}, nil},
		{"allergies", []map[string]any{{"allergen": "пенициллин", "reaction": "сыпь", "severity": "высокая"}},
			[]string{"Аллергии", "пенициллин", "сыпь", "высокая"}, nil},
		{"vaccinations", []map[string]any{{"vaccine": "грипп", "dose": "0.5 мл", "administered": "2026-10-01"}},
			[]string{"Вакцинации", "грипп", "0.5 мл", "2026-10-01"}, nil},
		{"files", []map[string]any{{"filename": "analysis.pdf", "mime": "application/pdf", "size_bytes": int64(1234), "notes": "кровь", "data": "BLOB", "storage_key": "k"}},
			[]string{"Файлы", "analysis.pdf", "application/pdf", "1234 байт", "кровь"},
			[]string{"BLOB", "storage_key"}},
	}
	for _, c := range cases {
		joined := strings.Join(recordsLines(c.kind, c.rows), "\n")
		for _, w := range c.want {
			if !strings.Contains(joined, w) {
				t.Errorf("%s: нет %q", c.kind, w)
			}
		}
		for _, b := range c.bad {
			if strings.Contains(joined, b) {
				t.Errorf("%s: лишнее %q", c.kind, b)
			}
		}
	}
}

func TestRecordsLines_Empty(t *testing.T) {
	if got := recordsLines("symptoms", nil); len(got) != 0 {
		t.Errorf("nil rows → %v", got)
	}
	if got := recordsLines("symptoms", []map[string]any{}); len(got) != 0 {
		t.Errorf("empty rows → %v", got)
	}
}

// Smoke: PDF собирается и растёт от records (без проверки текста в сжатом потоке).
func TestRenderPDF_Smoke(t *testing.T) {
	p := Patient{ID: "PAT-1", Name: "Test"}
	base, err := renderPatientCardPDF(p, `{}`, "", nil, nil)
	if err != nil {
		t.Fatalf("base: %v", err)
	}
	withRecs, err := renderPatientCardPDF(p, `{}`, "", &VitalsData{BP: "120/80", Temp: 36.7, SpO2: 98},
		map[string][]map[string]any{"symptoms": {{"name": "кашель"}}})
	if err != nil {
		t.Fatalf("withRecs: %v", err)
	}
	if !bytes.HasPrefix(base, []byte("%PDF-")) {
		t.Errorf("нет %%PDF-")
	}
	if len(withRecs) <= len(base) {
		t.Errorf("records не увеличили PDF: %d vs %d", len(withRecs), len(base))
	}
}

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
	b, err := renderPatientCardPDF(p, `{"vitals":{"bp":"120/80"}}`, "2026-11-12T10:00:00Z", nil)
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
	b, err := renderPatientCardPDF(Patient{ID: "PAT-2"}, "", "", nil)
	if err != nil {
		t.Fatalf("renderPatientCardPDF: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("нет сигнатуры %PDF-")
	}
}

func TestRenderPatientCardPDF_InvalidJSON(t *testing.T) {
	if _, err := renderPatientCardPDF(Patient{ID: "P"}, "not-json{{", "", nil); err != nil {
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

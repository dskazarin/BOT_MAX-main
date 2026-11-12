package main

// pdf_export_test.go — B30.16 шаг 1: базовая проверка рендера.

import (
	"bytes"
	"testing"
)

func TestRenderPatientCardPDF_Header(t *testing.T) {
	p := Patient{ID: "PAT-1", Name: "Иванов Иван", BirthDate: "1980-01-02", AgeDisplay: "45 лет", Gender: "м", Diagnosis: "ОРВИ"}
	b, err := renderPatientCardPDF(p, `{"vitals":{"bp":"120/80"}}`, "2026-11-12T10:00:00Z")
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
	b, err := renderPatientCardPDF(Patient{ID: "PAT-2"}, "", "")
	if err != nil {
		t.Fatalf("renderPatientCardPDF: %v", err)
	}
	if !bytes.HasPrefix(b, []byte("%PDF-")) {
		t.Fatal("нет сигнатуры %PDF-")
	}
}

func TestRenderPatientCardPDF_InvalidJSON(t *testing.T) {
	if _, err := renderPatientCardPDF(Patient{ID: "P"}, "not-json{{", ""); err != nil {
		t.Fatalf("невалидный JSON не должен ронять рендер: %v", err)
	}
}

package main

// pdf_export.go — B30.16 шаг 1: рендер карты пациента в PDF.
//
// На этом шаге — шапка пациента + метаданные + тело карты как pretty-JSON.
// Разбор разделов (vitals/diagnoses/allergies/...) — шаги 3–4; endpoint — шаг 2.
// gofpdf выбран как чистый Go без CGO (docker/infra в окружении недоступны).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf"
)

// renderPatientCardPDF собирает PDF карты пациента.
// cardJSON — произвольный JSON из cards.data (может быть пустым).
func renderPatientCardPDF(p Patient, cardJSON, updated string) ([]byte, error) {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 15)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 16)
	pdf.CellFormat(0, 10, "Карта пациента", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 11)
	line := func(label, value string) {
		if value == "" {
			value = "—"
		}
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(40, 7, label+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 7, value, "", "L", false)
	}
	line("ID", p.ID)
	line("ФИО", p.Name)
	line("Дата рождения", p.BirthDate)
	line("Возраст", p.AgeDisplay)
	line("Пол", p.Gender)
	line("Телефон", p.Phone)
	line("Email", p.Email)
	line("Диагноз", p.Diagnosis)
	line("Карта обновлена", updated)

	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Данные карты", "", 1, "L", false, 0, "")

	body := cardJSON
	if body == "" {
		body = "{}"
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, []byte(body), "", "  "); err != nil {
		pretty.Reset()
		pretty.WriteString(body)
	}
	pdf.SetFont("Courier", "", 8)
	pdf.MultiCell(0, 4, pretty.String(), "", "L", false)

	pdf.SetFont("Arial", "I", 8)
	pdf.SetY(-15)
	pdf.CellFormat(0, 5, fmt.Sprintf("BOT_MAX · сгенерировано %s", time.Now().Format(time.RFC3339)), "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("renderPatientCardPDF: %w", err)
	}
	return buf.Bytes(), nil
}

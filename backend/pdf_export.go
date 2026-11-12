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
func renderPatientCardPDF(p Patient, cardJSON, updated string, vitals *VitalsData) ([]byte, error) {
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
	renderCardSections(pdf, cardJSON)
	if vitals != nil {
		renderVitalsSection(pdf, *vitals)
	}

	pdf.SetFont("Arial", "I", 8)
	pdf.SetY(-15)
	pdf.CellFormat(0, 5, fmt.Sprintf("BOT_MAX · сгенерировано %s", time.Now().Format(time.RFC3339)), "", 0, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("renderPatientCardPDF: %w", err)
	}
	return buf.Bytes(), nil
}

// VitalsData — снимок последних vitals для PDF (B30.16 step3).
type VitalsData struct {
	BP      string
	Temp    float64
	SpO2    int
	Updated string
}

// renderCardSections выводит разделы карты из плоского cardJSON
// ({field_xxx: "value"}). Невалидный JSON / неизвестные ключи → «Прочее».
func renderCardSections(pdf *gofpdf.Fpdf, cardJSON string) {
	fields := map[string]string{}
	if cardJSON != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(cardJSON), &m); err == nil {
			for k, v := range m {
				if sv, ok := v.(string); ok {
					fields[k] = sv
				} else if v != nil {
					if b, err := json.Marshal(v); err == nil {
						fields[k] = string(b)
					}
				}
			}
		} else {
			renderSection(pdf, "Прочее", map[string]string{"raw": cardJSON})
			return
		}
	}

	section := func(title string, keys ...string) {
		m := map[string]string{}
		for _, k := range keys {
			if v, ok := fields[k]; ok && v != "" {
				m[k] = v
				delete(fields, k)
			}
		}
		if len(m) > 0 {
			renderSection(pdf, title, m)
		}
	}

	section("Жалобы и анамнез", "complaints", "anamnesis")
	section("Объективно", "objective")
	section("Диагноз", "diagnosis_clinical", "mkb")
	section("Рекомендации", "recommendations")
	section("Аллергии (из карты)", "allergy")

	if len(fields) > 0 {
		renderSection(pdf, "Прочее", fields)
	}
}

// renderVitalsSection выводит раздел витальных показателей.
func renderVitalsSection(pdf *gofpdf.Fpdf, v VitalsData) {
	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, "Витальные показатели", "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	line := func(label, value string) {
		if value == "" {
			value = "—"
		}
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(40, 7, label+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 7, value, "", "L", false)
	}
	line("АД", v.BP)
	line("Температура", fmt.Sprintf("%.1f", v.Temp))
	line("SpO2", fmt.Sprintf("%d", v.SpO2))
	line("Обновлено", v.Updated)
}

// renderSection выводит раздел «заголовок + пары ключ/значение».
func renderSection(pdf *gofpdf.Fpdf, title string, m map[string]string) {
	if len(m) == 0 {
		return
	}
	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, title, "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	for k, v := range m {
		if v == "" {
			continue
		}
		pdf.SetFont("Arial", "B", 10)
		pdf.CellFormat(40, 7, cardLabel(k)+":", "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.MultiCell(0, 7, v, "", "L", false)
	}
}

// cardLabel — человекочитаемые подписи для ключей плоской карты.
func cardLabel(k string) string {
	switch k {
	case "complaints":
		return "Жалобы"
	case "anamnesis":
		return "Анамнез"
	case "objective":
		return "Объективно"
	case "diagnosis_clinical":
		return "Клинический диагноз"
	case "mkb":
		return "МКБ"
	case "recommendations":
		return "Рекомендации"
	case "allergy":
		return "Аллергии"
	case "raw":
		return "Данные карты"
	}
	return k
}

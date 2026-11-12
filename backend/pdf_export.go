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
func renderPatientCardPDF(p Patient, cardJSON, updated string, vitals *VitalsData, records map[string][]map[string]any) ([]byte, error) {
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
	renderRecordsSections(pdf, records)

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
	renderLines(pdf, cardLines(cardJSON))
}

// renderVitalsSection выводит раздел витальных показателей.
func renderVitalsSection(pdf *gofpdf.Fpdf, v VitalsData) {
	renderLines(pdf, vitalsLines(v))
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

// renderRecordsSections выводит разделы серверных записей
// (symptoms/allergies/vaccinations/files). Порядок фиксирован.
// files — только имена/мета, без blob и URL.
func renderRecordsSections(pdf *gofpdf.Fpdf, records map[string][]map[string]any) {
	if len(records) == 0 {
		return
	}
	for _, kind := range []string{"symptoms", "allergies", "vaccinations", "files"} {
		rows := records[kind]
		if len(rows) == 0 {
			continue
		}
		renderRecordsSection(pdf, recordKindTitle(kind), kind, rows)
	}
}

// renderRecordsSection — один раздел: заголовок + по строке на запись.
// Для files дополнительно скрываем data/has_data/storage_key/url (blob не выгружаем).
func renderRecordsSection(pdf *gofpdf.Fpdf, title, kind string, rows []map[string]any) {
	lines := recordsLines(kind, rows)
	if len(lines) > 0 {
		renderLines(pdf, lines)
	}
	_ = title
}

// recordKindTitle — заголовки разделов.
func recordKindTitle(kind string) string {
	switch kind {
	case "symptoms":
		return "Симптомы"
	case "allergies":
		return "Аллергии"
	case "vaccinations":
		return "Вакцинации"
	case "files":
		return "Файлы"
	}
	return kind
}

// recordPrimaryField — ключевое поле для строки записи.
func recordPrimaryField(kind string) string {
	switch kind {
	case "symptoms":
		return "name"
	case "allergies":
		return "allergen"
	case "vaccinations":
		return "vaccine"
	}
	return ""
}

// recordFieldOrder — порядок полей для детализации.
func recordFieldOrder(kind string) []string {
	switch kind {
	case "symptoms":
		return []string{"name", "severity", "onset", "notes"}
	case "allergies":
		return []string{"allergen", "reaction", "severity", "notes"}
	case "vaccinations":
		return []string{"vaccine", "dose", "administered", "notes"}
	}
	return nil
}

// recordPrimary — значение ключевого поля + его метка.
func recordPrimary(kind string, row map[string]any) (string, string) {
	f := recordPrimaryField(kind)
	if f == "" {
		return "", ""
	}
	return stringOf(row[f]), recordLabel(f)
}

// recordLabel — человекочитаемые подписи полей записей.
func recordLabel(f string) string {
	switch f {
	case "name":
		return "Название"
	case "severity":
		return "Тяжесть"
	case "onset":
		return "Начало"
	case "allergen":
		return "Аллерген"
	case "reaction":
		return "Реакция"
	case "vaccine":
		return "Вакцина"
	case "dose":
		return "Доза"
	case "administered":
		return "Дата введения"
	case "notes":
		return "Примечания"
	}
	return f
}

func stringOf(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprintf("%v", x)
	}
}

func intOf(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// cardLines — строки разделов карты из плоского cardJSON.
// Чистая функция: тестируется без gofpdf.
func cardLines(cardJSON string) []string {
	if cardJSON == "" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(cardJSON), &m); err != nil {
		return []string{"Прочее", "Данные карты: " + cardJSON}
	}
	fields := map[string]string{}
	for k, v := range m {
		if sv, ok := v.(string); ok {
			fields[k] = sv
			continue
		}
		if v != nil {
			if b, err := json.Marshal(v); err == nil {
				fields[k] = string(b)
			}
		}
	}
	var out []string
	add := func(title string, keys ...string) {
		var block []string
		for _, k := range keys {
			if v, ok := fields[k]; ok && v != "" {
				block = append(block, cardLabel(k)+": "+v)
				delete(fields, k)
			}
		}
		if len(block) > 0 {
			out = append(out, title)
			out = append(out, block...)
		}
	}
	add("Жалобы и анамнез", "complaints", "anamnesis")
	add("Объективно", "objective")
	add("Диагноз", "diagnosis_clinical", "mkb")
	add("Рекомендации", "recommendations")
	add("Аллергии (из карты)", "allergy")
	if len(fields) > 0 {
		out = append(out, "Прочее")
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			out = append(out, cardLabel(k)+": "+fields[k])
		}
	}
	return out
}

// vitalsLines — строки раздела vitals. Чистая функция.
func vitalsLines(v VitalsData) []string {
	return []string{
		"Витальные показатели",
		"АД: " + orDash(v.BP),
		fmt.Sprintf("Температура: %.1f", v.Temp),
		fmt.Sprintf("SpO2: %d", v.SpO2),
		"Обновлено: " + orDash(v.Updated),
	}
}

// recordsLines — строки раздела записей kind. Чистая функция.
// files: filename + (mime, size) + notes; остальные: ключевое поле + детали.
func recordsLines(kind string, rows []map[string]any) []string {
	if len(rows) == 0 {
		return nil
	}
	out := []string{recordKindTitle(kind)}
	for _, row := range rows {
		if kind == "files" {
			fn := stringOf(row["filename"])
			if fn == "" {
				fn = "(без имени)"
			}
			meta := []string{}
			if v := stringOf(row["mime"]); v != "" {
				meta = append(meta, v)
			}
			if v := intOf(row["size_bytes"]); v > 0 {
				meta = append(meta, fmt.Sprintf("%d байт", v))
			}
			suffix := ""
			if len(meta) > 0 {
				suffix = " (" + joinComma(meta) + ")"
			}
			out = append(out, "• "+fn+suffix)
			if n := stringOf(row["notes"]); n != "" {
				out = append(out, "  "+n)
			}
			continue
		}
		key, keyLabel := recordPrimary(kind, row)
		head := "• "
		if keyLabel != "" && key != "" {
			head += keyLabel + ": " + key
		} else if key != "" {
			head += key
		} else {
			head += "(без названия)"
		}
		out = append(out, head)
		for _, f := range recordFieldOrder(kind) {
			if f == recordPrimaryField(kind) {
				continue
			}
			if v := stringOf(row[f]); v != "" {
				out = append(out, "  "+recordLabel(f)+": "+v)
			}
		}
	}
	return out
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

// sortStrings — простой sort.Strings без импорта (чтобы не плодить зависимости).
func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j-1] > a[j]; j-- {
			a[j-1], a[j] = a[j], a[j-1]
		}
	}
}

// renderLines пишет строки в PDF: первая — заголовок (жирный, 12), остальные — 10.
func renderLines(pdf *gofpdf.Fpdf, lines []string) {
	if len(lines) == 0 {
		return
	}
	pdf.Ln(4)
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(0, 8, lines[0], "", 1, "L", false, 0, "")
	pdf.SetFont("Arial", "", 10)
	for _, l := range lines[1:] {
		pdf.MultiCell(0, 6, l, "", "L", false)
	}
}

package main

// patient_profile.go — B30.7: серверный профиль пациента.
//
// Модель:
//   Пациент читает/редактирует свой профиль сам. Резолв — как в vitals/consents:
//     patientIDFromCtx(r)   → patient_accounts.id (patientAccountID)
//     resolvePatientIDForSelf → acc.PatientID (patients.id)
//
// Доступные поля (только «свои», не врачебные):
//   name, birth_date, gender, height, weight
// Плюс серверный пересчёт age / age_months / age_display из birth_date,
// чтобы врач в карточке не видел расхождения.
//
// Middleware: patientAuthMiddleware в registerPatientProfileRoutes.
//
// Врачебные поля (doctor_id, status, diagnosis, ai_*, last_sync, clinic_id,
// phone, email) через этот эндпоинт НЕ меняются — только через
// /api/patients/{id} врачом.

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// PatientProfile — JSON-контракт GET/PUT /api/patient/profile.
type PatientProfile struct {
	Name       string  `json:"name"`
	BirthDate  string  `json:"birthDate,omitempty"`
	Gender     string  `json:"gender,omitempty"`
	Height     float64 `json:"height,omitempty"`
	Weight     float64 `json:"weight,omitempty"`
	Age        int     `json:"age,omitempty"`
	AgeMonths  int     `json:"ageMonths,omitempty"`
	AgeDisplay string  `json:"ageDisplay,omitempty"`
}

// profileBody — тело PUT /api/patient/profile.
// Указатели, чтобы отличить «поле не прислали» от «прислали пусто/0».
type profileBody struct {
	Name      *string  `json:"name"`
	BirthDate *string  `json:"birthDate"`
	Gender    *string  `json:"gender"`
	Height    *float64 `json:"height"`
	Weight    *float64 `json:"weight"`
}

const (
	maxHeightCm = 250.0
	maxWeightKg = 500.0
	dateLayout  = "2006-01-02"
)

// getPatientProfile — SELECT «своих» полей пациента по patients.id.
func getPatientProfile(db *sql.DB, patientID string) (*PatientProfile, error) {
	var p PatientProfile
	err := db.QueryRow(`
		SELECT name,
		       COALESCE(birth_date, ''), COALESCE(gender, ''),
		       COALESCE(height, 0), COALESCE(weight, 0),
		       COALESCE(age, 0), COALESCE(age_months, 0), COALESCE(age_display, '')
		FROM patients WHERE id = ?`, patientID).
		Scan(&p.Name, &p.BirthDate, &p.Gender, &p.Height, &p.Weight,
			&p.Age, &p.AgeMonths, &p.AgeDisplay)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// updatePatientProfile — UPDATE только «своих» полей + пересчитанный возраст.
func updatePatientProfile(db *sql.DB, patientID string, p *PatientProfile) error {
	res, err := db.Exec(`
		UPDATE patients SET
			name = ?, birth_date = ?, gender = ?, height = ?, weight = ?,
			age = ?, age_months = ?, age_display = ?
		WHERE id = ?`,
		p.Name, p.BirthDate, p.Gender, p.Height, p.Weight,
		p.Age, p.AgeMonths, p.AgeDisplay, patientID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// calcAgeFromBirthDate — birthDate (YYYY-MM-DD) → age, ageMonths, ageDisplay.
// Пустая строка → все нули/пусто (пациент не указал дату).
// Возвращает ошибку, если дата не парсится или в будущем — вызывающий
// решает, вернуть 400 или проигнорировать (в PUT — 400).
func calcAgeFromBirthDate(birthDate string, now time.Time) (age, ageMonths int, display string, err error) {
	birthDate = strings.TrimSpace(birthDate)
	if birthDate == "" {
		return 0, 0, "", nil
	}
	bd, perr := time.Parse(dateLayout, birthDate)
	if perr != nil {
		return 0, 0, "", errInvalidBirthDate
	}
	if bd.After(now) {
		return 0, 0, "", errBirthDateFuture
	}
	years := now.Year() - bd.Year()
	months := int(now.Month()) - int(bd.Month())
	if now.Day() < bd.Day() {
		months--
	}
	if months < 0 {
		years--
		months += 12
	}
	ageMonths = years*12 + months
	return years, ageMonths, formatAgeDisplay(years, months), nil
}

// formatAgeDisplay — человекочитаемый возраст: «3 г 5 мес», «7 мес», «2 г».
func formatAgeDisplay(years, months int) string {
	switch {
	case years <= 0 && months <= 0:
		return "0 мес"
	case years <= 0:
		return profileItoa(months) + " мес"
	case months == 0:
		return profileItoa(years) + " г"
	default:
		return profileItoa(years) + " г " + profileItoa(months) + " мес"
	}
}

func profileItoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// Ошибки валидации — для различения 400-причин в тестах/логах.
var (
	errInvalidBirthDate = &profileErr{"birthDate must be YYYY-MM-DD"}
	errBirthDateFuture  = &profileErr{"birthDate must not be in the future"}
)

type profileErr struct{ msg string }

func (e *profileErr) Error() string { return e.msg }

// handlePatientProfile — GET / PUT /api/patient/profile.
func handlePatientProfile(w http.ResponseWriter, r *http.Request) {
	pid, err := resolvePatientIDForSelf(r)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"patient account not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch r.Method {
	case http.MethodGet:
		p, err := getPatientProfile(db, pid)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)

	case http.MethodPut:
		var body profileBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Текущий профиль — база для «частичного» PUT.
		cur, err := getPatientProfile(db, pid)
		if err == sql.ErrNoRows {
			http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Накатываем присланные поля.
		if body.Name != nil {
			n := strings.TrimSpace(*body.Name)
			if n == "" {
				http.Error(w, "name must not be empty", http.StatusBadRequest)
				return
			}
			cur.Name = n
		}
		if body.Gender != nil {
			g := strings.TrimSpace(*body.Gender)
			if g != "" && g != "male" && g != "female" && g != "other" {
				http.Error(w, "gender must be male, female or other", http.StatusBadRequest)
				return
			}
			cur.Gender = g
		}
		if body.Height != nil {
			h := *body.Height
			if h < 0 || h > maxHeightCm {
				http.Error(w, "height out of range (0..250 cm)", http.StatusBadRequest)
				return
			}
			cur.Height = h
		}
		if body.Weight != nil {
			wt := *body.Weight
			if wt < 0 || wt > maxWeightKg {
				http.Error(w, "weight out of range (0..500 kg)", http.StatusBadRequest)
				return
			}
			cur.Weight = wt
		}
		if body.BirthDate != nil {
			bd := strings.TrimSpace(*body.BirthDate)
			age, ageMonths, display, perr := calcAgeFromBirthDate(bd, time.Now().UTC())
			if perr != nil {
				http.Error(w, perr.Error(), http.StatusBadRequest)
				return
			}
			cur.BirthDate = bd
			cur.Age = age
			cur.AgeMonths = ageMonths
			cur.AgeDisplay = display
		}

		if err := updatePatientProfile(db, pid, cur); err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, `{"error":"patient not found"}`, http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		auditLog(db, patientIDFromCtx(r), "patient.profile.update", "patient", pid, clientIP(r), nil)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(cur)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// registerPatientProfileRoutes — self-profile за patientAuthMiddleware.
func registerPatientProfileRoutes() {
	http.Handle("/api/patient/profile",
		patientAuthMiddleware(http.HandlerFunc(handlePatientProfile)))
	slog.Info("registerPatientProfileRoutes", "routes", "/api/patient/profile")
}

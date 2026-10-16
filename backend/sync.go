package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

// B30.4 — sync: GET /api/patients/{id}/sync.
//
// Врач получает vitals_history пациента при наличии approved consent
// со scope='vitals_read'. Событие пишется в consents_audit (см. consent.go),
// а не в общий audit — это событие consent-контура.
//
// Контракт:
//
//   GET /api/patients/{id}/sync?limit=N   (doctor-only)
//
//   200 {"patientId":...,"consentId":...,"limit":N,"records":[...]}
//   403 {"error":"doctor only"}         — нет doctor-контекста
//   403 {"error":"consent_required"}    — нет approved consent / чужой scope
//   403 {"error":"consent_expired"}     — expires_at < now()
//   400 {"error":"invalid limit"}       — limit не число / <1 / >500
//   405 {"error":"method not allowed"}
//
// Отложено (§6.67): offset, from/to, пагинация курсором.
// Дефолт limit=50 (совместим с getVitalsHistory), max=500.

const (
	syncScope        = "vitals_read"
	syncDefaultLimit = 50
	syncMaxLimit     = 500
)

// tsLayout — формат timestamp'ов проекта (см. consent.go, audit.go).
const tsLayout = "2006-01-02 15:04:05"

// handlePatientSync — GET /api/patients/{id}/sync.
func handlePatientSync(w http.ResponseWriter, r *http.Request, patientID string) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	doctorID := doctorIDFromCtx(r)
	if doctorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusForbidden)
		return
	}
	if patientID == "" {
		http.Error(w, `{"error":"patient id required"}`, http.StatusBadRequest)
		return
	}

	limit, err := parseSyncLimit(r.URL.Query().Get("limit"))
	if err != nil {
		http.Error(w, `{"error":"invalid limit"}`, http.StatusBadRequest)
		return
	}

	// 1. Ищем активный approved consent со scope='vitals_read'.
	//    Существование пациента специально не проверяем: нет consent -> 403,
	//    не палим, есть ли такой пациент (решение #4, §6.67).
	consentID, expiresAt, err := findApprovedConsent(patientID, doctorID, syncScope)
	if errors.Is(err, sql.ErrNoRows) {
		consentAudit(db, "", patientID, doctorID, "read_denied", "doctor", doctorID, clientIP(r))
		http.Error(w, `{"error":"consent_required"}`, http.StatusForbidden)
		return
	}
	if err != nil {
		slog.Warn("sync: findApprovedConsent", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// 2. Проверяем expires_at (NULL -> бессрочный; в B30.3 TTL не выставляется).
	if expired, bad := checkConsentExpired(expiresAt); bad {
		slog.Warn("sync: bad expires_at", "consentId", consentID, "raw", expiresAt.String)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	} else if expired {
		consentAudit(db, consentID, patientID, doctorID, "read_denied", "doctor", doctorID, clientIP(r))
		http.Error(w, `{"error":"consent_expired"}`, http.StatusForbidden)
		return
	}

	// 3. Читаем историю. getVitalsHistory уже есть в db.go (дефолт 50).
	records, err := getVitalsHistory(db, patientID, limit)
	if err != nil {
		slog.Warn("sync: getVitalsHistory", "err", err, "patientId", patientID)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// 4. Аудит успешного чтения (общий audit — doctor действие).
	auditLog(db, doctorID, "vitals.read", "patient", patientID, clientIP(r), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"patientId": patientID,
		"consentId": consentID,
		"limit":     limit,
		"records":   records,
	})
}

// parseSyncLimit разбирает ?limit=. Пусто -> syncDefaultLimit.
// Не число, <=0 или >syncMaxLimit -> ошибка.
func parseSyncLimit(raw string) (int, error) {
	if raw == "" {
		return syncDefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > syncMaxLimit {
		return 0, errors.New("invalid limit")
	}
	return n, nil
}

// findApprovedConsent возвращает (id, expires_at) активного approved consent.
// sql.ErrNoRows — если нет подходящей строки.
// expires_at — TEXT (nullable), поэтому sql.NullString.
func findApprovedConsent(patientID, doctorID, scope string) (string, sql.NullString, error) {
	var (
		id        string
		expiresAt sql.NullString
	)
	row := db.QueryRow(`
                SELECT id, expires_at
                FROM consents
                WHERE patient_id = ? AND doctor_id = ? AND scope = ? AND status = 'approved'
                LIMIT 1
        `, patientID, doctorID, scope)
	if err := row.Scan(&id, &expiresAt); err != nil {
		return "", sql.NullString{}, err
	}
	return id, expiresAt, nil
}

// checkConsentExpired разбирает expires_at.
//   - NULL или пусто  -> (false, false) — бессрочный.
//   - парсится и <now -> (true,  false) — истёк.
//   - парсится и >=now-> (false, false).
//   - не парсится     -> (false, true)  — битые данные, 500.
func checkConsentExpired(raw sql.NullString) (expired, bad bool) {
	if !raw.Valid || raw.String == "" {
		return false, false
	}
	t, err := time.Parse(tsLayout, raw.String)
	if err != nil {
		return false, true
	}
	return t.Before(time.Now().UTC()), false
}

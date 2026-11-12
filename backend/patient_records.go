package main

// patient_records.go — B30.9c: self-records пациента (этап 2 §6.58).
//
// Сущности: symptoms, allergies, vaccinations, diagnoses.
// Модель — как patient_vitals.go:
//   patientIDFromCtx(r)   → patient_accounts.id (accountID)
//   resolvePatientIDForSelf → patients.id (pid)
//   clinic_id             → patients.clinic_id (снимок на момент записи)
//
// API (все под patientAuthMiddleware):
//   GET    /api/patient/records/{kind}       — список живых своих
//   POST   /api/patient/records/{kind}       — upsert по id (server-wins)
//   DELETE /api/patient/records/{kind}/{id}  — soft delete
//
// kind ∈ {symptoms, allergies, vaccinations, diagnoses}.
// Upsert по id: id — TEXT (клиентский Date.now()), created_at не меняется,
// updated_at = now. Soft delete: deleted_at = now, запись остаётся.

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var recordTables = map[string]bool{
	"symptoms":             true,
	"allergies":            true,
	"vaccinations":         true,
	"diagnoses":            true,
	"constant_medications": true,
	"course_medications":   true,
	"files":                true,
}

// recordKindFromPath — /api/patient/records/{kind}[/{id}] → kind, id.
func recordKindFromPath(p string) (kind, id string, ok bool) {
	const prefix = "/api/patient/records/"
	if !strings.HasPrefix(p, prefix) {
		return "", "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	if rest == "" {
		return "", "", false
	}
	parts := strings.SplitN(rest, "/", 2)
	kind = parts[0]
	if !recordTables[kind] {
		return "", "", false
	}
	if len(parts) == 2 {
		id = parts[1]
	}
	return kind, id, true
}

// clinicIDForPatient — patients.clinic_id (может быть NULL → "").
func clinicIDForPatient(pid string) (sql.NullString, error) {
	var c sql.NullString
	err := db.QueryRow(`SELECT clinic_id FROM patients WHERE id=?`, pid).Scan(&c)
	return c, err
}

// handlePatientRecords — общий диспатчер для 4 сущностей.
func handlePatientRecords(w http.ResponseWriter, r *http.Request) {
	kind, id, ok := recordKindFromPath(r.URL.Path)
	if !ok {
		http.Error(w, `{"error":"unknown record kind"}`, http.StatusNotFound)
		return
	}

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
		if id != "" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		list, err := listPatientRecords(kind, pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)

	case http.MethodPost:
		if id != "" {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)) // 8 MiB: files.data (base64 ≈ 6 MiB бинаря)
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		rid, err := upsertPatientRecord(kind, pid, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": rid})

	case http.MethodDelete:
		if id == "" {
			http.Error(w, "id required", http.StatusBadRequest)
			return
		}
		n, err := softDeletePatientRecord(kind, pid, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if n == 0 {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"ok": true})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// recordFields — белый список колонок по kind (кроме id/patient_id/clinic_id/
// created_at/updated_at/deleted_at — их ставим сами).
var recordFields = map[string][]string{
	"symptoms":             {"name", "severity", "onset", "notes"},
	"allergies":            {"allergen", "reaction", "severity", "notes"},
	"vaccinations":         {"vaccine", "dose", "administered", "notes"},
	"diagnoses":            {"code", "title", "diagnosed_at", "status", "notes"},
	"constant_medications": {"name", "dose", "schedule", "notes"},
	"course_medications":   {"name", "dose", "start_at", "end_at", "status", "notes"},
	"files":                {"filename", "mime", "size_bytes", "data", "notes"},
}

func listPatientRecords(kind, pid string) ([]map[string]any, error) {
	cols := append([]string{"id"}, recordFields[kind]...)
	// files: не отдаём BLOB в списке — только флаг has_data (BLOB качается отдельно).
	if kind == "files" {
		cols = []string{"id", "filename", "mime", "size_bytes", "notes"}
	}
	cols = append(cols, "created_at", "updated_at")
	selectCols := strings.Join(cols, ",")
	if kind == "files" {
		selectCols += ", CASE WHEN storage_kind != '' AND (data IS NOT NULL OR storage_key IS NOT NULL) THEN 1 ELSE 0 END AS has_data"
	}
	q := `SELECT ` + selectCols + ` FROM ` + kind +
		` WHERE patient_id=? AND deleted_at IS NULL ORDER BY updated_at DESC`
	rows, err := db.Query(q, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	n := len(cols)
	if kind == "files" {
		n++ // + has_data (CASE WHEN ...) — не входит в cols, но есть в SELECT
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, n)
		ptrs := make([]any, n)
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := map[string]any{}
		for i, c := range cols {
			m[c] = vals[i]
		}
		if kind == "files" {
			m["has_data"] = vals[n-1]
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// newFileID — серверный id для нового файла (без внешних зависимостей).
// Формат: "F" + unix-nano (монотонно, сортируется по времени).
func newFileID() string {
	return fmt.Sprintf("F%d", time.Now().UnixNano())
}

func upsertPatientRecord(kind, pid string, payload map[string]any) (string, error) {
	if kind == "files" {
		return upsertFileRecord(pid, payload)
	}
	rid, _ := payload["id"].(string)
	if strings.TrimSpace(rid) == "" {
		if kind == "files" {
			// B30.11-fix: загрузка нового файла — id выдаёт сервер.
			// Для остальных kind id обязателен (клиентский upsert по Date.now()).
			rid = newFileID()
			payload["id"] = rid
		} else {
			return "", errors.New("id required")
		}
	}

	// B30.11-fix: фронт patient_cabinet_full.html шлёт data_base64 (B30.11),
	// а тесты/API-контракт B30.9c — data. Принимаем оба, data приоритетнее.
	if _, ok := payload["data"]; !ok {
		if alt, ok2 := payload["data_base64"]; ok2 && alt != nil {
			payload["data"] = alt
		}
	}

	// files: data приходит base64-строкой → декодируем в []byte (BLOB).
	// size_bytes при наличии data вычисляем сами (клиентское значение игнорируем).
	if kind == "files" {
		if raw, ok := payload["data"]; ok && raw != nil {
			s, isStr := raw.(string)
			if !isStr {
				return "", errors.New("data must be base64 string")
			}
			if s == "" {
				payload["data"] = nil
				delete(payload, "size_bytes")
			} else {
				decoded, decErr := base64.StdEncoding.DecodeString(s)
				if decErr != nil {
					return "", fmt.Errorf("data base64: %w", decErr)
				}
				payload["data"] = decoded
				payload["size_bytes"] = int64(len(decoded))
			}
		}
	}

	clinic, err := clinicIDForPatient(pid)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	fields := recordFields[kind]
	// Partial upsert: в INSERT/UPDATE включаем только поля, реально
	// присутствующие в payload. Отсутствующие не трогаем — сработает
	// DEFAULT/NULL схемы (например course_medications.status DEFAULT 'active').
	vals := make([]any, 0, len(fields)+5)
	vals = append(vals, rid, pid)
	var clinicVal any
	if clinic.Valid {
		clinicVal = clinic.String
	} else {
		clinicVal = nil
	}
	vals = append(vals, clinicVal)

	cols := []string{"id", "patient_id", "clinic_id"}
	placeholders := []string{"?", "?", "?"}
	updates := []string{}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, f := range fields {
		v, ok := payload[f]
		if !ok {
			continue
		}
		cols = append(cols, f)
		placeholders = append(placeholders, "?")
		vals = append(vals, v)
		updates = append(updates, f+"=excluded."+f)
	}
	cols = append(cols, "created_at", "updated_at")
	placeholders = append(placeholders, "?", "?")
	vals = append(vals, now, now)
	updates = append(updates, "updated_at=excluded.updated_at")

	q := `INSERT INTO ` + kind + ` (` + strings.Join(cols, ",") + `) VALUES (` +
		strings.Join(placeholders, ",") + `) ON CONFLICT(id) DO UPDATE SET ` +
		strings.Join(updates, ", ") +
		` WHERE ` + kind + `.patient_id = excluded.patient_id`
	if _, err := db.Exec(q, vals...); err != nil {
		return "", err
	}
	return rid, nil
}

// upsertFileRecord — files: через FileStorage. Для blobStorage — data в БД,
// для fsStorage — файл на диск + storage_key, data=NULL. Контракт HTTP не меняется.
func upsertFileRecord(pid string, payload map[string]any) (string, error) {
	rid, _ := payload["id"].(string)
	if strings.TrimSpace(rid) == "" {
		rid = newFileID()
		payload["id"] = rid
	}
	if _, ok := payload["data"]; !ok {
		if alt, ok2 := payload["data_base64"]; ok2 && alt != nil {
			payload["data"] = alt
		}
	}

	// декод base64 → []byte
	var data []byte
	if raw, ok := payload["data"]; ok && raw != nil {
		s, isStr := raw.(string)
		if !isStr {
			return "", errors.New("data must be base64 string")
		}
		if s != "" {
			decoded, decErr := base64.StdEncoding.DecodeString(s)
			if decErr != nil {
				return "", fmt.Errorf("data base64: %w", decErr)
			}
			data = decoded
		}
	}

	filename, _ := payload["filename"].(string)
	mime, _ := payload["mime"].(string)
	notes, _ := payload["notes"].(string)

	if storage == nil {
		return "", errors.New("storage не инициализирован")
	}

	clinic, err := clinicIDForPatient(pid)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}
	clinicVal := any(nil)
	if clinic.Valid {
		clinicVal = clinic.String
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// 1) запись в хранилище (blob → files.data, fs → диск).
	if len(data) > 0 {
		if err := storage.Put(pid, rid, filename, mime, data); err != nil {
			return "", fmt.Errorf("storage.Put: %w", err)
		}
	}

	// 2) метаданные в files. Для blob — data в БД (совместимо со старым);
	//    для fs — data=NULL, storage_kind='fs', storage_key='<pid>/<id>.bin'.
	var (
		kind  string
		key   any
		size  any
		dataV any
	)
	switch storage.(type) {
	case *blobStorage:
		kind = string(StorageKindBlob)
		key = nil
		if len(data) > 0 {
			size = int64(len(data))
			dataV = data
		}
	case *fsStorage:
		kind = string(StorageKindFS)
		if len(data) > 0 {
			key = filepath.Join(pid, rid+".bin")
			size = int64(len(data))
		}
	default:
		return "", fmt.Errorf("storage: неизвестный тип %T", storage)
	}

	q := `INSERT INTO files (id, patient_id, clinic_id, filename, mime, size_bytes, data, notes,
	                         storage_kind, storage_key, created_at, updated_at)
	      VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	      ON CONFLICT(id) DO UPDATE SET
	        filename=excluded.filename, mime=excluded.mime,
	        size_bytes=excluded.size_bytes, data=COALESCE(excluded.data, files.data), notes=excluded.notes,
	        storage_kind=excluded.storage_kind, storage_key=excluded.storage_key,
	        updated_at=excluded.updated_at
	      WHERE files.patient_id = excluded.patient_id`
	if _, err := db.Exec(q, rid, pid, clinicVal, filename, mime, size, dataV, notes,
		kind, key, now, now); err != nil {
		// откат файла на диске — best-effort
		if kind == string(StorageKindFS) && len(data) > 0 {
			_ = storage.Delete(pid, rid)
		}
		return "", err
	}
	return rid, nil
}

func softDeletePatientRecord(kind, pid, id string) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(`UPDATE `+kind+` SET deleted_at=?, updated_at=? WHERE id=? AND patient_id=? AND deleted_at IS NULL`,
		now, now, id, pid)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	// B30.10 §6.59: для files — удалить файл в FS (best-effort; blob — no-op).
	if n > 0 && kind == "files" && storage != nil {
		_ = storage.Delete(pid, id)
	}
	return n, nil
}

// registerPatientRecordsRoutes — все 4 сущности под patientAuthMiddleware.
// parseFilesDataPath — /api/patient/records/files/{id}/data → id, true.
func parseFilesDataPath(p string) (id string, ok bool) {
	const prefix = "/api/patient/records/files/"
	if !strings.HasPrefix(p, prefix) {
		return "", false
	}
	rest := strings.TrimPrefix(p, prefix)
	// ожидаем ровно "{id}/data"
	parts := strings.Split(rest, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "data" {
		return "", false
	}
	return parts[0], true
}

// handlePatientFileDownload — GET /api/patient/records/files/{id}/data (self).
// Отдаём BLOB c Content-Type из files.mime (fallback octet-stream) и
// Content-Disposition: attachment; filename="...".
func handlePatientFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fid, ok := parseFilesDataPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	pid, err := resolvePatientIDForSelf(r)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"patient account not found"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeFileBlob(w, pid, fid)
}

// handleDoctorPatientFileData — GET /api/patients/{id}/files/{fid}/data (doctor).
// Требует approved consent scope=files_read.
func handleDoctorPatientFileData(w http.ResponseWriter, r *http.Request, pid, fid string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	doctorID := doctorIDFromCtx(r)
	if doctorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusUnauthorized)
		return
	}
	if _, err := getPatientByID(db, pid); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, expiresAt, err := findApprovedConsent(pid, doctorID, "files_read")
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"consent_required"}`, http.StatusForbidden)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if expired, bad := checkConsentExpired(expiresAt); bad {
		http.Error(w, `{"error":"consent_bad_expires"}`, http.StatusInternalServerError)
		return
	} else if expired {
		http.Error(w, `{"error":"consent_expired"}`, http.StatusForbidden)
		return
	}
	writeFileBlob(w, pid, fid)
}

// handleDoctorPatientRecordDelete — DELETE /api/patients/{id}/{kind}/{rid} (doctor).
// Требует approved consent scope=kind+"_write". Soft delete; 404 если записи
// нет / она чужая / уже удалена.
func handleDoctorPatientRecordDelete(w http.ResponseWriter, r *http.Request, pid, kind, rid string) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !requireDoctorConsent(w, r, pid, kind, "write") {
		return
	}
	n, err := softDeletePatientRecord(kind, pid, rid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if n == 0 {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

// writeFileBlob — общая часть: SELECT filename, mime, data FROM files
// WHERE id=? AND patient_id=? AND deleted_at IS NULL. NULL data → 404.
// writeFileBlob — метаданные из files (filename/mime), байты — из storage.
// Контракт: 200 + bytes, Content-Disposition: attachment.
func writeFileBlob(w http.ResponseWriter, pid, fid string) {
	if storage == nil {
		http.Error(w, "storage не инициализирован", http.StatusInternalServerError)
		return
	}
	var filename, mime sql.NullString
	err := db.QueryRow(`SELECT filename, mime FROM files
		WHERE id=? AND patient_id=? AND deleted_at IS NULL`, fid, pid).
		Scan(&filename, &mime)
	if err == sql.ErrNoRows {
		http.NotFound(w, nil)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, mimeFromStorage, data, ok, err := storage.Get(pid, fid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !ok || len(data) == 0 {
		http.NotFound(w, nil)
		return
	}
	ct := "application/octet-stream"
	if mime.Valid && mime.String != "" {
		ct = mime.String
	} else if mimeFromStorage != "" {
		ct = mimeFromStorage
	}
	fn := fid
	if filename.Valid && filename.String != "" {
		fn = filename.String
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(fn, `"`, "")+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// handlePatientFilesRouter — тонкий диспетчер для /api/patient/records/files/*.
// Причина: ServeMux выбирает самый длинный префикс, поэтому если зарегистрировать
// "/files/" прямо на handlePatientFileDownload, то list/upload/delete (которые не
// содержат "/{id}/data") тоже уйдут в download-хендлер и получат 404/405.
// См. регрессионный тест TestPatientFilesRouting.
func handlePatientFilesRouter(w http.ResponseWriter, r *http.Request) {
	if _, ok := parseFilesDataPath(r.URL.Path); ok {
		handlePatientFileDownload(w, r)
		return
	}
	handlePatientRecords(w, r)
}

func registerPatientRecordsRoutes() {
	// files/{id}/data → download; всё остальное под /files (list/upload/delete)
	// уходит в handlePatientRecords через диспетчер.
	// Точный путь без слэша — чтобы фронт/curl не получали 307 и не полагались
	// на автоматический редирект ServeMux (важно для POST: 307 сохраняет метод,
	// но лишний round-trip и риск потерять тело в некоторых клиентах).
	http.Handle("/api/patient/records/files",
		patientAuthMiddleware(http.HandlerFunc(handlePatientFilesRouter)))
	http.Handle("/api/patient/records/files/",
		patientAuthMiddleware(http.HandlerFunc(handlePatientFilesRouter)))
	http.Handle("/api/patient/records/",
		patientAuthMiddleware(http.HandlerFunc(handlePatientRecords)))
	slog.Info("registerPatientRecordsRoutes",
		"routes", "/api/patient/records/{symptoms,allergies,vaccinations,diagnoses,constant_medications,course_medications,files} + /files/{id}/data")
}

// requireDoctorConsent — общая проверка для doctor read/write по kind.
// action ∈ {"read","write"} → scope = kind + "_" + action.
// Возвращает true, если можно продолжать; иначе уже записал HTTP-ошибку.
//
//   - нет doctorID в ctx          → 401 doctor only
//   - пациент не существует       → 404 patient not found
//   - нет approved consent        → 403 consent_required
//   - consent истёк               → 403 consent_expired
//   - битый expires_at            → 500 consent_bad_expires
func requireDoctorConsent(w http.ResponseWriter, r *http.Request, pid, kind, action string) bool {
	actorID := doctorIDFromCtx(r)
	if actorID == "" {
		http.Error(w, `{"error":"doctor only"}`, http.StatusUnauthorized)
		return false
	}

	if _, err := getPatientByID(db, pid); err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "patient not found", http.StatusNotFound)
			return false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}

	scope := kind + "_" + action
	_, expiresAt, err := findApprovedConsent(pid, actorID, scope)
	if err == sql.ErrNoRows {
		http.Error(w, `{"error":"consent_required"}`, http.StatusForbidden)
		return false
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return false
	}
	if expired, bad := checkConsentExpired(expiresAt); bad {
		http.Error(w, `{"error":"consent_bad_expires"}`, http.StatusInternalServerError)
		return false
	} else if expired {
		http.Error(w, `{"error":"consent_expired"}`, http.StatusForbidden)
		return false
	}
	return true
}

// handleDoctorPatientRecords — /api/patients/{id}/{kind} для врача.
// GET (read, consent kind+"_read") → список живых (deleted_at IS NULL).
// POST/PUT (write, consent kind+"_write") → generic upsert по id.
// Проверка — requireDoctorConsent (401 / 404 patient / 403 / 500).
func handleDoctorPatientRecords(w http.ResponseWriter, r *http.Request, pid, kind string) {
	switch r.Method {
	case http.MethodGet:
		if !requireDoctorConsent(w, r, pid, kind, "read") {
			return
		}
		list, err := listPatientRecords(kind, pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)

	case http.MethodPost, http.MethodPut:
		if !requireDoctorConsent(w, r, pid, kind, "write") {
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 8<<20))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "invalid json: "+err.Error(), http.StatusBadRequest)
			return
		}
		rid, err := upsertPatientRecord(kind, pid, payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": rid})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

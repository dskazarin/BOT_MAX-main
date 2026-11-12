package main

// files_url.go — B30.14 шаг 2b: GET /api/files/{id}/url (presigned URL для S3).
//
// Авторизация обязательна (путь НЕ в isAuthWhitelist).
// Ответ: {"url": "...", "expires_in": 900}
// Ошибки:
//   400 — storage_kind != 's3' (presign доступен только для s3)
//   404 — файл не найден / удалён
//   501 — storage не умеет PresignURL (например, presign отключён в конфиге)

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// presigner — type-assert-интерфейс: s3Storage умеет, blob/fs — нет.
type presigner interface {
	PresignURL(pid, id string) (string, time.Duration, error)
}

// handleFileURL — GET /api/files/{id}/url.
func handleFileURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	if storage == nil || db == nil {
		http.Error(w, "storage/db не инициализированы", http.StatusInternalServerError)
		return
	}

	// /api/files/{id}/url → id
	rest := strings.TrimPrefix(r.URL.Path, "/api/files/")
	fid, ok := strings.CutSuffix(rest, "/url")
	if !ok || fid == "" || strings.Contains(fid, "/") {
		http.NotFound(w, r)
		return
	}

	var pid, kind string
	err := db.QueryRow(
		`SELECT patient_id, storage_kind FROM files
		 WHERE id=? AND deleted_at IS NULL`, fid,
	).Scan(&pid, &kind)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if kind != string(StorageKindS3) {
		http.Error(w, `presign доступен только для storage_kind=s3`, http.StatusBadRequest)
		return
	}

	ps, ok := storage.(presigner)
	if !ok {
		http.Error(w, `storage не поддерживает presign`, http.StatusNotImplemented)
		return
	}
	url, ttl, err := ps.PresignURL(pid, fid)
	if err != nil {
		// presign отключён конфигом → 501; прочие ошибки → 500.
		if strings.Contains(err.Error(), "presign отключён") {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"url":        url,
		"expires_in": int(ttl / time.Second),
	})
}

// registerFilesURLRoute — регистрация /api/files/ в main.go.
func registerFilesURLRoute() {
	http.HandleFunc("/api/files/", handleFileURL)
}

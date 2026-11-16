package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakePresigner — минимальная реализация presigner для теста.
type fakePresigner struct {
	url string
	ttl time.Duration
	err error
}

func (f *fakePresigner) PresignURL(pid, id string) (string, time.Duration, error) {
	return f.url, f.ttl, f.err
}

func TestHandleFileURL_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/files/f1/url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d, want 405", rec.Code)
	}
}

func TestHandleFileURL_NotFound(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/files//url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d, want 404/500", rec.Code)
	}
}

func TestPresignerInterface(t *testing.T) {
	var _ presigner = (*s3Storage)(nil)
	_ = fakePresigner{url: "https://x", ttl: time.Minute}
	_ = json.Marshal
}

// --- B30.17 Шаг 2a: presign-эндпоинт, кейсы 400/501/200/500 ---

// fakePresignStorage — FileStorage + PresignURL. Для 200/500.
type fakePresignStorage struct {
	FileStorage
	url string
	ttl time.Duration
	err error
}

func (f *fakePresignStorage) PresignURL(pid, id string) (string, time.Duration, error) {
	return f.url, f.ttl, f.err
}

// withStorage — подмена глобального storage на время теста (паттерн consent_test.go).
func withStorage(t *testing.T, s FileStorage) {
	t.Helper()
	saved := storage
	storage = s
	t.Cleanup(func() { storage = saved })
}

// seedFileRow — минимальная запись files для handleFileURL.
// storage_kind добавляется миграцией db.go (TEXT NOT NULL DEFAULT 'blob').
func seedFileRow(t *testing.T, id, pid, storageKind string) {
	t.Helper()
	// FK files.patient_id → patients(id): сначала пациент+аккаунт (как в patient_records_*_test).
	seedPatientWithAccount(t, pid, "ACC-"+pid)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(
		`INSERT INTO files(id, patient_id, filename, created_at, updated_at, storage_kind, storage_key, deleted_at)
		 VALUES (?, ?, '', ?, ?, ?, '', NULL)`,
		id, pid, now, now, storageKind); err != nil {
		t.Fatalf("seed files: %v", err)
	}
}

func TestHandleFileURL_NotS3_400(t *testing.T) {
	setupConsentDB(t)
	seedFileRow(t, "f1", "PAT-1", "blob")
	withStorage(t, newBlobStorage(db))

	req := httptest.NewRequest(http.MethodGet, "/api/files/f1/url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestHandleFileURL_StorageNoPresigner_501(t *testing.T) {
	setupConsentDB(t)
	seedFileRow(t, "f1", "PAT-1", "s3")
	withStorage(t, newBlobStorage(db)) // kind=s3, но blob не реализует PresignURL

	req := httptest.NewRequest(http.MethodGet, "/api/files/f1/url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("code=%d body=%s, want 501", rec.Code, rec.Body.String())
	}
}

func TestHandleFileURL_Ok_200(t *testing.T) {
	setupConsentDB(t)
	seedFileRow(t, "f1", "PAT-1", "s3")
	withStorage(t, &fakePresignStorage{
		FileStorage: newBlobStorage(db),
		url:         "https://s3/x",
		ttl:         15 * time.Minute,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/files/f1/url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var out struct {
		URL       string `json:"url"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.URL != "https://s3/x" || out.ExpiresIn != 900 {
		t.Fatalf("out=%+v, want {https://s3/x, 900}", out)
	}
}

func TestHandleFileURL_PresignError_500(t *testing.T) {
	setupConsentDB(t)
	seedFileRow(t, "f1", "PAT-1", "s3")
	withStorage(t, &fakePresignStorage{
		FileStorage: newBlobStorage(db),
		err:         errors.New("boom"),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/files/f1/url", nil)
	rec := httptest.NewRecorder()
	handleFileURL(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d, want 500", rec.Code)
	}
}

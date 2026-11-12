package main

import (
	"encoding/json"
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

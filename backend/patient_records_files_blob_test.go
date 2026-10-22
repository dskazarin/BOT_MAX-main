package main

// patient_records_files_blob_test.go — B30.9g (этап 3 §6.58): files.data BLOB.
// data приходит base64-строкой; список не отдаёт BLOB, только has_data.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func listFiles(t *testing.T, accID string) []map[string]any {
	t.Helper()
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords(accID, http.MethodGet, "/api/patient/records/files", ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET files status = %d body=%s", rr.Code, rr.Body.String())
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return list
}

func TestPatientRecords_Files_Base64_Upload(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	raw := []byte("hello, BLOB")
	b64 := base64.StdEncoding.EncodeToString(raw)
	body := `{"id":"F-1","filename":"a.txt","mime":"text/plain","data":"` + b64 + `","notes":"n"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/files", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rr.Code, rr.Body.String())
	}

	// в БД data действительно BLOB нужной длины
	var size int
	if err := db.QueryRow(`SELECT length(data) FROM files WHERE id='F-1'`).Scan(&size); err != nil {
		t.Fatalf("select length: %v", err)
	}
	if size != len(raw) {
		t.Errorf("length(data) = %d, want %d", size, len(raw))
	}
	// size_bytes вычислен сервером
	var sizeBytes int64
	db.QueryRow(`SELECT size_bytes FROM files WHERE id='F-1'`).Scan(&sizeBytes)
	if sizeBytes != int64(len(raw)) {
		t.Errorf("size_bytes = %d, want %d", sizeBytes, len(raw))
	}

	// список: без data, has_data=1
	list := listFiles(t, "ACC-1")
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
	if _, has := list[0]["data"]; has {
		t.Errorf("list содержит data — BLOB не должен утекать в список")
	}
	if fmt.Sprint(list[0]["has_data"]) != "1" {
		t.Errorf("has_data = %v (%T), want 1", list[0]["has_data"], list[0]["has_data"])
	}
}

func TestPatientRecords_Files_NoData_HasData0(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	body := `{"id":"F-1","filename":"meta.txt","mime":"text/plain"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/files", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rr.Code, rr.Body.String())
	}
	list := listFiles(t, "ACC-1")
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
	if fmt.Sprint(list[0]["has_data"]) != "0" {
		t.Errorf("has_data = %v (%T), want 0", list[0]["has_data"], list[0]["has_data"])
	}
}

func TestPatientRecords_Files_InvalidBase64_400(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	body := `{"id":"F-1","filename":"x.bin","data":"!!!not-base64!!!"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/files", body))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "base64") {
		t.Errorf("body = %s, want base64 error", rr.Body.String())
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM files WHERE id='F-1'`).Scan(&n)
	if n != 0 {
		t.Errorf("invalid base64 persisted: %d", n)
	}
}

func TestPatientRecords_Files_TooLarge_Rejected(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	// 9 MiB "A" — тело > лимита 8 MiB (LimitReader обрежет → JSON битый → 400).
	big := strings.Repeat("A", 9<<20)
	body := `{"id":"F-1","filename":"big.bin","data":"` + big + `"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/files", body))
	if rr.Code == http.StatusOK {
		t.Fatalf("status = 200, want reject (>8MiB)")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM files WHERE id='F-1'`).Scan(&n)
	if n != 0 {
		t.Errorf("oversized payload persisted: %d", n)
	}
}

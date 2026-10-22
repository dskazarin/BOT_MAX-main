package main

// patient_records_doctor_write_files_test.go — 3g (этап 3 §6.58): doctor-upload
// files через POST /api/patients/{id}/files с consent files_write.
// Логика декодирования base64 / size_bytes / лимита 8 MiB переиспользуется
// из upsertPatientRecord (этап 3e); здесь проверяем именно doctor-путь.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctorWrite_Files_Base64_Upload(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")

	raw := []byte("hello, doctor BLOB")
	b64 := base64.StdEncoding.EncodeToString(raw)
	body := `{"id":"F-1","filename":"a.txt","mime":"text/plain","data":"` + b64 + `","notes":"n"}`

	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files", body), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rr.Code, rr.Body.String())
	}

	var size int
	if err := db.QueryRow(`SELECT length(data) FROM files WHERE id='F-1'`).Scan(&size); err != nil {
		t.Fatalf("select length: %v", err)
	}
	if size != len(raw) {
		t.Errorf("length(data) = %d, want %d", size, len(raw))
	}
	var sizeBytes int64
	db.QueryRow(`SELECT size_bytes FROM files WHERE id='F-1'`).Scan(&sizeBytes)
	if sizeBytes != int64(len(raw)) {
		t.Errorf("size_bytes = %d, want %d", sizeBytes, len(raw))
	}

	// GET (doctor, files_read): data не утекает, has_data=1.
	approveConsent(t, "PAT-1", "DOC-1", "files_read")
	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "files"), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rr.Code)
	}
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
	if _, has := list[0]["data"]; has {
		t.Errorf("list содержит data — BLOB не должен утекать")
	}
	if fmt.Sprint(list[0]["has_data"]) != "1" {
		t.Errorf("has_data = %v, want 1", list[0]["has_data"])
	}
}

func TestDoctorWrite_Files_NoData_HasData0(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")
	approveConsent(t, "PAT-1", "DOC-1", "files_read")

	body := `{"id":"F-1","filename":"meta.txt","mime":"text/plain"}`
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files", body), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status = %d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorRecords("DOC-1", "PAT-1", "files"), "PAT-1", "files")
	var list []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list len = %d", len(list))
	}
	if fmt.Sprint(list[0]["has_data"]) != "0" {
		t.Errorf("has_data = %v, want 0", list[0]["has_data"])
	}
}

func TestDoctorWrite_Files_InvalidBase64_400(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")

	body := `{"id":"F-1","filename":"x.bin","data":"!!!not-base64!!!"}`
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files", body), "PAT-1", "files")
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

func TestDoctorWrite_Files_TooLarge_Rejected(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")

	big := strings.Repeat("A", 9<<20)
	body := `{"id":"F-1","filename":"big.bin","data":"` + big + `"}`
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files", body), "PAT-1", "files")
	if rr.Code == http.StatusOK {
		t.Fatalf("status = 200, want reject (>8MiB)")
	}
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM files WHERE id='F-1'`).Scan(&n)
	if n != 0 {
		t.Errorf("oversized payload persisted: %d", n)
	}
}

func TestDoctorWrite_Files_PutMetaKeepsBlob(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")

	raw := []byte("persist me")
	b64 := base64.StdEncoding.EncodeToString(raw)
	body := `{"id":"F-1","filename":"a.txt","mime":"text/plain","data":"` + b64 + `"}`
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files", body), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rr.Code, rr.Body.String())
	}

	// PUT без data — меняем только notes; BLOB и size_bytes не трогаем.
	rr = httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPut, "DOC-1",
		"/api/patients/PAT-1/files",
		`{"id":"F-1","notes":"обновлено"}`), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rr.Code, rr.Body.String())
	}

	var size int
	if err := db.QueryRow(`SELECT length(data) FROM files WHERE id='F-1'`).Scan(&size); err != nil {
		t.Fatalf("length(data): %v", err)
	}
	if size != len(raw) {
		t.Errorf("BLOB после PUT: length=%d, want %d (не должен затираться)", size, len(raw))
	}
	var notes string
	db.QueryRow(`SELECT notes FROM files WHERE id='F-1'`).Scan(&notes)
	if notes != "обновлено" {
		t.Errorf("notes = %q, want обновлено", notes)
	}
}

func TestDoctorWrite_Files_Delete_BlocksDownload(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_write")
	approveConsent(t, "PAT-1", "DOC-1", "files_read")

	raw := []byte("bye")
	b64 := base64.StdEncoding.EncodeToString(raw)
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files",
		`{"id":"F-1","filename":"a.bin","data":"`+b64+`"}`), "PAT-1", "files")
	if rr.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rr.Code, rr.Body.String())
	}

	// Скачивание до DELETE — 200.
	rr = httptest.NewRecorder()
	handleDoctorPatientFileData(rr, reqDoctorWrite(http.MethodGet, "DOC-1",
		"/api/patients/PAT-1/files/F-1/data", ""), "PAT-1", "F-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("download before delete: %d %s", rr.Code, rr.Body.String())
	}

	// DELETE.
	rr = httptest.NewRecorder()
	handleDoctorPatientRecordDelete(rr, reqDoctorWrite(http.MethodDelete, "DOC-1",
		"/api/patients/PAT-1/files/F-1", ""), "PAT-1", "files", "F-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE: %d %s", rr.Code, rr.Body.String())
	}

	// Скачивание после DELETE — 404 (soft-deleted скрыт).
	rr = httptest.NewRecorder()
	handleDoctorPatientFileData(rr, reqDoctorWrite(http.MethodGet, "DOC-1",
		"/api/patients/PAT-1/files/F-1/data", ""), "PAT-1", "F-1")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("download after delete: %d, want 404", rr.Code)
	}
}

func TestDoctorWrite_Files_ReadOnly_UploadDenied_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	approveConsent(t, "PAT-1", "DOC-1", "files_read")

	raw := []byte("x")
	b64 := base64.StdEncoding.EncodeToString(raw)
	rr := httptest.NewRecorder()
	handleDoctorPatientRecords(rr, reqDoctorWrite(http.MethodPost, "DOC-1",
		"/api/patients/PAT-1/files",
		`{"id":"F-1","filename":"a.bin","data":"`+b64+`"}`), "PAT-1", "files")
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "consent_required") {
		t.Fatalf("status=%d body=%s, want 403 consent_required", rr.Code, rr.Body.String())
	}
}

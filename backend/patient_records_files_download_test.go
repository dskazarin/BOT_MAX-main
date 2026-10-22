package main

// patient_records_files_download_test.go — B30.9g (этап 3 §6.58, 3e-2):
// скачивание BLOB files.data — self и doctor (consent files_read).

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// seedFileWithBlob — кладём files-запись с BLOB через generic upsert (base64).
func seedFileWithBlob(t *testing.T, accID, pid, fid, filename, mime string, raw []byte) {
	t.Helper()
	b64 := base64.StdEncoding.EncodeToString(raw)
	body := `{"id":"` + fid + `","filename":"` + filename + `","mime":"` + mime + `","data":"` + b64 + `"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords(accID, http.MethodPost, "/api/patient/records/files", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("seed file POST status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func reqPatientDownload(accID, fid string) *http.Request {
	ctx := withPatientID(context.Background(), accID)
	return httptest.NewRequest(http.MethodGet, "/api/patient/records/files/"+fid+"/data", nil).WithContext(ctx)
}

func reqDoctorDownload(doctorID, pid, fid string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodGet, "/api/patients/"+pid+"/files/"+fid+"/data", nil).WithContext(ctx)
}

func TestFileDownload_Self_Ok(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	raw := []byte("PDF-bytes-here")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "отчёт.pdf", "application/pdf", raw)

	rr := httptest.NewRecorder()
	handlePatientFileDownload(rr, reqPatientDownload("ACC-1", "F-1"))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Body.Bytes(); string(got) != string(raw) {
		t.Errorf("body = %q, want %q", got, raw)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q", ct)
	}
	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "отчёт.pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}
}

func TestFileDownload_Self_NoData_404(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	// метаданные без BLOB
	body := `{"id":"F-1","filename":"meta.txt","mime":"text/plain"}`
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, "/api/patient/records/files", body))
	if rr.Code != http.StatusOK {
		t.Fatalf("seed status = %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	handlePatientFileDownload(rr, reqPatientDownload("ACC-1", "F-1"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (нет BLOB)", rr.Code)
	}
}

func TestFileDownload_Self_ForeignPatient_404(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedPatientWithAccount(t, "PAT-2", "ACC-2")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "a.txt", "text/plain", []byte("x"))

	rr := httptest.NewRecorder()
	handlePatientFileDownload(rr, reqPatientDownload("ACC-2", "F-1"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (чужой пациент)", rr.Code)
	}
}

func TestFileDownload_Self_SoftDeleted_404(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "a.txt", "text/plain", []byte("x"))

	if _, err := db.Exec(`UPDATE files SET deleted_at=datetime('now') WHERE id='F-1'`); err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	rr := httptest.NewRecorder()
	handlePatientFileDownload(rr, reqPatientDownload("ACC-1", "F-1"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (soft-deleted)", rr.Code)
	}
}

func TestFileDownload_Doctor_NoConsent_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "a.txt", "text/plain", []byte("x"))

	rr := httptest.NewRecorder()
	handleDoctorPatientFileData(rr, reqDoctorDownload("DOC-1", "PAT-1", "F-1"), "PAT-1", "F-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "consent_required") {
		t.Errorf("body = %s, want consent_required", rr.Body.String())
	}
}

func TestFileDownload_Doctor_Approved_Ok(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	raw := []byte("doctor-visible")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "scan.png", "image/png", raw)
	approveConsent(t, "PAT-1", "DOC-1", "files_read")

	rr := httptest.NewRecorder()
	handleDoctorPatientFileData(rr, reqDoctorDownload("DOC-1", "PAT-1", "F-1"), "PAT-1", "F-1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if string(rr.Body.Bytes()) != string(raw) {
		t.Errorf("body = %q, want %q", rr.Body.Bytes(), raw)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestFileDownload_Doctor_WrongScope_403(t *testing.T) {
	setupConsentDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedDoctorForRefresh(t, db, "DOC-1")
	seedFileWithBlob(t, "ACC-1", "PAT-1", "F-1", "a.txt", "text/plain", []byte("x"))
	approveConsent(t, "PAT-1", "DOC-1", "symptoms_read") // не files_read

	rr := httptest.NewRecorder()
	handleDoctorPatientFileData(rr, reqDoctorDownload("DOC-1", "PAT-1", "F-1"), "PAT-1", "F-1")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (чужой scope)", rr.Code)
	}
}

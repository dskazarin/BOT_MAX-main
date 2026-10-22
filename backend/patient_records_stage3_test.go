package main

// patient_records_stage3_test.go — B30.9g (этап 3 §6.58): self-CRUD
// для constant_medications / course_medications / files.
// Generic-диспатчер (patient_records.go) работает по recordTables/recordFields;
// здесь проверяем, что новые kind действительно проходят upsert/list/soft-delete
// и изоляцию между пациентами. BLOB-специфика files — в 3e.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// generic: upsert → list → soft-delete → list пуст.
func testGenericSelfCRUD(t *testing.T, kind, path, body string, wantKey, wantVal string) {
	t.Helper()
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")

	// upsert
	rr := httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, path, body))
	if rr.Code != http.StatusOK {
		t.Fatalf("POST %s status = %d body=%s", kind, rr.Code, rr.Body.String())
	}

	// list
	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodGet, path, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d", kind, rr.Code)
	}
	var list []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode %s: %v", kind, err)
	}
	if len(list) != 1 {
		t.Fatalf("%s list len = %d, want 1: %+v", kind, len(list), list)
	}
	if got, _ := list[0][wantKey].(string); got != wantVal {
		t.Errorf("%s %s = %q, want %q", kind, wantKey, got, wantVal)
	}

	// id из list — для DELETE
	rid, _ := list[0]["id"].(string)
	if rid == "" {
		t.Fatalf("%s list[0].id empty", kind)
	}

	// soft delete
	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodDelete, path+"/"+rid, ""))
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE %s status = %d body=%s", kind, rr.Code, rr.Body.String())
	}

	// list снова пуст
	rr = httptest.NewRecorder()
	handlePatientRecords(rr, reqRecords("ACC-1", http.MethodGet, path, ""))
	var list2 []map[string]any
	json.Unmarshal(rr.Body.Bytes(), &list2)
	if len(list2) != 0 {
		t.Errorf("%s after delete len = %d, want 0", kind, len(list2))
	}
}

func TestPatientRecords_Stage3_ConstantMedications_CRUD(t *testing.T) {
	testGenericSelfCRUD(t, "constant_medications",
		"/api/patient/records/constant_medications",
		`{"id":"CM-1","name":"метформин","dose":"500mg","schedule":"2x/день","notes":"с едой"}`,
		"name", "метформин")
}

func TestPatientRecords_Stage3_CourseMedications_CRUD(t *testing.T) {
	testGenericSelfCRUD(t, "course_medications",
		"/api/patient/records/course_medications",
		`{"id":"CR-1","name":"амоксициллин","dose":"1g","start_at":"2026-10-20","end_at":"2026-10-27","status":"active"}`,
		"name", "амоксициллин")
}

func TestPatientRecords_Stage3_Files_CRUD_MetaOnly(t *testing.T) {
	// files без BLOB-поля data: проверяем метаданные (имя/mime/размер).
	testGenericSelfCRUD(t, "files",
		"/api/patient/records/files",
		`{"id":"F-1","filename":"анализ.pdf","mime":"application/pdf","size_bytes":12345,"notes":"кровь"}`,
		"filename", "анализ.pdf")
}

func TestPatientRecords_Stage3_Isolation(t *testing.T) {
	// PAT-2 не может удалить запись PAT-1 ни в одном из трёх новых kind.
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
	seedPatientWithAccount(t, "PAT-2", "ACC-2")

	for _, c := range []struct{ path, body string }{
		{"/api/patient/records/constant_medications", `{"id":"CM-1","name":"x"}`},
		{"/api/patient/records/course_medications", `{"id":"CR-1","name":"x"}`},
		{"/api/patient/records/files", `{"id":"F-1","filename":"x"}`},
	} {
		rr := httptest.NewRecorder()
		handlePatientRecords(rr, reqRecords("ACC-1", http.MethodPost, c.path, c.body))
		if rr.Code != http.StatusOK {
			t.Fatalf("seed POST %s status = %d body=%s", c.path, rr.Code, rr.Body.String())
		}
	}

	for _, c := range []struct{ path, id string }{
		{"/api/patient/records/constant_medications", "CM-1"},
		{"/api/patient/records/course_medications", "CR-1"},
		{"/api/patient/records/files", "F-1"},
	} {
		rr := httptest.NewRecorder()
		handlePatientRecords(rr, reqRecords("ACC-2", http.MethodDelete, c.path+"/"+c.id, ""))
		if rr.Code != http.StatusNotFound {
			t.Errorf("foreign DELETE %s/%s status = %d, want 404", c.path, c.id, rr.Code)
		}
	}
}

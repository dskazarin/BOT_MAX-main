package main

// patient_records_files_routing_test.go — B30.11-fix: регрессия на маршрутизацию
// /api/patient/records/files*. ServeMux выбирает самый длинный префикс, поэтому
// "/files/" целиком уходил в handlePatientFileDownload, и list/upload/delete
// получали 404/405. Тест проверяет parseFilesDataPath — именно он решает,
// куда делегировать в handlePatientFilesRouter.

import "testing"

func TestPatientFilesRouting(t *testing.T) {
	cases := []struct {
		path     string
		wantData bool // true → download (handlePatientFileDownload)
	}{
		{"/api/patient/records/files/", false},          // list
		{"/api/patient/records/files", false},           // list (без слэша; ServeMux редиректит)
		{"/api/patient/records/files/abc", false},       // delete → records
		{"/api/patient/records/files/abc/data", true},   // download
		{"/api/patient/records/files//data", false},     // пустой id — не download
		{"/api/patient/records/files/abc/other", false}, // не data → records (404 внутри)
	}
	for _, tc := range cases {
		if _, ok := parseFilesDataPath(tc.path); ok != tc.wantData {
			t.Errorf("parseFilesDataPath(%q) ok=%v, want %v", tc.path, ok, tc.wantData)
		}
	}
}

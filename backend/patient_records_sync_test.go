package main

// patient_records_sync_test.go — B30.17 Шаг 3a: batch-sync doctor-write.
//
// Покрытие:
//   - 405 GET; 403 no-doctor; 403 no-consent (один kind без *_write)
//   - 400 invalid json; 400 batch_too_large (101 op)
//   - 200 батч create -> все ok, записи в БД
//   - идемпотентность: повтор батча -> все dup, COUNT не вырос
//   - частичный успех: 1 валидная + 1 invalid (неизвестный kind) -> 200 ok/invalid
//   - конфликт LWW: update с updated_at старее серверного -> conflict, серверная не изменилась
//   - read-after-write: после sync GET /api/patients/{id}/{kind} отдаёт созданное
//   - конкурентные ретраи: 2 горутины шлют одинаковый create -> 1 запись, статусы {ok,dup}

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// reqSyncPost — POST /api/patients/{id}/records/sync с doctor-контекстом.
func reqSyncPost(doctorID, patientID string, body []byte) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodPost,
		"/api/patients/"+patientID+"/records/sync",
		bytes.NewReader(body)).WithContext(ctx)
}

// callRecordsSync — вызывает handlePatientRecordsSync напрямую.
func callRecordsSync(t *testing.T, r *http.Request, patientID string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	handlePatientRecordsSync(w, r, patientID)
	return w
}

// seedApprovedWriteConsents — approved consent на все 4 *_write для (pid, did).
func seedApprovedWriteConsents(t *testing.T, pid, did string, kinds ...string) {
	t.Helper()
	for _, k := range kinds {
		if _, err := db.Exec(`INSERT INTO consents
                        (id, patient_id, doctor_id, scope, status, requested_at, approved_at)
                        VALUES (?,?,?,?, 'approved', ?, ?)`,
			"C-"+pid+"-"+did+"-"+k, pid, did, k+"_write",
			time.Now().UTC().Format(tsLayout),
			time.Now().UTC().Format(tsLayout)); err != nil {
			t.Fatalf("seed consent %s: %v", k, err)
		}
	}
}

// setupSyncWriteEnv — doctor + patient + approved *_write для 4 kind'ов.
func setupSyncWriteEnv(t *testing.T) (doctorID, patientID string) {
	t.Helper()
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")
	seedApprovedWriteConsents(t, "PAT-1", "DOC-1",
		"symptoms", "allergies", "vaccinations", "diagnoses")
	return "DOC-1", "PAT-1"
}

func mkCreateOp(clientOpID, kind, id string, extra map[string]any) map[string]any {
	p := map[string]any{"id": id, "name": "x"}
	for k, v := range extra {
		p[k] = v
	}
	return map[string]any{
		"client_op_id": clientOpID,
		"kind":         kind,
		"action":       "create",
		"payload":      p,
	}
}

func mkSyncBody(t *testing.T, ops ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"ops": ops})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

type syncResp struct {
	Results []struct {
		ClientOpID string `json:"client_op_id"`
		Status     string `json:"status"`
		RecordID   string `json:"record_id"`
	} `json:"results"`
}

func decodeSyncResp(t *testing.T, w *httptest.ResponseRecorder) syncResp {
	t.Helper()
	var r syncResp
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	return r
}

// ---------------------------------------------------------------

func TestRecordsSync_MethodNotAllowed(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	r := reqSyncPost(did, pid, nil)
	r.Method = http.MethodGet
	w := callRecordsSync(t, r, pid)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestRecordsSync_NoDoctor(t *testing.T) {
	_, pid := setupSyncWriteEnv(t)
	r := httptest.NewRequest(http.MethodPost,
		"/api/patients/"+pid+"/records/sync",
		bytes.NewReader(mkSyncBody(t, mkCreateOp("op1", "symptoms", "S1", nil))))
	w := callRecordsSync(t, r, pid)
	if w.Code != http.StatusForbidden || !hasErr(w, "doctor only") {
		t.Fatalf("want 403 doctor only, got %d %s", w.Code, w.Body.String())
	}
}

func TestRecordsSync_NoConsent(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")
	// consent только на symptoms_write, нет diagnoses_write
	seedApprovedWriteConsents(t, "PAT-1", "DOC-1", "symptoms")

	body := mkSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", nil),
		mkCreateOp("op2", "diagnoses", "D1", nil),
	)
	w := callRecordsSync(t, reqSyncPost("DOC-1", "PAT-1", body), "PAT-1")
	if w.Code != http.StatusForbidden || !hasErr(w, "consent_required") {
		t.Fatalf("want 403 consent_required, got %d %s", w.Code, w.Body.String())
	}
}

func TestRecordsSync_InvalidJSON(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	w := callRecordsSync(t, reqSyncPost(did, pid, []byte("{bad")), pid)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestRecordsSync_EmptyOps(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	w := callRecordsSync(t, reqSyncPost(did, pid, []byte(`{"ops":[]}`)), pid)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestRecordsSync_BatchTooLarge(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	ops := make([]map[string]any, 0, maxSyncOps+1)
	for i := 0; i <= maxSyncOps; i++ {
		ops = append(ops, mkCreateOp(fmt.Sprintf("op%d", i), "symptoms", fmt.Sprintf("S%d", i), nil))
	}
	w := callRecordsSync(t, reqSyncPost(did, pid, mkSyncBody(t, ops...)), pid)
	if w.Code != http.StatusBadRequest || !hasErr(w, "batch_too_large") {
		t.Fatalf("want 400 batch_too_large, got %d %s", w.Code, w.Body.String())
	}
}

func TestRecordsSync_CreateOK(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	body := mkSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", map[string]any{"notes": "headache"}),
		mkCreateOp("op2", "allergies", "A1", map[string]any{"description": "pollen"}),
	)
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	if len(resp.Results) != 2 {
		t.Fatalf("results: want 2, got %d", len(resp.Results))
	}
	for _, r := range resp.Results {
		if r.Status != "ok" {
			t.Errorf("op %s: want ok, got %s", r.ClientOpID, r.Status)
		}
	}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, pid).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("symptoms count: want 1, got %d", cnt)
	}
}

func TestRecordsSync_IdempotentReplay(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	body := mkSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", nil),
		mkCreateOp("op2", "diagnoses", "D1", nil),
	)
	// первый прогон
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("first: want 200, got %d %s", w.Code, w.Body.String())
	}
	// повтор того же батча
	w = callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("replay: want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	for _, r := range resp.Results {
		if r.Status != "dup" {
			t.Errorf("replay op %s: want dup, got %s", r.ClientOpID, r.Status)
		}
	}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, pid).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("symptoms count after replay: want 1, got %d", cnt)
	}
}

func TestRecordsSync_PartialInvalid(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	body := mkSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", nil),
		map[string]any{ // unknown kind -> invalid
			"client_op_id": "op2",
			"kind":         "nonsense",
			"action":       "create",
			"payload":      map[string]any{"id": "X1"},
		},
	)
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	if len(resp.Results) != 2 {
		t.Fatalf("results: want 2, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != "ok" {
		t.Errorf("op1: want ok, got %s", resp.Results[0].Status)
	}
	if resp.Results[1].Status != "invalid" {
		t.Errorf("op2: want invalid, got %s", resp.Results[1].Status)
	}
}

func TestRecordsSync_ConflictLWW(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)

	// 1) create через sync
	createBody := mkSyncBody(t,
		mkCreateOp("op-create", "symptoms", "S1", map[string]any{"description": "v1"}))
	w := callRecordsSync(t, reqSyncPost(did, pid, createBody), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("create: want 200, got %d %s", w.Code, w.Body.String())
	}

	// 2) детерминированно «состарим» серверную запись, чтобы update прошёл LWW
	//    (create и update в одну секунду -> updated_at < now ложно -> conflict).
	if _, err := db.Exec(`UPDATE symptoms SET updated_at=? WHERE id=? AND patient_id=?`,
		"2020-01-01T00:00:00Z", "S1", pid); err != nil {
		t.Fatalf("age updated_at: %v", err)
	}

	// 3) update с тем же id — updated_at(2020) < now -> ok
	updBody := mkSyncBody(t, map[string]any{
		"client_op_id": "op-upd-1",
		"kind":         "symptoms",
		"action":       "update",
		"payload":      map[string]any{"id": "S1", "notes": "v2"},
	})
	w = callRecordsSync(t, reqSyncPost(did, pid, updBody), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("update1: want 200, got %d %s", w.Code, w.Body.String())
	}
	// второй update в тот же момент времени (now() может совпасть до секунды) — LWW
	// может дать conflict, если updated_at не строго меньше now. Для стабильности
	// делаем update с ЯВНО более старым now: подменяем updated_at записи в БД
	// на будущее, чтобы следующий update гарантированно был conflict.
	if _, err := db.Exec(`UPDATE symptoms SET updated_at=? WHERE id=? AND patient_id=?`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "S1", pid); err != nil {
		t.Fatalf("bump updated_at: %v", err)
	}
	w = callRecordsSync(t, reqSyncPost(did, pid, mkSyncBody(t, map[string]any{
		"client_op_id": "op-upd-2",
		"kind":         "symptoms",
		"action":       "update",
		"payload":      map[string]any{"id": "S1", "notes": "v3"},
	})), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("update2: want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "conflict" {
		t.Fatalf("want conflict, got %s", resp.Results[0].Status)
	}
	// серверная description не должна измениться на v3
	var desc string
	if err := db.QueryRow(`SELECT notes FROM symptoms WHERE id=? AND patient_id=?`,
		"S1", pid).Scan(&desc); err != nil {
		t.Fatalf("select desc: %v", err)
	}
	if desc == "v3" {
		t.Fatalf("conflict должен был сохранить серверную версию, а desc=v3")
	}
}

func TestRecordsSync_ConcurrentReplay(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	// :memory: без cache=shared — каждое соединение пула = своя БД.
	// Один conn сериализует доступ; UNIQUE-гонка всё равно проверяется (ok+dup).
	db.SetMaxOpenConns(1)
	body := mkSyncBody(t, mkCreateOp("op-conc", "symptoms", "S1", nil))

	var wg sync.WaitGroup
	codes := make([]int, 2)
	bodies := make([]string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
			codes[i] = w.Code
			bodies[i] = w.Body.String()
		}(i)
	}
	wg.Wait()

	for i, c := range codes {
		if c != http.StatusOK {
			t.Fatalf("goroutine %d: want 200, got %d body=%s", i, c, bodies[i])
		}
	}
	// Ровно одна запись в БД, статусы {ok, dup}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, pid).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("symptoms count: want 1, got %d", cnt)
	}
	// один из ответов содержит ok, другой dup
	combined := bodies[0] + bodies[1]
	if !bytes.Contains([]byte(combined), []byte(`"ok"`)) ||
		!bytes.Contains([]byte(combined), []byte(`"dup"`)) {
		t.Fatalf("want {ok,dup}, got %s", combined)
	}
}

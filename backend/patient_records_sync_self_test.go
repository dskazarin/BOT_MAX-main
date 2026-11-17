package main

// patient_records_sync_self_test.go — B30.17 3b: batch-sync self (patient).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func callSelfSync(t *testing.T, accID string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	r := reqRecords(accID, http.MethodPost, "/api/patient/records/sync", string(body))
	w := httptest.NewRecorder()
	handlePatientRecordsSyncSelf(w, r)
	return w
}

func setupSelfSyncEnv(t *testing.T) {
	t.Helper()
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-1", "ACC-1")
}

func selfSyncBody(t *testing.T, ops ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{"ops": ops})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func decodeSelfSync(t *testing.T, w *httptest.ResponseRecorder) syncResp {
	t.Helper()
	var r syncResp
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, w.Body.String())
	}
	return r
}

func TestSelfSync_MethodNotAllowed(t *testing.T) {
	setupSelfSyncEnv(t)
	r := reqRecords("ACC-1", http.MethodGet, "/api/patient/records/sync", "")
	w := httptest.NewRecorder()
	handlePatientRecordsSyncSelf(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", w.Code)
	}
}

func TestSelfSync_InvalidJSON(t *testing.T) {
	setupSelfSyncEnv(t)
	w := callSelfSync(t, "ACC-1", []byte("{bad"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestSelfSync_EmptyOps(t *testing.T) {
	setupSelfSyncEnv(t)
	w := callSelfSync(t, "ACC-1", []byte(`{"ops":[]}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d %s", w.Code, w.Body.String())
	}
}

func TestSelfSync_BatchTooLarge(t *testing.T) {
	setupSelfSyncEnv(t)
	ops := make([]map[string]any, 0, maxSyncOps+1)
	for i := 0; i <= maxSyncOps; i++ {
		ops = append(ops, mkCreateOp(fmt.Sprintf("op%d", i), "symptoms", fmt.Sprintf("S%d", i), nil))
	}
	w := callSelfSync(t, "ACC-1", selfSyncBody(t, ops...))
	if w.Code != http.StatusBadRequest || !hasErr(w, "batch_too_large") {
		t.Fatalf("want 400 batch_too_large, got %d %s", w.Code, w.Body.String())
	}
}

func TestSelfSync_CreateOK(t *testing.T) {
	setupSelfSyncEnv(t)
	body := selfSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", map[string]any{"notes": "headache"}),
		mkCreateOp("op2", "allergies", "A1", map[string]any{"notes": "pollen"}),
	)
	w := callSelfSync(t, "ACC-1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSelfSync(t, w)
	for _, r := range resp.Results {
		if r.Status != "ok" {
			t.Errorf("op %s: want ok, got %s", r.ClientOpID, r.Status)
		}
	}
	var cnt int
	if err := db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, "PAT-1").Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("symptoms count: want 1, got %d", cnt)
	}
}

func TestSelfSync_IdempotentReplay(t *testing.T) {
	setupSelfSyncEnv(t)
	body := selfSyncBody(t, mkCreateOp("op1", "symptoms", "S1", nil))
	if w := callSelfSync(t, "ACC-1", body); w.Code != http.StatusOK {
		t.Fatalf("first: want 200, got %d %s", w.Code, w.Body.String())
	}
	w := callSelfSync(t, "ACC-1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("replay: want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSelfSync(t, w)
	if resp.Results[0].Status != "dup" {
		t.Fatalf("replay: want dup, got %s", resp.Results[0].Status)
	}
	var cnt int
	db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, "PAT-1").Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("count after replay: want 1, got %d", cnt)
	}
}

func TestSelfSync_PartialInvalid(t *testing.T) {
	setupSelfSyncEnv(t)
	body := selfSyncBody(t,
		mkCreateOp("op1", "symptoms", "S1", nil),
		map[string]any{"client_op_id": "op2", "kind": "nonsense", "action": "create",
			"payload": map[string]any{"id": "X1"}},
	)
	w := callSelfSync(t, "ACC-1", body)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSelfSync(t, w)
	if resp.Results[0].Status != "ok" || resp.Results[1].Status != "invalid" {
		t.Fatalf("want ok/invalid, got %s/%s", resp.Results[0].Status, resp.Results[1].Status)
	}
}

func TestSelfSync_ConflictLWW(t *testing.T) {
	setupSelfSyncEnv(t)
	createBody := selfSyncBody(t, mkCreateOp("op-create", "symptoms", "S1", map[string]any{"notes": "v1"}))
	if w := callSelfSync(t, "ACC-1", createBody); w.Code != http.StatusOK {
		t.Fatalf("create: want 200, got %d %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`UPDATE symptoms SET updated_at=? WHERE id=? AND patient_id=?`,
		"2020-01-01T00:00:00Z", "S1", "PAT-1"); err != nil {
		t.Fatalf("age: %v", err)
	}
	w := callSelfSync(t, "ACC-1", selfSyncBody(t, map[string]any{
		"client_op_id": "op-upd-1", "kind": "symptoms", "action": "update",
		"payload": map[string]any{"id": "S1", "notes": "v2"}}))
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d %s", w.Code, w.Body.String())
	}
	if resp := decodeSelfSync(t, w); resp.Results[0].Status != "ok" {
		t.Fatalf("update: want ok, got %s", resp.Results[0].Status)
	}
	if _, err := db.Exec(`UPDATE symptoms SET updated_at=? WHERE id=? AND patient_id=?`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "S1", "PAT-1"); err != nil {
		t.Fatalf("bump: %v", err)
	}
	w = callSelfSync(t, "ACC-1", selfSyncBody(t, map[string]any{
		"client_op_id": "op-upd-2", "kind": "symptoms", "action": "update",
		"payload": map[string]any{"id": "S1", "notes": "v3"}}))
	if w.Code != http.StatusOK {
		t.Fatalf("update2: want 200, got %d %s", w.Code, w.Body.String())
	}
	if resp := decodeSelfSync(t, w); resp.Results[0].Status != "conflict" {
		t.Fatalf("want conflict, got %s", resp.Results[0].Status)
	}
}

func TestSelfSync_ConcurrentReplay(t *testing.T) {
	setupSelfSyncEnv(t)
	db.SetMaxOpenConns(1)
	body := selfSyncBody(t, mkCreateOp("op-conc", "symptoms", "S1", nil))

	var wg sync.WaitGroup
	codes := make([]int, 2)
	bodies := make([]string, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := callSelfSync(t, "ACC-1", body)
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
	var cnt int
	db.QueryRow(`SELECT COUNT(*) FROM symptoms WHERE patient_id=?`, "PAT-1").Scan(&cnt)
	if cnt != 1 {
		t.Fatalf("count: want 1, got %d", cnt)
	}
	combined := bodies[0] + bodies[1]
	if !bytes.Contains([]byte(combined), []byte(`"ok"`)) ||
		!bytes.Contains([]byte(combined), []byte(`"dup"`)) {
		t.Fatalf("want {ok,dup}, got %s", combined)
	}
}

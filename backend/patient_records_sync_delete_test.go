package main

// patient_records_sync_delete_test.go — B30.17 Шаг 3c: batch-sync delete (LWW soft-delete).

import (
	"net/http"
	"testing"
	"time"
)

func mkDeleteOp(clientOpID, kind, id string) map[string]any {
	return map[string]any{
		"client_op_id": clientOpID,
		"kind":         kind,
		"action":       "delete",
		"payload":      map[string]any{"id": id},
	}
}

func seedViaSync(t *testing.T, did, pid, clientOpID, kind, id string) {
	t.Helper()
	body := mkSyncBody(t, mkCreateOp(clientOpID, kind, id, map[string]any{"name": "x"}))
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("seed create: want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "ok" {
		t.Fatalf("seed create: want ok, got %s", resp.Results[0].Status)
	}
	// «Состарим» запись, чтобы последующий delete прошёл LWW (updated_at < now).
	// В проде между create и delete проходят секунды/минуты; в тесте — микросекунды,
	// и RFC3339-секундная точность даёт updated_at == now -> conflict.
	if _, err := db.Exec(`UPDATE `+kind+` SET updated_at=? WHERE id=? AND patient_id=?`,
		"2020-01-01T00:00:00Z", id, pid); err != nil {
		t.Fatalf("age updated_at: %v", err)
	}
}

func TestRecordsSyncDelete_OK(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	seedViaSync(t, did, pid, "op-c1", "symptoms", "S1")

	body := mkSyncBody(t, mkDeleteOp("op-d1", "symptoms", "S1"))
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d %s", w.Code, w.Body.String())
	}
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "ok" {
		t.Fatalf("want ok, got %s", resp.Results[0].Status)
	}

	var deletedAt *string
	if err := db.QueryRow(`SELECT deleted_at FROM symptoms WHERE id=? AND patient_id=?`,
		"S1", pid).Scan(&deletedAt); err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatalf("deleted_at должен быть NOT NULL после delete")
	}
}

func TestRecordsSyncDelete_ReplayConflict(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	seedViaSync(t, did, pid, "op-c1", "symptoms", "S1")

	w := callRecordsSync(t, reqSyncPost(did, pid,
		mkSyncBody(t, mkDeleteOp("op-d1", "symptoms", "S1"))), pid)
	if resp := decodeSyncResp(t, w); resp.Results[0].Status != "ok" {
		t.Fatalf("first delete: want ok, got %s", resp.Results[0].Status)
	}
	w = callRecordsSync(t, reqSyncPost(did, pid,
		mkSyncBody(t, mkDeleteOp("op-d2", "symptoms", "S1"))), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "conflict" {
		t.Fatalf("replay delete: want conflict, got %s", resp.Results[0].Status)
	}
}

func TestRecordsSyncDelete_LWWConflict(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	seedViaSync(t, did, pid, "op-c1", "symptoms", "S1")

	if _, err := db.Exec(`UPDATE symptoms SET updated_at=? WHERE id=? AND patient_id=?`,
		time.Now().UTC().Add(time.Hour).Format(time.RFC3339), "S1", pid); err != nil {
		t.Fatalf("bump updated_at: %v", err)
	}

	w := callRecordsSync(t, reqSyncPost(did, pid,
		mkSyncBody(t, mkDeleteOp("op-d1", "symptoms", "S1"))), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "conflict" {
		t.Fatalf("want conflict, got %s", resp.Results[0].Status)
	}

	var deletedAt *string
	if err := db.QueryRow(`SELECT deleted_at FROM symptoms WHERE id=? AND patient_id=?`,
		"S1", pid).Scan(&deletedAt); err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt != nil {
		t.Fatalf("deleted_at должен остаться NULL после LWW-конфликта")
	}
}

func TestRecordsSyncDelete_InvalidNoID(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	body := mkSyncBody(t, map[string]any{
		"client_op_id": "op-d1",
		"kind":         "symptoms",
		"action":       "delete",
		"payload":      map[string]any{},
	})
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "invalid" {
		t.Fatalf("want invalid, got %s", resp.Results[0].Status)
	}
}

func TestRecordsSyncDelete_NotFound(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	w := callRecordsSync(t, reqSyncPost(did, pid,
		mkSyncBody(t, mkDeleteOp("op-d1", "symptoms", "NOPE"))), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "conflict" {
		t.Fatalf("delete not found: want conflict, got %s", resp.Results[0].Status)
	}
}

func TestRecordsSyncDelete_NoResurrect(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	seedViaSync(t, did, pid, "op-c1", "symptoms", "S1")

	w := callRecordsSync(t, reqSyncPost(did, pid,
		mkSyncBody(t, mkDeleteOp("op-d1", "symptoms", "S1"))), pid)
	if resp := decodeSyncResp(t, w); resp.Results[0].Status != "ok" {
		t.Fatalf("delete: want ok, got %s", resp.Results[0].Status)
	}

	w = callRecordsSync(t, reqSyncPost(did, pid, mkSyncBody(t, map[string]any{
		"client_op_id": "op-u1",
		"kind":         "symptoms",
		"action":       "update",
		"payload":      map[string]any{"id": "S1", "notes": "v2"},
	})), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "conflict" {
		t.Fatalf("update после delete: want conflict, got %s", resp.Results[0].Status)
	}

	var deletedAt *string
	if err := db.QueryRow(`SELECT deleted_at FROM symptoms WHERE id=? AND patient_id=?`,
		"S1", pid).Scan(&deletedAt); err != nil {
		t.Fatalf("select deleted_at: %v", err)
	}
	if deletedAt == nil {
		t.Fatalf("deleted_at должен остаться NOT NULL после replay update")
	}
}

func TestRecordsSyncDelete_MixedBatch(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	seedViaSync(t, did, pid, "op-c-seed", "symptoms", "S2")

	body := mkSyncBody(t,
		mkCreateOp("op-c1", "symptoms", "S1", map[string]any{"name": "x"}),
		mkDeleteOp("op-d2", "symptoms", "S2"),
		mkDeleteOp("op-d3", "symptoms", "S2"),
	)
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	resp := decodeSyncResp(t, w)
	want := []string{"ok", "ok", "conflict"}
	if len(resp.Results) != len(want) {
		t.Fatalf("want %d results, got %d", len(want), len(resp.Results))
	}
	for i, w := range want {
		if resp.Results[i].Status != w {
			t.Fatalf("result[%d]: want %s, got %s", i, w, resp.Results[i].Status)
		}
	}
}

func TestRecordsSync_CompatNoAction(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	body := mkSyncBody(t, map[string]any{
		"client_op_id": "op-x1",
		"kind":         "symptoms",
		"payload":      map[string]any{"id": "S1", "name": "x"},
	})
	w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
	resp := decodeSyncResp(t, w)
	if resp.Results[0].Status != "invalid" {
		t.Fatalf("no-action: want invalid, got %s", resp.Results[0].Status)
	}
}

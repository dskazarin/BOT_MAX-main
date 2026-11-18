package main

// sync_plan_test.go — B30.17 (B): проверка индексов и идемпотентности sync-пути.
//
// Ловит регресс: partial UNIQUE (patient_id, client_op_id) должен быть на ВСЕХ
// таблицах видов, иначе ON CONFLICT в insertSyncCreate не срабатывает (create
// через sync ломается / теряет идемпотентность), а replay-SELECT идёт seq scan.

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"
)

var syncPlanKinds = []string{
	"symptoms", "allergies", "vaccinations", "diagnoses",
	"constant_medications", "course_medications", "files",
}

// TestSyncCreate_ClientOpIdempotent_AllKinds: create + повтор того же op_id
// на КАЖДОМ виде должен дать ok -> dup (идемпотентность через ON CONFLICT).
func TestSyncCreate_ClientOpIdempotent_AllKinds(t *testing.T) {
	did, pid := setupSyncWriteEnv(t)
	// setupSyncWriteEnv даёт write-consent только на 4 «старых» вида;
	// для медикментов и файлов выдаём локально, не меняя общий хелпер.
	seedApprovedWriteConsents(t, pid, did,
		"constant_medications", "course_medications", "files")
	for _, kind := range syncPlanKinds {
		opID := "op-idem-" + kind
		id := "IDEM-" + kind
		body := mkSyncBody(t, mkCreateOp(opID, kind, id, map[string]any{"name": "x"}))
		w := callRecordsSync(t, reqSyncPost(did, pid, body), pid)
		if w.Code != http.StatusOK {
			t.Fatalf("%s create: want 200, got %d %s", kind, w.Code, w.Body.String())
		}
		resp := decodeSyncResp(t, w)
		if got := resp.Results[0].Status; got != "ok" {
			t.Fatalf("%s create: want ok, got %s", kind, got)
		}

		// Повтор того же op_id — должен быть dup, не invalid и не второй insert.
		w = callRecordsSync(t, reqSyncPost(did, pid, body), pid)
		if w.Code != http.StatusOK {
			t.Fatalf("%s replay: want 200, got %d %s", kind, w.Code, w.Body.String())
		}
		resp = decodeSyncResp(t, w)
		if got := resp.Results[0].Status; got != "dup" {
			t.Fatalf("%s replay: want dup (idempotent), got %s", kind, got)
		}
	}
}

// TestSyncPlan_ClientOpReplayUsesIndex: replay-SELECT по (patient_id, client_op_id)
// должен опираться на ux_<kind>_client_op, а не сканировать таблицу.
func TestSyncPlan_ClientOpReplayUsesIndex(t *testing.T) {
	_, _ = setupSyncWriteEnv(t)
	for _, kind := range syncPlanKinds {
		q := "SELECT id FROM " + kind + " WHERE patient_id=? AND client_op_id=?"
		plan := explainPlan(t, db, q, "P", "op")
		want := "ux_" + kind + "_client_op"
		if !strings.Contains(plan, want) {
			t.Fatalf("%s: plan не использует %s: %s", kind, want, plan)
		}
		if strings.Contains(plan, "SCAN "+kind) {
			t.Fatalf("%s: seq scan вместо индекса: %s", kind, plan)
		}
	}
}

// TestSyncPlan_DeleteUsesIndex: LWW-delete по (id, patient_id, deleted_at, updated_at)
// должен опираться на PK(id) + idx_<kind>_patient, без полного скана.
func TestSyncPlan_DeleteUsesIndex(t *testing.T) {
	_, _ = setupSyncWriteEnv(t)
	kind := "symptoms"
	q := "UPDATE " + kind + " SET deleted_at=?, updated_at=? " +
		"WHERE id=? AND patient_id=? AND deleted_at IS NULL AND updated_at < ?"
	plan := explainPlan(t, db, q, "now", "now", "id1", "P", "now")
	if strings.Contains(plan, "SCAN "+kind) {
		t.Fatalf("%s delete: seq scan: %s", kind, plan)
	}
}

var _ = sql.ErrNoRows // keep database/sql imported even если ниже не используется

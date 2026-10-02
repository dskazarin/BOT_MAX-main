package main

// retention_test.go — подпатч 6.34 (B25.1): тесты retention status + healthcheck.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// resetRetentionStatus — сброс глобального статуса (для изоляции тестов).
func resetRetentionStatus() {
	retentionStatusMu.Lock()
	retentionStatusVal = RetentionStatus{}
	retentionStatusMu.Unlock()
}

// seedOldRefresh — вставляет протухшую refresh_tokens запись и возвращает её jti.
func seedOldRefresh(t *testing.T, db interface {
	Exec(string, ...any) (interface{ LastInsertId() (int64, error) }, error)
}, jti, doctorID string, daysAgo int) {
	// helper не используется — реализовано в других тестах
}

// TestRetentionStatus_Empty — до первого прогона статус пустой, healthy=false.
func TestRetentionStatus_Empty(t *testing.T) {
	resetRetentionStatus()
	if retentionHealthy(30, 1, time.Hour) {
		t.Fatalf("healthy должен быть false до первого прогона")
	}
	st := getRetentionStatus()
	if st.HasRun {
		t.Fatalf("HasRun должен быть false")
	}
}

// TestRetentionStatus_Disabled — при обоих days<=0 healthy=true (фича выключена).
func TestRetentionStatus_Disabled(t *testing.T) {
	resetRetentionStatus()
	if !retentionHealthy(0, 0, time.Hour) {
		t.Fatalf("healthy должен быть true при выключенной фиче")
	}
}

// TestRetentionStatus_UpdatedOnRun — runRetention обновляет статус.
func TestRetentionStatus_UpdatedOnRun(t *testing.T) {
	resetRetentionStatus()
	testDB := newTestDB(t)
	seedDoctorForRefresh(t, testDB, "D1")

	// Старая audit-запись.
	oldTs := time.Now().AddDate(0, 0, -60).Format("2006-01-02 15:04:05")
	_, err := testDB.Exec(
		`INSERT INTO audit_log (ts, doctor_id, action, entity, entity_id, ip, detail) VALUES (?, 'X', 'test', '', '', '', '')`,
		oldTs,
	)
	if err != nil {
		t.Fatalf("insert audit: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Запускаем runner в горутине, ждём первый прогон.
	go runRetention(ctx, testDB, 30, 1, time.Hour)

	// Ждём появления HasRun (первый прогон — сразу).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if getRetentionStatus().HasRun {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	st := getRetentionStatus()
	if !st.HasRun {
		t.Fatalf("HasRun не выставился за 2с")
	}
	if st.AuditDeleted != 1 {
		t.Errorf("AuditDeleted: got %d, want 1", st.AuditDeleted)
	}
	if st.LastError != "" {
		t.Errorf("LastError: got %q, want empty", st.LastError)
	}
	if st.LastRunAt.IsZero() {
		t.Errorf("LastRunAt пустой")
	}

	// healthy=true (свежий прогон).
	if !retentionHealthy(30, 1, time.Hour) {
		t.Errorf("healthy должен быть true после свежего прогона")
	}
}

// TestRetentionStatus_UnhealthyAfterInterval — last_run_at старше interval*2 → healthy=false.
func TestRetentionStatus_UnhealthyAfterInterval(t *testing.T) {
	resetRetentionStatus()
	retentionStatusMu.Lock()
	retentionStatusVal.HasRun = true
	retentionStatusVal.LastRunAt = time.Now().Add(-3 * time.Hour)
	retentionStatusMu.Unlock()

	if retentionHealthy(30, 1, time.Hour) {
		t.Fatalf("healthy должен быть false при last_run_at старше 2*interval")
	}
}

// TestRetentionStatus_ErrorClearedOnSuccess — last_error сбрасывается при успехе.
func TestRetentionStatus_ErrorClearedOnSuccess(t *testing.T) {
	resetRetentionStatus()
	updateRetentionStatusError(errors.New("test-error"))
	if getRetentionStatus().LastError == "" {
		t.Fatalf("LastError должен быть установлен")
	}
	updateRetentionStatusSuccess(0, 0)
	if getRetentionStatus().LastError != "" {
		t.Fatalf("LastError должен сбрасываться при успехе")
	}
}

// TestHealth_NoRetentionSection — оба флага 0 → секции retention нет.
func TestHealth_NoRetentionSection(t *testing.T) {
	resetRetentionStatus()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{
		Name:                 "dev",
		AuditRetentionDays:   0,
		RefreshRetentionDays: 0,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("status: got %v", resp["status"])
	}
	if _, ok := resp["retention"]; ok {
		t.Fatalf("секция retention не должна присутствовать при выключенной фиче")
	}
}

// TestHealth_RetentionSection — флаг > 0 → секция есть.
func TestHealth_RetentionSection(t *testing.T) {
	resetRetentionStatus()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{
		Name:                 "dev",
		AuditRetentionDays:   30,
		RefreshRetentionDays: 1,
		AuditCleanupInterval: time.Hour,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	w := httptest.NewRecorder()
	handleHealth(w, req)

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	sec, ok := resp["retention"].(map[string]any)
	if !ok {
		t.Fatalf("секция retention отсутствует")
	}
	if sec["enabled"] != true {
		t.Fatalf("enabled: got %v", sec["enabled"])
	}
	if sec["healthy"] != false {
		t.Fatalf("healthy: got %v (want false — ещё не было прогона)", sec["healthy"])
	}
	if sec["audit_days"] != float64(30) {
		t.Fatalf("audit_days: got %v", sec["audit_days"])
	}
}

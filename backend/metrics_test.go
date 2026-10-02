package main

// metrics_test.go — подпатч 6.35 (B26.1): тесты /api/metrics.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// reqMetrics — GET /api/metrics (метод можно задать).
func reqMetrics(method string) *http.Request {
	return httptest.NewRequest(method, "/api/metrics", nil)
}

// withMetricsEnv — временно ставит env для тестов metrics.
func withMetricsEnv(t *testing.T, auditDays, refreshDays int, interval time.Duration) {
	t.Helper()
	prev := currentEnv
	t.Cleanup(func() { currentEnv = prev })
	currentEnv = &Env{
		Name:                 "dev",
		AuditRetentionDays:   auditDays,
		RefreshRetentionDays: refreshDays,
		AuditCleanupInterval: interval,
	}
}

// TestMetrics_Disabled — retention выключен → enabled 0, healthy 0.
func TestMetrics_Disabled(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 0, 0, time.Hour)

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodGet))

	if w.Code != http.StatusOK {
		t.Fatalf("code: got %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "botmax_retention_enabled 0") {
		t.Errorf("ожидали enabled 0, got:\n%s", body)
	}
	if !strings.Contains(body, "botmax_retention_healthy 0") {
		t.Errorf("ожидали healthy 0, got:\n%s", body)
	}
	if !strings.Contains(body, `botmax_retention_last_run_timestamp_seconds 0`) {
		t.Errorf("ожидали last_run 0, got:\n%s", body)
	}
}

// TestMetrics_EnabledNotRun — retention включён, но ещё не запускался.
func TestMetrics_EnabledNotRun(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 30, 1, time.Hour)

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodGet))

	body := w.Body.String()
	if !strings.Contains(body, "botmax_retention_enabled 1") {
		t.Errorf("ожидали enabled 1, got:\n%s", body)
	}
	if !strings.Contains(body, "botmax_retention_healthy 0") {
		t.Errorf("ожидали healthy 0 (ещё не запускался), got:\n%s", body)
	}
	if !strings.Contains(body, "botmax_retention_last_run_timestamp_seconds 0") {
		t.Errorf("ожидали last_run 0, got:\n%s", body)
	}
}

// TestMetrics_AfterRun — после успешного прогона.
func TestMetrics_AfterRun(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 30, 1, time.Hour)

	updateRetentionStatusSuccess(5, 3)

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodGet))

	body := w.Body.String()
	if !strings.Contains(body, "botmax_retention_healthy 1") {
		t.Errorf("ожидали healthy 1, got:\n%s", body)
	}
	if !strings.Contains(body, `botmax_retention_deleted_total{table="audit"} 5`) {
		t.Errorf("ожидали audit 5, got:\n%s", body)
	}
	if !strings.Contains(body, `botmax_retention_deleted_total{table="refresh"} 3`) {
		t.Errorf("ожидали refresh 3, got:\n%s", body)
	}
	if !strings.Contains(body, "botmax_retention_last_error 0") {
		t.Errorf("ожидали last_error 0, got:\n%s", body)
	}
	// last_run_timestamp_seconds — ненулевой
	if strings.Contains(body, "botmax_retention_last_run_timestamp_seconds 0\n") {
		t.Errorf("ожидали ненулевой last_run, got:\n%s", body)
	}
}

// TestMetrics_Error — ошибка в последнем прогоне → last_error 1, healthy 0.
func TestMetrics_Error(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 30, 1, time.Hour)

	updateRetentionStatusError(errMetricsTest{})

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodGet))

	body := w.Body.String()
	if !strings.Contains(body, "botmax_retention_last_error 1") {
		t.Errorf("ожидали last_error 1, got:\n%s", body)
	}
	if !strings.Contains(body, "botmax_retention_healthy 0") {
		t.Errorf("ожидали healthy 0, got:\n%s", body)
	}
}

// errMetricsTest — простая реализация error для теста.
type errMetricsTest struct{}

func (errMetricsTest) Error() string { return "test-error" }

// TestMetrics_MethodNotAllowed — POST → 405.
func TestMetrics_MethodNotAllowed(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 30, 1, time.Hour)

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodPost))

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: got %d, want 405", w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("Allow: got %q, want GET", got)
	}
}

// TestMetrics_ContentType — Prometheus text 0.0.4.
func TestMetrics_ContentType(t *testing.T) {
	resetRetentionStatus()
	withMetricsEnv(t, 30, 1, time.Hour)

	w := httptest.NewRecorder()
	handleMetrics(w, reqMetrics(http.MethodGet))

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") || !strings.Contains(ct, "version=0.0.4") {
		t.Fatalf("Content-Type: got %q, want 'text/plain; version=0.0.4; ...'", ct)
	}
}

package main

// metrics.go — подпатч 6.35 (B26.1): Prometheus-совместимый /api/metrics.
//
// Формат: text exposition format 0.0.4 — генерируется вручную,
// без клиента prometheus/client_golang (лишняя зависимость для проекта).
//
// Endpoint публичный (как /api/health): Prometheus scrape'ит без auth
// по внутренней сети. Добавлен в isAuthWhitelist в auth.go.
//
// Метрики retention:
//   botmax_retention_enabled                     gauge   1|0
//   botmax_retention_healthy                     gauge   1|0
//   botmax_retention_last_run_timestamp_seconds  gauge   unix-время, 0 если не было
//   botmax_retention_deleted_total{table=...}   counter накопительно (последний успех)
//   botmax_retention_last_error                  gauge   1|0

import (
	"fmt"
	"net/http"
)

// registerMetricsRoutes — регистрирует GET /api/metrics.
func registerMetricsRoutes() {
	http.HandleFunc("/api/metrics", handleMetrics)
}

// handleMetrics — Prometheus text exposition format 0.0.4.
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	enabled := currentEnv != nil &&
		(currentEnv.AuditRetentionDays > 0 || currentEnv.RefreshRetentionDays > 0)

	st := getRetentionStatus()

	var enabledVal, healthyVal, lastRun float64
	if enabled {
		enabledVal = 1
		if retentionHealthy(
			currentEnv.AuditRetentionDays,
			currentEnv.RefreshRetentionDays,
			currentEnv.AuditCleanupInterval,
		) {
			healthyVal = 1
		}
		if !st.LastRunAt.IsZero() {
			lastRun = float64(st.LastRunAt.Unix())
		}
	}

	var errVal float64
	if st.LastError != "" {
		errVal = 1
	}

	// --- retention_enabled ---
	fmt.Fprintln(w, "# HELP botmax_retention_enabled 1 if retention is enabled (any *_RETENTION_DAYS > 0), 0 otherwise.")
	fmt.Fprintln(w, "# TYPE botmax_retention_enabled gauge")
	fmt.Fprintf(w, "botmax_retention_enabled %g\n", enabledVal)

	// --- retention_healthy ---
	fmt.Fprintln(w, "# HELP botmax_retention_healthy 1 if retention is enabled, has run recently and has no error; 0 otherwise.")
	fmt.Fprintln(w, "# TYPE botmax_retention_healthy gauge")
	fmt.Fprintf(w, "botmax_retention_healthy %g\n", healthyVal)

	// --- retention_last_run_timestamp_seconds ---
	fmt.Fprintln(w, "# HELP botmax_retention_last_run_timestamp_seconds Unix time of the last successful retention run. 0 if never ran.")
	fmt.Fprintln(w, "# TYPE botmax_retention_last_run_timestamp_seconds gauge")
	fmt.Fprintf(w, "botmax_retention_last_run_timestamp_seconds %g\n", lastRun)

	// --- retention_deleted_total ---
	fmt.Fprintln(w, "# HELP botmax_retention_deleted_total Rows deleted by retention on the last successful run, by table.")
	fmt.Fprintln(w, "# TYPE botmax_retention_deleted_total counter")
	fmt.Fprintf(w, "botmax_retention_deleted_total{table=\"audit\"} %d\n", st.AuditDeleted)
	fmt.Fprintf(w, "botmax_retention_deleted_total{table=\"refresh\"} %d\n", st.RefreshDeleted)

	// --- retention_last_error ---
	fmt.Fprintln(w, "# HELP botmax_retention_last_error 1 if the last retention run failed, 0 otherwise.")
	fmt.Fprintln(w, "# TYPE botmax_retention_last_error gauge")
	fmt.Fprintf(w, "botmax_retention_last_error %g\n", errVal)
}

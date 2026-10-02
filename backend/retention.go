package main

// retention.go — подпатч 6.34 (B25.1): retention status для healthcheck.
//
// Сюда перенесены из audit.go:
//   - runRetention       (общий runner: audit + refresh)
//   - runAuditRetention  (обёртка для обратной совместимости B19)
//
// Добавлено:
//   - RetentionStatus    (последний прогон, счётчики, ошибка)
//   - retentionStatusVal (глобальное состояние, обновляется в runOnce)
//   - getRetentionStatus (snapshot для /api/health)
//   - retentionHealthy   (проверка: last_run_at не старше interval*2)

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"
)

// ---------------------------------------------------------------
// Статус retention (для /api/health)
// ---------------------------------------------------------------

// RetentionStatus — снимок последнего прогона retention.
// Защищён mutex'ом, читается через getRetentionStatus().
type RetentionStatus struct {
	LastRunAt      time.Time // время последнего успешного прогона
	LastError      string    // текст последней ошибки (сбрасывается при успехе)
	AuditDeleted   int64     // deleted на последнем успехе
	RefreshDeleted int64     // deleted на последнем успехе
	HasRun         bool      // true после хотя бы одного прогона (в т.ч. с ошибкой)
}

var (
	retentionStatusMu  sync.Mutex
	retentionStatusVal RetentionStatus
)

// updateRetentionStatusSuccess вызывается после успешного runOnce.
func updateRetentionStatusSuccess(auditDeleted, refreshDeleted int64) {
	retentionStatusMu.Lock()
	defer retentionStatusMu.Unlock()
	retentionStatusVal.LastRunAt = time.Now()
	retentionStatusVal.LastError = ""
	retentionStatusVal.AuditDeleted = auditDeleted
	retentionStatusVal.RefreshDeleted = refreshDeleted
	retentionStatusVal.HasRun = true
}

// updateRetentionStatusError вызывается при ошибке в runOnce.
// LastRunAt НЕ обновляем: чтобы healthy стал false, если ошибки идут подряд.
func updateRetentionStatusError(err error) {
	retentionStatusMu.Lock()
	defer retentionStatusMu.Unlock()
	if err != nil {
		retentionStatusVal.LastError = err.Error()
	}
	retentionStatusVal.HasRun = true
}

// getRetentionStatus возвращает копию статуса.
func getRetentionStatus() RetentionStatus {
	retentionStatusMu.Lock()
	defer retentionStatusMu.Unlock()
	return retentionStatusVal
}

// retentionHealthy — true, если retention «здоров»:
//   - фича выключена (оба days<=0) → не наша забота, возвращаем true (нет сигнала unhealthy);
//   - не было ни одного прогона → false (runner не стартовал?);
//   - last_run_at старше interval*2 → false (что-то застряло);
//   - LastError не пуст → false.
func retentionHealthy(auditDays, refreshDays int, interval time.Duration) bool {
	if auditDays <= 0 && refreshDays <= 0 {
		return true
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	st := getRetentionStatus()
	if !st.HasRun {
		return false
	}
	if st.LastError != "" {
		return false
	}
	if st.LastRunAt.IsZero() {
		return false
	}
	return time.Since(st.LastRunAt) <= interval*2
}

// ---------------------------------------------------------------
// runRetention
// ---------------------------------------------------------------

// runRetention — общий runner retention'а (B19 + B23 + B25.1).
// В одной горутине чистит audit_log и refresh_tokens.
// Каждый из *Days-параметров независим: 0 → соответствующий cleanup пропускается.
//
// После каждого прогона обновляет retentionStatusVal:
//   - успех → updateRetentionStatusSuccess
//   - ошибка хотя бы одной очистки → updateRetentionStatusError
func runRetention(ctx context.Context, db *sql.DB, auditDays, refreshDays int, interval time.Duration) {
	if auditDays <= 0 && refreshDays <= 0 {
		slog.Info("retention: выключен (auditDays<=0 && refreshDays<=0)",
			"auditDays", auditDays, "refreshDays", refreshDays)
		return
	}
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	slog.Info("retention: запущен",
		"auditDays", auditDays, "refreshDays", refreshDays, "interval", interval.String())

	runOnce := func() {
		var (
			auditN   int64
			refreshN int64
			firstErr error
		)
		if auditDays > 0 {
			n, err := auditCleanup(db, auditDays)
			if err != nil {
				slog.Warn("retention: audit cleanup", "err", err)
				if firstErr == nil {
					firstErr = err
				}
			} else {
				slog.Info("retention: audit cleanup", "deleted", n, "days", auditDays)
				auditN = n
			}
		}
		if refreshDays > 0 {
			n, err := refreshCleanup(db, refreshDays)
			if err != nil {
				slog.Warn("retention: refresh cleanup", "err", err)
				if firstErr == nil {
					firstErr = err
				}
			} else {
				slog.Info("retention: refresh cleanup", "deleted", n, "days", refreshDays)
				refreshN = n
			}
		}
		if firstErr != nil {
			updateRetentionStatusError(firstErr)
		} else {
			updateRetentionStatusSuccess(auditN, refreshN)
		}
	}

	// Первый прогон — сразу при старте.
	runOnce()

	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("retention: остановлен", "reason", ctx.Err().Error())
			return
		case <-t.C:
			runOnce()
		}
	}
}

// runAuditRetention — обёртка над runRetention для обратной совместимости (B19).
// audit-only: refreshDays=0.
func runAuditRetention(ctx context.Context, db *sql.DB, days int, interval time.Duration) {
	runRetention(ctx, db, days, 0, interval)
}

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// GCStorage — физическая чистка файлов в FS-хранилище, у которых
// запись в files мягко удалена (deleted_at IS NOT NULL) и
// deleted_at < now - olderThan.
//
// Для blobStorage — no-op: данные в files.data, история soft-delete
// не рвётся (её чистит только retention строк, отдельная история).
// Для fsStorage — вызывается storage.Delete(pid, id), который
// идемпотентен (нет файла — nil).
//
// deleted_at хранится как TEXT в формате RFC3339Nano (как пишет
// blobStorage.Delete и fsStorage через insert/update). Поэтому cutoff
// передаём строкой в том же формате — SQLite сравнивает лексикографически.
//
// Возвращает количество физически удалённых файлов.
// Ошибки по отдельным записям агрегируются; при ctx.Done() цикл прерывается.
func GCStorage(ctx context.Context, storage FileStorage, db *sql.DB, olderThan time.Duration) (int, error) {
	if _, ok := storage.(*fsStorage); !ok {
		return 0, nil // blobStorage и s3 (позже) — no-op.
	}
	if db == nil {
		return 0, errors.New("GCStorage: db == nil")
	}
	if olderThan <= 0 {
		return 0, fmt.Errorf("GCStorage: olderThan должен быть > 0, got %v", olderThan)
	}

	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339Nano)
	rows, err := db.QueryContext(ctx,
		`SELECT patient_id, id FROM files
		 WHERE storage_kind = 'fs'
		   AND deleted_at IS NOT NULL
		   AND deleted_at != ''
		   AND deleted_at < ?`,
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("GCStorage: select: %w", err)
	}
	defer rows.Close()

	var (
		deleted int
		errs    []error
	)
	for rows.Next() {
		select {
		case <-ctx.Done():
			return deleted, errors.Join(append(errs, ctx.Err())...)
		default:
		}

		var pid, id string
		if err := rows.Scan(&pid, &id); err != nil {
			errs = append(errs, fmt.Errorf("scan: %w", err))
			continue
		}
		if err := storage.Delete(pid, id); err != nil {
			errs = append(errs, fmt.Errorf("delete pid=%s id=%s: %w", pid, id, err))
			continue
		}
		deleted++
		slog.Info("GCStorage: файл удалён", "pid", pid, "id", id)
	}
	if err := rows.Err(); err != nil {
		errs = append(errs, fmt.Errorf("rows.Err: %w", err))
	}
	return deleted, errors.Join(errs...)
}

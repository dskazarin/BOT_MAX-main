package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"
)

// migrateToS3 переносит все blob/fs-файлы в S3. Возвращает число перенесённых.
// fsSrc может быть nil — тогда fs-строки пропускаются с предупреждением.
func migrateToS3(db *sql.DB, fsSrc *fsStorage, s3 *s3Storage) (int, error) {
	if db == nil || s3 == nil {
		return 0, fmt.Errorf("migrateToS3: nil db/s3")
	}
	rows, err := db.Query(`
		SELECT id, patient_id, filename, mime, storage_kind, storage_key, data
		FROM files
		WHERE deleted_at IS NULL
		  AND storage_kind IN ('blob','fs')
		  AND ((data IS NOT NULL AND length(data) > 0) OR storage_key IS NOT NULL)
		ORDER BY patient_id, id
	`)
	if err != nil {
		return 0, fmt.Errorf("migrateToS3: select: %w", err)
	}
	defer rows.Close()

	type item struct {
		id, pid, filename, mime, kind string
		key                           sql.NullString
		data                          []byte
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.pid, &it.filename, &it.mime, &it.kind, &it.key, &it.data); err != nil {
			return 0, fmt.Errorf("migrateToS3: scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("migrateToS3: rows: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	moved := 0
	for _, it := range items {
		var body []byte
		switch it.kind {
		case string(StorageKindBlob):
			body = it.data
		case string(StorageKindFS):
			if fsSrc == nil {
				slog.Warn("migrateToS3: fs-файл пропущен (fs не инициализирован)", "id", it.id)
				continue
			}
			_, _, data, ok, err := fsSrc.Get(it.pid, it.id)
			if err != nil {
				return moved, fmt.Errorf("migrateToS3: fs.Get %s: %w", it.id, err)
			}
			if !ok || len(data) == 0 {
				slog.Warn("migrateToS3: fs-файл пуст/отсутствует, пропуск", "id", it.id)
				continue
			}
			body = data
		default:
			continue
		}
		if err := s3.Put(it.pid, it.id, it.filename, it.mime, body); err != nil {
			return moved, fmt.Errorf("migrateToS3: s3.Put %s: %w", it.id, err)
		}
		res, err := db.Exec(`
			UPDATE files
			SET storage_kind='s3', storage_key=?, data=NULL, size_bytes=?, updated_at=?
			WHERE id=? AND storage_kind IN ('blob','fs')`,
			s3Key(it.pid, it.id), len(body), now, it.id,
		)
		if err != nil {
			return moved, fmt.Errorf("migrateToS3: update %s: %w", it.id, err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			slog.Warn("migrateToS3: update 0 rows (гонка)", "id", it.id)
			continue
		}
		if it.kind == string(StorageKindFS) && fsSrc != nil {
			_ = fsSrc.DeleteBlob(it.pid, it.id)
		}
		moved++
	}
	return moved, nil
}

package main

// storage_migrate.go — B30.10 §6.59 step4b: миграция BLOB → FS.
//
// Вызывается из main.go при старте, если Env.StorageKind=fs && Env.StorageMigrate.
// Поведение:
//   - SELECT строки files, которые ещё blob, живые, с непустым data;
//   - по каждой: fs.PutBlob(pid, id, data), затем UPDATE files
//     SET storage_kind='fs', storage_key=<relpath>, data=NULL, size_bytes=len(data),
//         updated_at=now
//     WHERE id=? AND storage_kind='blob'
//     (условие storage_kind='blob' в WHERE — защита от гонки с параллельным
//      переключением; при 0 rows — файл остался «сиротой» на диске, повторим
//      при следующем запуске: идемпотентность по WHERE storage_kind='blob').
//
// Идемпотентность:
//   - повторный вызов вернёт moved=0 (все уже fs);
//   - deleted_at IS NOT NULL — пропускаются (retention сам разберётся);
//   - data IS NULL / length=0 — пропускаются (нечего мигрировать);
//   - storage_kind='fs' — пропускаются.
//
// honest-ошибки: не маскируем в «не найдено».

import (
	"database/sql"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// migrateBlobToFS переносит все blob-файлы в FS. Возвращает число перенесённых.
func migrateBlobToFS(db *sql.DB, fs *fsStorage) (int, error) {
	if db == nil || fs == nil {
		return 0, fmt.Errorf("migrateBlobToFS: nil db/fs")
	}
	rows, err := db.Query(`
                SELECT id, patient_id, data
                FROM files
                WHERE deleted_at IS NULL
                  AND storage_kind = 'blob'
                  AND data IS NOT NULL
                  AND length(data) > 0
                ORDER BY patient_id, id
        `)
	if err != nil {
		return 0, fmt.Errorf("migrateBlobToFS: select: %w", err)
	}
	type item struct {
		id, pid string
		data    []byte
	}
	var items []item
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.pid, &it.data); err != nil {
			rows.Close()
			return 0, fmt.Errorf("migrateBlobToFS: scan: %w", err)
		}
		items = append(items, it)
	}
	if err := rows.Close(); err != nil {
		return 0, fmt.Errorf("migrateBlobToFS: close rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("migrateBlobToFS: rows.Err: %w", err)
	}

	moved := 0
	for _, it := range items {
		if err := fs.PutBlob(it.pid, it.id, it.data); err != nil {
			return moved, fmt.Errorf("migrateBlobToFS: fs.PutBlob(%s/%s): %w", it.pid, it.id, err)
		}
		// storage_key = относительный путь внутри fs.dir
		key := filepath.Join(it.pid, it.id+".bin")
		now := time.Now().UTC().Format(time.RFC3339Nano)
		res, err := db.Exec(`
                        UPDATE files
                        SET storage_kind='fs', storage_key=?, data=NULL, size_bytes=?, updated_at=?
                        WHERE id=? AND storage_kind='blob'
                `, key, len(it.data), now, it.id)
		if err != nil {
			return moved, fmt.Errorf("migrateBlobToFS: update %s: %w", it.id, err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			// гонка: кто-то уже переключил — не считаем.
			slog.Warn("migrateBlobToFS: update skipped (already moved?)",
				"id", it.id, "patient_id", it.pid)
			continue
		}
		moved++
	}
	slog.Info("migrateBlobToFS: done", "moved", moved, "candidates", len(items))
	return moved, nil
}

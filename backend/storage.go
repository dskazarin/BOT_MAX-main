package main

// storage.go — B30.10 §6.59 step2: абстракция хранилища файлов.
//
// На этом шаге реализована только BLOB-ветка (kind=blob): данные лежат в
// files.data, поведение patient_records.go НЕ меняется. Интерфейс вводится
// заранее, чтобы:
//   - Шаг 3 добавил fs-реализацию без переписывания вызывающего кода;
//   - Шаг 4 (миграция схемы) и Шаг 5 (main.go: setStorageCfg + MkdirAll)
//     подключили выбор реализации по StorageCfg.Kind.

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// FileStorage — общий контракт. Реализации: blobStorage (сейчас), fsStorage (Шаг 3).
type FileStorage interface {
	Put(pid, id, filename, mime string, data []byte) error
	Get(pid, id string) (filename, mime string, data []byte, ok bool, err error)
	Delete(pid, id string) error
}

// blobStorage — files.data BLOB. Семантика как в patient_records.go:
// upsert по id, soft delete через deleted_at.
type blobStorage struct {
	db *sql.DB
}

func newBlobStorage(db *sql.DB) *blobStorage {
	return &blobStorage{db: db}
}

// Put — upsert по id. Если строка есть — UPDATE; иначе INSERT.
func (s *blobStorage) Put(pid, id, filename, mime string, data []byte) error {
	if pid == "" || id == "" {
		return fmt.Errorf("blobStorage.Put: пустой pid/id")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	clinic, err := clinicIDForPatient(pid)
	if err != nil {
		return fmt.Errorf("blobStorage.Put: clinic_id: %w", err)
	}
	res, err := s.db.Exec(
		`UPDATE files SET filename=?, mime=?, size_bytes=?, data=?, updated_at=?, deleted_at=NULL
                 WHERE id=? AND patient_id=?`,
		filename, mime, len(data), data, now, id, pid,
	)
	if err != nil {
		return fmt.Errorf("blobStorage.Put: update: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.db.Exec(
		`INSERT INTO files (id, patient_id, clinic_id, filename, mime, size_bytes, data, created_at, updated_at)
                 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, pid, clinic, filename, mime, len(data), data, now, now,
	)
	if err != nil {
		return fmt.Errorf("blobStorage.Put: insert: %w", err)
	}
	return nil
}

// Get — прочитать живой файл. ok=false, если строки нет / удалён / data пуст.
// err — только реальные ошибки БД (не «не найдено»).
func (s *blobStorage) Get(pid, id string) (filename, mime string, data []byte, ok bool, err error) {
	var (
		fn sql.NullString
		mt sql.NullString
		d  []byte
	)
	row := s.db.QueryRow(
		`SELECT filename, mime, data FROM files
                 WHERE id=? AND patient_id=? AND deleted_at IS NULL`,
		id, pid,
	)
	if err := row.Scan(&fn, &mt, &d); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", "", nil, false, nil
		}
		return "", "", nil, false, fmt.Errorf("blobStorage.Get: %w", err)
	}
	if len(d) == 0 {
		return fn.String, mt.String, nil, false, nil
	}
	return fn.String, mt.String, d, true, nil
}

// Delete — soft delete (deleted_at = now). Идемпотентно.
func (s *blobStorage) Delete(pid, id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := s.db.Exec(
		`UPDATE files SET deleted_at=?, updated_at=? WHERE id=? AND patient_id=? AND deleted_at IS NULL`,
		now, now, id, pid,
	)
	if err != nil {
		return fmt.Errorf("blobStorage.Delete: %w", err)
	}
	return nil
}

package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"database/sql"
)

// openGCStorage — открыть SQLite, создать таблицу files (минимально), вернуть storage.
func openGCStorage(t *testing.T) (*sql.DB, *fsStorage) {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "gc_test.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, err = db.Exec(`
		CREATE TABLE files (
			id TEXT PRIMARY KEY,
			patient_id TEXT NOT NULL,
			storage_kind TEXT NOT NULL DEFAULT 'blob',
			storage_key TEXT,
			deleted_at TEXT
		);
	`)
	if err != nil {
		t.Fatalf("create files: %v", err)
	}

	storage, err := newFSStorage(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatalf("newFSStorage: %v", err)
	}
	return db, storage
}

// insertFile — вставка + физическая запись через storage.Put.
func insertFile(t *testing.T, db *sql.DB, s *fsStorage, pid, id, deletedAt string) {
	t.Helper()
	if err := s.Put(pid, id, id+".bin", "application/octet-stream", []byte("payload-"+id)); err != nil {
		t.Fatalf("Put: %v", err)
	}
	_, err := db.Exec(
		`INSERT INTO files(id, patient_id, storage_kind, storage_key, deleted_at)
		 VALUES(?, ?, 'fs', ?, ?)`,
		id, pid, id+".key", sql.NullString{String: deletedAt, Valid: deletedAt != ""},
	)
	if err != nil {
		t.Fatalf("insert file %s: %v", id, err)
	}
}

func fileExists(t *testing.T, s *fsStorage, pid, id string) bool {
	t.Helper()
	_, _, _, ok, err := s.Get(pid, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	return ok
}

// --- Тесты ---

func TestGCStorage_DeletesExpiredSoftDeleted(t *testing.T) {
	db, s := openGCStorage(t)
	pid, id := "P-1", "F-1"
	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339Nano)
	insertFile(t, db, s, pid, id, old)

	n, err := GCStorage(context.Background(), s, db, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GCStorage: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted=%d, want 1", n)
	}
	if fileExists(t, s, pid, id) {
		t.Fatalf("файл должен быть физически удалён")
	}
}

func TestGCStorage_KeepsRecentSoftDeleted(t *testing.T) {
	db, s := openGCStorage(t)
	pid, id := "P-1", "F-1"
	recent := time.Now().UTC().Add(-1 * 24 * time.Hour).Format(time.RFC3339Nano)
	insertFile(t, db, s, pid, id, recent)

	n, err := GCStorage(context.Background(), s, db, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GCStorage: %v", err)
	}
	if n != 0 {
		t.Fatalf("deleted=%d, want 0", n)
	}
	if !fileExists(t, s, pid, id) {
		t.Fatalf("свежий soft-deleted файл не должен удаляться")
	}
}

func TestGCStorage_KeepsAlive(t *testing.T) {
	db, s := openGCStorage(t)
	insertFile(t, db, s, "P-1", "F-1", "") // deleted_at IS NULL
	n, err := GCStorage(context.Background(), s, db, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GCStorage: %v", err)
	}
	if n != 0 {
		t.Fatalf("deleted=%d, want 0 (живые файлы не трогаем)", n)
	}
	if !fileExists(t, s, "P-1", "F-1") {
		t.Fatalf("живой файл не должен удаляться")
	}
}

func TestGCStorage_BlobIsNoop(t *testing.T) {
	db, _ := openGCStorage(t)
	blob := newBlobStorage(db)
	n, err := GCStorage(context.Background(), blob, db, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("GCStorage(blob): %v", err)
	}
	if n != 0 {
		t.Fatalf("blob: deleted=%d, want 0 (no-op)", n)
	}
}

func TestGCStorage_InvalidArgs(t *testing.T) {
	db, s := openGCStorage(t)
	if _, err := GCStorage(context.Background(), s, nil, time.Hour); err == nil {
		t.Fatalf("want error for nil db")
	}
	if _, err := GCStorage(context.Background(), s, db, 0); err == nil {
		t.Fatalf("want error for olderThan=0")
	}
}

func TestGCStorage_ContextCanceled(t *testing.T) {
	db, s := openGCStorage(t)
	old := time.Now().UTC().Add(-40 * 24 * time.Hour).Format(time.RFC3339Nano)
	insertFile(t, db, s, "P-1", "F-1", old)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GCStorage(ctx, s, db, 30*24*time.Hour)
	if err == nil {
		t.Fatalf("want ctx.Err, got nil")
	}
}

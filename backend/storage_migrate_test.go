package main

// storage_migrate_test.go — B30.10 §6.59 step4b: migrateBlobToFS.
// setupPatientVitalsDB поднимает глобальную db (InitDB, схема уже с storage_*).
// fs — t.TempDir() (изолированно на тест).

import (
	"bytes"
	"database/sql"
	"testing"
)

func mustFS(t *testing.T) *fsStorage {
	t.Helper()
	fs, err := newFSStorage(t.TempDir())
	if err != nil {
		t.Fatalf("newFSStorage: %v", err)
	}
	return fs
}

func insertBlobFile(t *testing.T, id, pid string, data []byte) {
	t.Helper()
	_, err := db.Exec(`
                INSERT INTO files (id, patient_id, filename, mime, size_bytes, data, created_at, updated_at, storage_kind)
                VALUES (?, ?, 'x.bin', 'application/octet-stream', ?, ?, datetime('now'), datetime('now'), 'blob')
        `, id, pid, len(data), data)
	if err != nil {
		t.Fatalf("insert blob file %s: %v", id, err)
	}
}

func countFSRows(t *testing.T) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM files WHERE storage_kind='fs'`).Scan(&n); err != nil {
		t.Fatalf("count fs: %v", err)
	}
	return n
}

func TestMigrateBlobToFS_Basic(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-M1", "ACC-M1")
	fs := mustFS(t)

	raw1 := []byte("first")
	raw2 := []byte("second-longer")
	insertBlobFile(t, "F-M1", "PAT-M1", raw1)
	insertBlobFile(t, "F-M2", "PAT-M1", raw2)

	moved, err := migrateBlobToFS(db, fs)
	if err != nil {
		t.Fatalf("migrateBlobToFS: %v", err)
	}
	if moved != 2 {
		t.Fatalf("moved=%d, want 2", moved)
	}
	if n := countFSRows(t); n != 2 {
		t.Fatalf("fs rows=%d, want 2", n)
	}

	for _, tc := range []struct {
		id  string
		raw []byte
	}{{"F-M1", raw1}, {"F-M2", raw2}} {
		var (
			d   sql.NullString
			key string
			sz  int
		)
		if err := db.QueryRow(`SELECT data, storage_key, size_bytes FROM files WHERE id=?`, tc.id).
			Scan(&d, &key, &sz); err != nil {
			t.Fatalf("select %s: %v", tc.id, err)
		}
		if d.Valid {
			t.Fatalf("%s: data=%v, want NULL", tc.id, d.String)
		}
		if key == "" {
			t.Fatalf("%s: storage_key пуст", tc.id)
		}
		if sz != len(tc.raw) {
			t.Fatalf("%s: size_bytes=%d, want %d", tc.id, sz, len(tc.raw))
		}
		got, ok, err := fs.GetBlob("PAT-M1", tc.id)
		if err != nil || !ok {
			t.Fatalf("%s: fs.GetBlob ok=%v err=%v", tc.id, ok, err)
		}
		if !bytes.Equal(got, tc.raw) {
			t.Fatalf("%s: fs data=%q want=%q", tc.id, got, tc.raw)
		}
	}
}

func TestMigrateBlobToFS_Idempotent(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-M2", "ACC-M2")
	fs := mustFS(t)
	insertBlobFile(t, "F-M3", "PAT-M2", []byte("x"))

	if _, err := migrateBlobToFS(db, fs); err != nil {
		t.Fatalf("первый migrate: %v", err)
	}
	moved, err := migrateBlobToFS(db, fs)
	if err != nil {
		t.Fatalf("второй migrate: %v", err)
	}
	if moved != 0 {
		t.Fatalf("второй moved=%d, want 0", moved)
	}
}

func TestMigrateBlobToFS_SkipsDeleted(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-M3", "ACC-M3")
	fs := mustFS(t)

	if _, err := db.Exec(`
                INSERT INTO files (id, patient_id, filename, mime, size_bytes, data, created_at, updated_at, deleted_at, storage_kind)
                VALUES ('F-DEL','PAT-M3','x.bin','application/octet-stream',1, x'00', datetime('now'), datetime('now'), datetime('now'), 'blob')
        `); err != nil {
		t.Fatalf("insert deleted: %v", err)
	}

	moved, err := migrateBlobToFS(db, fs)
	if err != nil {
		t.Fatalf("migrateBlobToFS: %v", err)
	}
	if moved != 0 {
		t.Fatalf("moved=%d, want 0 (deleted пропускается)", moved)
	}
	var kind string
	var d sql.NullString
	if err := db.QueryRow(`SELECT storage_kind, data FROM files WHERE id='F-DEL'`).Scan(&kind, &d); err != nil {
		t.Fatalf("select: %v", err)
	}
	if kind != "blob" {
		t.Fatalf("storage_kind=%q, want 'blob'", kind)
	}
	if !d.Valid {
		t.Fatalf("data=NULL у deleted, want BLOB")
	}
}

func TestMigrateBlobToFS_SkipsEmpty(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-M4", "ACC-M4")
	fs := mustFS(t)

	if _, err := db.Exec(`
                INSERT INTO files (id, patient_id, filename, mime, size_bytes, data, created_at, updated_at, storage_kind)
                VALUES ('F-EMP','PAT-M4','x.bin','application/octet-stream',0, NULL, datetime('now'), datetime('now'), 'blob')
        `); err != nil {
		t.Fatalf("insert empty: %v", err)
	}

	moved, err := migrateBlobToFS(db, fs)
	if err != nil {
		t.Fatalf("migrateBlobToFS: %v", err)
	}
	if moved != 0 {
		t.Fatalf("moved=%d, want 0 (пустые пропускаются)", moved)
	}
}

func TestMigrateBlobToFS_SkipsAlreadyFS(t *testing.T) {
	setupPatientVitalsDB(t)
	seedPatientWithAccount(t, "PAT-M5", "ACC-M5")
	fs := mustFS(t)

	if _, err := db.Exec(`
                INSERT INTO files (id, patient_id, filename, mime, size_bytes, data, created_at, updated_at, storage_kind, storage_key)
                VALUES ('F-FS','PAT-M5','x.bin','application/octet-stream',1, NULL, datetime('now'), datetime('now'), 'fs', 'PAT-M5/F-FS.bin')
        `); err != nil {
		t.Fatalf("insert fs: %v", err)
	}

	moved, err := migrateBlobToFS(db, fs)
	if err != nil {
		t.Fatalf("migrateBlobToFS: %v", err)
	}
	if moved != 0 {
		t.Fatalf("moved=%d, want 0 (уже fs)", moved)
	}
	var key string
	if err := db.QueryRow(`SELECT storage_key FROM files WHERE id='F-FS'`).Scan(&key); err != nil {
		t.Fatalf("select: %v", err)
	}
	if key != "PAT-M5/F-FS.bin" {
		t.Fatalf("storage_key=%q, want 'PAT-M5/F-FS.bin' (не перезаписан)", key)
	}
}

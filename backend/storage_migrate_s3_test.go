package main

import (
	"testing"
)

func TestMigrateToS3_BlobAndFS(t *testing.T) {
	db := newFileTestDB(t)
	defer db.Close()

	now := "2026-11-12T00:00:00Z"

	// 0) клиника + пациент (FK для files).
	if _, err := db.Exec(`INSERT INTO clinics (id, name, created) VALUES ('C1', 'Test Clinic', ?)`, now); err != nil {
		t.Fatalf("insert clinic: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO patients (id, clinic_id, name, created) VALUES ('P1', 'C1', 'Test Patient', ?)`, now); err != nil {
		t.Fatalf("insert patient: %v", err)
	}

	// 1) blob-фикстура.
	if _, err := db.Exec(`INSERT INTO files
		(id, patient_id, clinic_id, filename, mime, size_bytes, data, storage_kind, created_at, updated_at)
		VALUES ('F-BLOB','P1','C1','a.txt','text/plain',5,'hello','blob',?,?)`, now, now); err != nil {
		t.Fatalf("insert blob: %v", err)
	}

	// 2) fs-фикстура: файл на диске + строка с storage_kind='fs'.
	tmp := t.TempDir()
	fs, err := newFSStorage(tmp)
	if err != nil {
		t.Fatalf("newFSStorage: %v", err)
	}
	if err := fs.PutBlob("P1", "F-FS", []byte("world")); err != nil {
		t.Fatalf("fs.PutBlob: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO files
		(id, patient_id, clinic_id, filename, mime, size_bytes, data, storage_kind, storage_key, created_at, updated_at)
		VALUES ('F-FS','P1','C1','b.txt','text/plain',5,NULL,'fs','P1/F-FS.bin',?,?)`, now, now); err != nil {
		t.Fatalf("insert fs: %v", err)
	}

	// 3) s3-фикстура — должна быть пропущена (уже s3).
	if _, err := db.Exec(`INSERT INTO files
		(id, patient_id, clinic_id, filename, mime, size_bytes, data, storage_kind, storage_key, created_at, updated_at)
		VALUES ('F-S3','P1','C1','c.txt','text/plain',5,NULL,'s3','patients/P1/F-S3',?,?)`, now, now); err != nil {
		t.Fatalf("insert s3: %v", err)
	}

	st := newStubS3()
	s3 := &s3Storage{client: st, bucket: "b"}

	moved, err := migrateToS3(db, fs, s3)
	if err != nil {
		t.Fatalf("migrateToS3: %v", err)
	}
	if moved != 2 {
		t.Fatalf("moved=%d, want 2", moved)
	}

	// Оба объекта в стабе.
	for _, key := range []string{"patients/P1/F-BLOB", "patients/P1/F-FS"} {
		if _, ok := st.objects[key]; !ok {
			t.Fatalf("объект %s не в S3", key)
		}
	}
	// F-S3 не должен попасть в S3 (уже там).
	if _, ok := st.objects["patients/P1/F-S3"]; ok {
		t.Fatal("s3-файл не должен мигрировать повторно")
	}

	// БД: все три — s3, data=NULL.
	rows, err := db.Query(`SELECT id, storage_kind, data, storage_key FROM files ORDER BY id`)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id, kind string
		var data []byte
		var key string
		if err := rows.Scan(&id, &kind, &data, &key); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if kind != "s3" {
			t.Fatalf("%s: kind=%q, want s3", id, kind)
		}
		if data != nil {
			t.Fatalf("%s: data=%v, want NULL", id, data)
		}
		if key == "" {
			t.Fatalf("%s: storage_key пуст", id)
		}
	}

	// fs-файл на диске удалён (best-effort).
	if _, _, _, ok, _ := fs.Get("P1", "F-FS"); ok {
		t.Fatal("fs-файл не удалён после миграции")
	}
}

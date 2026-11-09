package main

// storage_test.go — B30.10 §6.59 step2: blobStorage на files.data BLOB.
// Без HTTP: только *sql.DB (глобальная db из setupPatientVitalsDB) и seedPatientWithAccount.
// Поведение patient_records.go не проверяем — там свои тесты (не ломаем).

import (
        "bytes"
        "testing"
)

func TestBlobStorage_PutGetDelete(t *testing.T) {
        setupPatientVitalsDB(t)
        seedPatientWithAccount(t, "PAT-S1", "ACC-S1")

        s := newBlobStorage(db)
        raw := []byte("hello, blob")

        if err := s.Put("PAT-S1", "F-S1", "a.txt", "text/plain", raw); err != nil {
                t.Fatalf("Put: %v", err)
        }

        fn, mt, got, ok, err := s.Get("PAT-S1", "F-S1")
        if err != nil {
                t.Fatalf("Get: %v", err)
        }
        if !ok {
                t.Fatalf("Get: ok=false, ожидалось true")
        }
        if fn != "a.txt" || mt != "text/plain" {
                t.Fatalf("Get meta: fn=%q mt=%q", fn, mt)
        }
        if !bytes.Equal(got, raw) {
                t.Fatalf("Get data: got=%q want=%q", got, raw)
        }

        if err := s.Delete("PAT-S1", "F-S1"); err != nil {
                t.Fatalf("Delete: %v", err)
        }
        _, _, _, ok, err = s.Get("PAT-S1", "F-S1")
        if err != nil {
                t.Fatalf("Get после Delete: %v", err)
        }
        if ok {
                t.Fatalf("Get после Delete: ok=true, ожидалось false")
        }

        // идемпотентность: повторный Delete — без ошибки
        if err := s.Delete("PAT-S1", "F-S1"); err != nil {
                t.Fatalf("Delete повторно: %v", err)
        }
}

func TestBlobStorage_PutUpsert(t *testing.T) {
        setupPatientVitalsDB(t)
        seedPatientWithAccount(t, "PAT-S2", "ACC-S2")

        s := newBlobStorage(db)
        if err := s.Put("PAT-S2", "F-S2", "v1.txt", "text/plain", []byte("v1")); err != nil {
                t.Fatalf("Put v1: %v", err)
        }
        var created1 string
        if err := db.QueryRow(`SELECT created_at FROM files WHERE id='F-S2'`).Scan(&created1); err != nil {
                t.Fatalf("SELECT created_at: %v", err)
        }

        if err := s.Put("PAT-S2", "F-S2", "v2.txt", "text/plain", []byte("v2!")); err != nil {
                t.Fatalf("Put v2: %v", err)
        }
        fn, _, data, ok, err := s.Get("PAT-S2", "F-S2")
        if err != nil || !ok {
                t.Fatalf("Get после upsert: ok=%v err=%v", ok, err)
        }
        if fn != "v2.txt" || string(data) != "v2!" {
                t.Fatalf("upsert не применился: fn=%q data=%q", fn, data)
        }
        var created2 string
        if err := db.QueryRow(`SELECT created_at FROM files WHERE id='F-S2'`).Scan(&created2); err != nil {
                t.Fatalf("SELECT created_at (2): %v", err)
        }
        if created1 != created2 {
                t.Fatalf("created_at изменился: %q → %q", created1, created2)
        }
}

func TestBlobStorage_GetMissing(t *testing.T) {
        setupPatientVitalsDB(t)
        seedPatientWithAccount(t, "PAT-S3", "ACC-S3")

        s := newBlobStorage(db)
        _, _, _, ok, err := s.Get("PAT-S3", "NOPE")
        if err != nil {
                t.Fatalf("Get missing: err=%v, ожидалось nil (honest «не найдено»)", err)
        }
        if ok {
                t.Fatalf("Get missing: ok=true, ожидалось false")
        }
}

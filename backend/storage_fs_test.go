package main

// storage_fs_test.go — B30.10 §6.59 step3: fsStorage на t.TempDir().
// Без БД и без HTTP — проверяем только файловый слой.

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFSStorage_PutGetDelete(t *testing.T) {
	dir := t.TempDir()
	s, err := newFSStorage(dir)
	if err != nil {
		t.Fatalf("newFSStorage: %v", err)
	}
	raw := []byte("hello, fs")

	if err := s.PutBlob("PAT-1", "F-1", raw); err != nil {
		t.Fatalf("PutBlob: %v", err)
	}
	want := filepath.Join(dir, "PAT-1", "F-1.bin")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("файл не создан: %v", err)
	}

	got, ok, err := s.GetBlob("PAT-1", "F-1")
	if err != nil || !ok {
		t.Fatalf("GetBlob: ok=%v err=%v", ok, err)
	}
	if !bytes.Equal(got, raw) {
		t.Fatalf("GetBlob data=%q want=%q", got, raw)
	}

	if err := s.DeleteBlob("PAT-1", "F-1"); err != nil {
		t.Fatalf("DeleteBlob: %v", err)
	}
	_, ok, err = s.GetBlob("PAT-1", "F-1")
	if err != nil {
		t.Fatalf("GetBlob после Delete: %v", err)
	}
	if ok {
		t.Fatalf("GetBlob после Delete: ok=true, ожидалось false")
	}
	if err := s.DeleteBlob("PAT-1", "F-1"); err != nil {
		t.Fatalf("DeleteBlob повторно: %v", err)
	}
}

func TestFSStorage_Overwrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := newFSStorage(dir)

	if err := s.PutBlob("P", "F", []byte("v1")); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := s.PutBlob("P", "F", []byte("v2-longer")); err != nil {
		t.Fatalf("Put v2: %v", err)
	}
	got, ok, err := s.GetBlob("P", "F")
	if err != nil || !ok {
		t.Fatalf("GetBlob: ok=%v err=%v", ok, err)
	}
	if string(got) != "v2-longer" {
		t.Fatalf("overwrite не применился: %q", got)
	}
}

func TestFSStorage_GetMissing(t *testing.T) {
	s, _ := newFSStorage(t.TempDir())
	_, ok, err := s.GetBlob("P", "NOPE")
	if err != nil {
		t.Fatalf("GetBlob missing: err=%v, ожидалось nil", err)
	}
	if ok {
		t.Fatalf("GetBlob missing: ok=true, ожидалось false")
	}
}

func TestFSStorage_RejectUnsafeIDs(t *testing.T) {
	s, _ := newFSStorage(t.TempDir())
	bad := []struct{ pid, id string }{
		{"../escape", "F"},
		{"P", "../escape"},
		{"P", "a/b"},
		{"P", ".."},
		{"", "F"},
		{"P", ""},
		{"P", "a b"},
	}
	for _, c := range bad {
		if err := s.PutBlob(c.pid, c.id, []byte("x")); err == nil {
			t.Fatalf("PutBlob(%q,%q): ожидалась ошибка", c.pid, c.id)
		}
	}
}

func TestSafeID(t *testing.T) {
	okCases := []string{"PAT-1", "F_1", "a.b", "ABC123", "x-y_z.9"}
	for _, v := range okCases {
		if _, err := safeID("id", v); err != nil {
			t.Fatalf("safeID(%q): %v", v, err)
		}
	}
	badCases := []string{"", " ", ".", "..", "a/b", `a\b`, "a b", "a:b", "a;b"}
	for _, v := range badCases {
		if _, err := safeID("id", v); err == nil {
			t.Fatalf("safeID(%q): ожидалась ошибка", v)
		}
	}
}

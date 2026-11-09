package main

// storage_cfg_test.go — B30.10 §6.59 step1: парсинг конфигурации хранилища.
// Не трогает диск и БД: только parse*/load* с подменой env.

import (
        "os"
        "path/filepath"
        "testing"
)

func TestParseStorageKind(t *testing.T) {
        cases := []struct {
                in   string
                want StorageKind
                err  bool
        }{
                {"", StorageKindBlob, false},
                {"blob", StorageKindBlob, false},
                {"BLOB", StorageKindBlob, false},
                {" fs ", StorageKindFS, false},
                {"s3", StorageKindS3, false},
                {"nfs", "", true},
                {"blobs", "", true},
        }
        for _, c := range cases {
                got, err := parseStorageKind(c.in)
                if (err != nil) != c.err {
                        t.Fatalf("parseStorageKind(%q): err=%v, want err=%v", c.in, err, c.err)
                }
                if err == nil && got != c.want {
                        t.Fatalf("parseStorageKind(%q)=%q, want %q", c.in, got, c.want)
                }
        }
}

func TestParseStorageMaxMiB(t *testing.T) {
        if v, err := parseStorageMaxMiB(""); err != nil || v != DefaultStorageMaxMiB {
                t.Fatalf("пусто: got (%d,%v), want (%d,nil)", v, err, DefaultStorageMaxMiB)
        }
        if v, err := parseStorageMaxMiB("20"); err != nil || v != 20 {
                t.Fatalf("20: got (%d,%v), want (20,nil)", v, err)
        }
        if _, err := parseStorageMaxMiB("0"); err == nil {
                t.Fatalf("0: ожидалась ошибка")
        }
        if _, err := parseStorageMaxMiB("-1"); err == nil {
                t.Fatalf("-1: ожидалась ошибка")
        }
        if _, err := parseStorageMaxMiB("abc"); err == nil {
                t.Fatalf("abc: ожидалась ошибка")
        }
}

func TestParseStorageMigrate(t *testing.T) {
        truthy := []string{"1", "true", "TRUE", "yes", "on"}
        falsy := []string{"", "0", "false", "no", "off"}
        for _, s := range truthy {
                if v, err := parseStorageMigrate(s); err != nil || !v {
                        t.Fatalf("parseStorageMigrate(%q)=(%v,%v), want (true,nil)", s, v, err)
                }
        }
        for _, s := range falsy {
                if v, err := parseStorageMigrate(s); err != nil || v {
                        t.Fatalf("parseStorageMigrate(%q)=(%v,%v), want (false,nil)", s, v, err)
                }
        }
        if _, err := parseStorageMigrate("maybe"); err == nil {
                t.Fatalf("maybe: ожидалась ошибка")
        }
}

func TestLoadStorageCfg_Defaults(t *testing.T) {
        t.Setenv("BOTMAX_STORAGE_KIND", "")
        t.Setenv("BOTMAX_STORAGE_DIR", "")
        t.Setenv("BOTMAX_STORAGE_MAX_MIB", "")
        t.Setenv("BOTMAX_STORAGE_MIGRATE", "")

        cfg, err := loadStorageCfg("")
        if err != nil {
                t.Fatalf("loadStorageCfg: %v", err)
        }
        if cfg.Kind != StorageKindBlob {
                t.Fatalf("Kind=%q, want blob", cfg.Kind)
        }
        if cfg.Dir != "" {
                t.Fatalf("Dir=%q, want \"\" для blob", cfg.Dir)
        }
        if cfg.MaxMiB != DefaultStorageMaxMiB {
                t.Fatalf("MaxMiB=%d, want %d", cfg.MaxMiB, DefaultStorageMaxMiB)
        }
        if cfg.Migrate {
                t.Fatalf("Migrate=true, want false")
        }
}

func TestLoadStorageCfg_FS(t *testing.T) {
        dir := t.TempDir()
        t.Setenv("BOTMAX_STORAGE_KIND", "fs")
        t.Setenv("BOTMAX_STORAGE_DIR", dir)
        t.Setenv("BOTMAX_STORAGE_MAX_MIB", "42")
        t.Setenv("BOTMAX_STORAGE_MIGRATE", "1")

        cfg, err := loadStorageCfg(filepath.Join(t.TempDir(), "x.db"))
        if err != nil {
                t.Fatalf("loadStorageCfg: %v", err)
        }
        if cfg.Kind != StorageKindFS {
                t.Fatalf("Kind=%q, want fs", cfg.Kind)
        }
        if cfg.Dir != dir {
                t.Fatalf("Dir=%q, want %q", cfg.Dir, dir)
        }
        if cfg.MaxMiB != 42 {
                t.Fatalf("MaxMiB=%d, want 42", cfg.MaxMiB)
        }
        if !cfg.Migrate {
                t.Fatalf("Migrate=false, want true")
        }
}

func TestLoadStorageCfg_Errors(t *testing.T) {
        t.Setenv("BOTMAX_STORAGE_KIND", "s3")
        if _, err := loadStorageCfg(""); err == nil {
                t.Fatalf("s3: ожидалась ошибка (не реализовано)")
        }

        t.Setenv("BOTMAX_STORAGE_KIND", "blob")
        t.Setenv("BOTMAX_STORAGE_MIGRATE", "1")
        if _, err := loadStorageCfg(""); err == nil {
                t.Fatalf("migrate без fs: ожидалась ошибка")
        }

        t.Setenv("BOTMAX_STORAGE_KIND", "nfs")
        t.Setenv("BOTMAX_STORAGE_MIGRATE", "")
        if _, err := loadStorageCfg(""); err == nil {
                t.Fatalf("nfs: ожидалась ошибка")
        }
}

func TestResolveStorageDir_DefaultNextToDB(t *testing.T) {
        got, err := resolveStorageDir(StorageKindFS, "", "/tmp/dbroot/x.db")
        if err != nil {
                t.Fatalf("resolveStorageDir: %v", err)
        }
        want := filepath.Clean("/tmp/dbroot/storage")
        if filepath.Clean(got) != want {
                t.Fatalf("Dir=%q, want %q", got, want)
        }
        if _, err := os.Stat(got); err == nil {
                t.Fatalf("resolveStorageDir не должен создавать каталог")
        }
}

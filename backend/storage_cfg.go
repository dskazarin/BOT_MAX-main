package main

// storage_cfg.go — B30.10 §6.59: конфигурация хранилища файлов.
//
// KIND:   blob | fs | s3 (s3 — зарезервировано под бэклог, пока не реализовано)
// DIR:    базовый каталог для kind=fs (по умолчанию <db_dir>/storage)
// MAX_MIB: максимальный размер файла в МиБ (по умолчанию 6)
// MIGRATE: 1 — при старте мигрировать BLOB → FS (реализуется в storage_migrate.go)
//
// На Шаге 1 конфиг только читается/валидируется. Потребители (patient_records,
// main) подключаются на Шагах 2–5, поэтому поведение рантайма НЕ меняется.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StorageKind — режим хранения.
type StorageKind string

const (
	StorageKindBlob StorageKind = "blob"
	StorageKindFS   StorageKind = "fs"
	StorageKindS3   StorageKind = "s3"
)

// DefaultStorageMaxMiB — дефолт, совпадает с текущим хардкодом UI (6 МиБ).
const DefaultStorageMaxMiB = 6

// S3Cfg — параметры S3-совместимого хранилища (B30.14).
// Заполняется из BOTMAX_STORAGE_S3_* только при KIND=s3.
type S3Cfg struct {
	Endpoint  string
	Bucket    string
	AccessKey string
	SecretKey string
	Region    string
	UseSSL    bool
}

// StorageCfg — итоговая конфигурация (заполняется из Env при старте).
type StorageCfg struct {
	Kind    StorageKind
	Dir     string // только для kind=fs
	MaxMiB  int    // > 0
	Migrate bool   // только для kind=fs
	S3      S3Cfg  // только для kind=s3
}


// parseStorageKind — BOTMAX_STORAGE_KIND. Пусто → blob (текущее поведение).
func parseStorageKind(raw string) (StorageKind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "blob":
		return StorageKindBlob, nil
	case "fs":
		return StorageKindFS, nil
	case "s3":
		return StorageKindS3, nil
	default:
		return "", fmt.Errorf("BOTMAX_STORAGE_KIND=%q: ожидается blob|fs|s3", raw)
	}
}

// parseStorageMaxMiB — BOTMAX_STORAGE_MAX_MIB. Пусто → DefaultStorageMaxMiB.
func parseStorageMaxMiB(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return DefaultStorageMaxMiB, nil
	}
	n, err := parseNonNegativeInt("BOTMAX_STORAGE_MAX_MIB", raw, DefaultStorageMaxMiB)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("BOTMAX_STORAGE_MAX_MIB=%q: должно быть > 0", raw)
	}
	return n, nil
}

// parseStorageMigrate — BOTMAX_STORAGE_MIGRATE. Пусто → false.
func parseStorageMigrate(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "0", "false", "no", "off":
		return false, nil
	case "1", "true", "yes", "on":
		return true, nil
	default:
		return false, fmt.Errorf("BOTMAX_STORAGE_MIGRATE=%q: ожидается 1|true|yes|on либо 0|false|no|off", raw)
	}
}

// resolveStorageDir — BOTMAX_STORAGE_DIR. Пусто → <dbDir>/storage.
// Для kind!=fs не используется (возвращаем "").
func resolveStorageDir(kind StorageKind, raw, dbPath string) (string, error) {
	raw = strings.TrimSpace(raw)
	if kind != StorageKindFS {
		return "", nil
	}
	if raw != "" {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", fmt.Errorf("BOTMAX_STORAGE_DIR=%q: %w", raw, err)
		}
		return abs, nil
	}
	base := "."
	if dbPath != "" {
		base = filepath.Dir(dbPath)
	}
	abs, err := filepath.Abs(filepath.Join(base, "storage"))
	if err != nil {
		return "", fmt.Errorf("storage dir по умолчанию: %w", err)
	}
	return abs, nil
}

// loadStorageCfg — собрать StorageCfg из окружения. Чистая функция
// (читает os.Getenv, ничего не создаёт на диске). MkdirAll — на Шаге 5 (main).
func loadStorageCfg(dbPath string) (StorageCfg, error) {
	kind, err := parseStorageKind(os.Getenv("BOTMAX_STORAGE_KIND"))
	if err != nil {
		return StorageCfg{}, err
	}
	maxMiB, err := parseStorageMaxMiB(os.Getenv("BOTMAX_STORAGE_MAX_MIB"))
	if err != nil {
		return StorageCfg{}, err
	}
	migrate, err := parseStorageMigrate(os.Getenv("BOTMAX_STORAGE_MIGRATE"))
	if err != nil {
		return StorageCfg{}, err
	}
	dir, err := resolveStorageDir(kind, os.Getenv("BOTMAX_STORAGE_DIR"), dbPath)
	if err != nil {
		return StorageCfg{}, err
	}
	var s3cfg S3Cfg
	if kind == StorageKindS3 {
		var err error
		s3cfg, err = loadS3Cfg()
		if err != nil {
			return StorageCfg{}, err
		}
	}
	if migrate && kind != StorageKindFS {
		return StorageCfg{}, fmt.Errorf("BOTMAX_STORAGE_MIGRATE=1 требует BOTMAX_STORAGE_KIND=fs")
	}
	return StorageCfg{Kind: kind, Dir: dir, MaxMiB: maxMiB, Migrate: migrate, S3: s3cfg}, nil
}

// loadS3Cfg — BOTMAX_STORAGE_S3_* → S3Cfg. Обязательны endpoint/bucket/access_key/secret_key.
func loadS3Cfg() (S3Cfg, error) {
	cfg := S3Cfg{
		Endpoint:  os.Getenv("BOTMAX_STORAGE_S3_ENDPOINT"),
		Bucket:    os.Getenv("BOTMAX_STORAGE_S3_BUCKET"),
		AccessKey: os.Getenv("BOTMAX_STORAGE_S3_ACCESS_KEY"),
		SecretKey: os.Getenv("BOTMAX_STORAGE_S3_SECRET_KEY"),
		Region:    os.Getenv("BOTMAX_STORAGE_S3_REGION"),
		UseSSL:    true,
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	if v := os.Getenv("BOTMAX_STORAGE_S3_USE_SSL"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return S3Cfg{}, fmt.Errorf("BOTMAX_STORAGE_S3_USE_SSL=%q: %w", v, err)
		}
		cfg.UseSSL = b
	}
	if cfg.Endpoint == "" {
		return S3Cfg{}, fmt.Errorf("BOTMAX_STORAGE_S3_ENDPOINT обязателен при KIND=s3")
	}
	if cfg.Bucket == "" {
		return S3Cfg{}, fmt.Errorf("BOTMAX_STORAGE_S3_BUCKET обязателен при KIND=s3")
	}
	if cfg.AccessKey == "" || cfg.SecretKey == "" {
		return S3Cfg{}, fmt.Errorf("BOTMAX_STORAGE_S3_ACCESS_KEY/SECRET_KEY обязательны при KIND=s3")
	}
	return cfg, nil
}

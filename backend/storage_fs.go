package main

// storage_fs.go — B30.10 §6.59 step3: fs-реализация файлового слоя.
//
// На этом шаге — ТОЛЬКО работа с диском, без БД:
//   - файлы лежат в <dir>/<pid>/<id>.bin
//   - <pid>/<id> санитизируются (никаких ../, никаких разделителей пути)
//
// Привязка к БД (files.storage_kind='fs', files.storage_key=<relpath>,
// files.data=NULL) — Шаг 4 (миграция схемы) + Шаг 5 (main.go: setStorageCfg).
// patient_records.go на этом шаге НЕ переключается — поведение рантайма
// не меняется, blobStorage остаётся дефолтом.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fsStorage — файлы на диске. Без БД (пока).
type fsStorage struct {
	dir string
}

func newFSStorage(dir string) (*fsStorage, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("fsStorage: пустой dir")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("fsStorage: abs(%q): %w", dir, err)
	}
	return &fsStorage{dir: abs}, nil
}

// safeID — санитизация pid/id: только [A-Za-z0-9._-], не пусто, не "." / "..".
// Возвращает ошибку на небезопасный ввод (это не «не найдено» — это 4xx).
func safeID(name, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" || v == "." || v == ".." {
		return "", fmt.Errorf("fsStorage: недопустимый %s=%q", name, v)
	}
	if strings.ContainsAny(v, `/\`) {
		return "", fmt.Errorf("fsStorage: %s=%q содержит разделитель пути", name, v)
	}
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.', r == '_', r == '-':
		default:
			return "", fmt.Errorf("fsStorage: %s=%q содержит недопустимый символ %q", name, v, r)
		}
	}
	return v, nil
}

// pathFor — <dir>/<pid>/<id>.bin. Ошибка, если pid/id небезопасны.
func (s *fsStorage) pathFor(pid, id string) (string, error) {
	p, err := safeID("pid", pid)
	if err != nil {
		return "", err
	}
	i, err := safeID("id", id)
	if err != nil {
		return "", err
	}
	return filepath.Join(s.dir, p, i+".bin"), nil
}

// PutBlob — атомарная запись: tmp → rename. Перезапись разрешена (upsert).
func (s *fsStorage) PutBlob(pid, id string, data []byte) error {
	dst, err := s.pathFor(pid, id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("fsStorage.PutBlob: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".tmp-*")
	if err != nil {
		return fmt.Errorf("fsStorage.PutBlob: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // если rename не случился — подчистим
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("fsStorage.PutBlob: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fsStorage.PutBlob: close: %w", err)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return fmt.Errorf("fsStorage.PutBlob: rename: %w", err)
	}
	return nil
}

// GetBlob — чтение. ok=false, err=nil, если файла нет (honest «не найдено»).
func (s *fsStorage) GetBlob(pid, id string) (data []byte, ok bool, err error) {
	p, err := s.pathFor(pid, id)
	if err != nil {
		return nil, false, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("fsStorage.GetBlob: %w", err)
	}
	return b, true, nil
}

// DeleteBlob — идемпотентно: нет файла — nil.
func (s *fsStorage) DeleteBlob(pid, id string) error {
	p, err := s.pathFor(pid, id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("fsStorage.DeleteBlob: %w", err)
	}
	return nil
}

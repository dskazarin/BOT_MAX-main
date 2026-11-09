package main

// storage_fs_iface.go — B30.10 §6.59 step5a: адаптеры fsStorage → FileStorage.
//
// fsStorage реализует файловый слой (PutBlob/GetBlob/DeleteBlob), а интерфейс
// FileStorage объявлен в storage.go как Put/Get/Delete. Тонкие адаптеры ниже
// связывают одно с другим, не меняя ни интерфейс, ни fsStorage.
//
// ВАЖНО: fsStorage.Get возвращает filename=""/mime="" — метаданные лежат
// в files.* и читаются patient_records. writeFileBlob в 5b берёт
// Content-Type/имя из files.*, а не из storage.Get.

import "fmt"

// Put — адаптер к PutBlob.
func (s *fsStorage) Put(pid, id, filename, mime string, data []byte) error {
	_ = filename
	_ = mime
	return s.PutBlob(pid, id, data)
}

// Get — адаптер к GetBlob. filename/mime не отдаём (они в files.*).
func (s *fsStorage) Get(pid, id string) (filename, mime string, data []byte, ok bool, err error) {
	b, found, err := s.GetBlob(pid, id)
	if err != nil {
		return "", "", nil, false, fmt.Errorf("fsStorage.Get: %w", err)
	}
	if !found {
		return "", "", nil, false, nil
	}
	return "", "", b, true, nil
}

// Delete — адаптер к DeleteBlob (идемпотентно).
func (s *fsStorage) Delete(pid, id string) error {
	return s.DeleteBlob(pid, id)
}

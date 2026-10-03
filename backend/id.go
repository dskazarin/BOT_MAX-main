package main

// id.go — подпатч 6.38 (B28.1): генератор ID для сущностей МИС.
//
// Формат: "<prefix>-<YYYYMMDD>-<8 hex>" — уникален, сортируется по дате,
// читается глазами. Пример: ENC-20261003-a3f1c9e2.
//
// Если prefix == "", используется "<8 hex>" без дефиса.

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// newID — универсальный генератор ID.
func newID(prefix string) (string, error) {
	b := make([]byte, 4) // 4 байта = 8 hex
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("newID rand.Read: %w", err)
	}
	hexPart := hex.EncodeToString(b)
	if prefix == "" {
		return hexPart, nil
	}
	return fmt.Sprintf("%s-%s-%s", prefix, time.Now().UTC().Format("20060102"), hexPart), nil
}

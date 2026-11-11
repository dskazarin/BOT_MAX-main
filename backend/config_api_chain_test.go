package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestConfigAPI_ThroughChain_PublicNoToken — реальный путь запроса:
// recoverMiddleware(authMiddleware(requirePasswordChanged(handleConfig)))
// без токена должен вернуть 200 и дефолтный лимит.
func TestConfigAPI_ThroughChain_PublicNoToken(t *testing.T) {
	oldEnv := currentEnv
	defer func() { currentEnv = oldEnv }()
	currentEnv = &Env{StorageMaxMiB: DefaultStorageMaxMiB, StorageKind: StorageKindBlob, AuthEnforce: true}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/config", handleConfig)
	h := recoverMiddleware(authMiddleware(requirePasswordChanged(mux)))

	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/config")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200 (config должен быть публичным)", resp.StatusCode)
	}
	var got struct {
		StorageMaxMiB int `json:"storage_max_mib"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.StorageMaxMiB != DefaultStorageMaxMiB {
		t.Fatalf("storage_max_mib=%d, want %d", got.StorageMaxMiB, DefaultStorageMaxMiB)
	}
}

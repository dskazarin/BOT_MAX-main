package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func decodeConfig(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%q)", err, rr.Body.String())
	}
	return got
}

func TestConfigAPI_StorageMaxMiB(t *testing.T) {
	old := currentEnv
	defer func() { currentEnv = old }()

	cases := []struct {
		name     string
		env      *Env
		wantMiB  int
		wantKind string
	}{
		{"default-nil-env", nil, DefaultStorageMaxMiB, string(StorageKindBlob)},
		{"custom-42-fs", &Env{StorageMaxMiB: 42, StorageKind: StorageKindFS, Name: "test"}, 42, string(StorageKindFS)},
		{"zero-falls-back", &Env{StorageMaxMiB: 0, StorageKind: StorageKindBlob}, DefaultStorageMaxMiB, string(StorageKindBlob)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			currentEnv = tc.env
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			handleConfig(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d, want 200", rr.Code)
			}
			got := decodeConfig(t, rr)
			if int(got["storage_max_mib"].(float64)) != tc.wantMiB {
				t.Fatalf("storage_max_mib=%v, want %d", got["storage_max_mib"], tc.wantMiB)
			}
			if got["storage_kind"] != tc.wantKind {
				t.Fatalf("storage_kind=%v, want %q", got["storage_kind"], tc.wantKind)
			}
		})
	}
}

func TestConfigAPI_MethodNotAllowed(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/config", nil)
	handleConfig(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d, want 405", rr.Code)
	}
}

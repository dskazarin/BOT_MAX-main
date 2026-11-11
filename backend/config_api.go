package main

import (
	"encoding/json"
	"net/http"
)

// handleConfig отдаёт публичные параметры клиента (без секретов).
// B30.12: UI patient_card.html берёт лимит загрузки отсюда вместо хардкода 6 МиБ.
// Источник — currentEnv (B30.10 §6.59); при nil/нуле — DefaultStorageMaxMiB.
func handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	maxMiB := DefaultStorageMaxMiB
	kind := string(StorageKindBlob)
	name := ""
	if currentEnv != nil {
		if currentEnv.StorageMaxMiB > 0 {
			maxMiB = currentEnv.StorageMaxMiB
		}
		if currentEnv.StorageKind != "" {
			kind = string(currentEnv.StorageKind)
		}
		name = currentEnv.Name
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"storage_max_mib": maxMiB,
		"storage_kind":    kind,
		"name":            name,
	})
}

package main

// impersonation.go — подпатч 6.29 (B20.3): endpoints «глазами доктора».
//
// Схема:
//   POST /api/impersonation/start {doctor_id} — superadmin-only.
//     Выдаёт access-токен с claims actor_id/actor_role/act_as, кладёт
//     его в cookie botmax_access (refresh НЕ трогаем). Пишет audit-запись
//     impersonation.start.
//
//   POST /api/impersonation/stop — работает и в impersonation-режиме,
//     и вне его (идемпотентно, 200 noop). Если actor_id есть в контексте —
//     перевыдаёт actor'у его обычный access-токен, пишет impersonation.stop.
//
// Запрет эскалации: impersonation-токен всегда role=doctor, поэтому
// requireRole(RoleAdmin/Superadmin) автоматически отсекает опасные роуты.
// Дополнительно (B20.4) на CRUD докторов и /api/audit вешается
// requireNonImpersonated.

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// impersonationStartReq — тело POST /api/impersonation/start.
type impersonationStartReq struct {
	DoctorID string `json:"doctor_id"`
}

// impersonationStartResp — ответ на успешный /start.
type impersonationStartResp struct {
	OK         bool        `json:"ok"`
	Target     doctorBrief `json:"target"`
	TTLSeconds int         `json:"ttl_seconds"`
}

// impersonationStopResp — ответ на /stop.
type impersonationStopResp struct {
	OK       bool `json:"ok"`
	Restored bool `json:"restored"` // true — реально вышли; false — noop
}

// impersonationWhoamiResp — ответ на /whoami.
// impersonating=false → все остальные поля пустые (кроме ttl_seconds=0).
type impersonationWhoamiResp struct {
	Impersonating bool   `json:"impersonating"`
	ActorID       string `json:"actor_id,omitempty"`
	ActorRole     string `json:"actor_role,omitempty"`
	TargetID      string `json:"target_id,omitempty"`
	TargetName    string `json:"target_name,omitempty"`
	TargetRole    string `json:"target_role,omitempty"`
}

// registerImpersonationRoutes регистрирует /api/impersonation/{start,stop}.
// Вызывается из main() рядом с registerAuthRoutes().
func registerImpersonationRoutes() {
	guard := requireRole(RoleSuperadmin)

	http.Handle("/api/impersonation/start", guard(http.HandlerFunc(handleImpersonationStart)))
	http.Handle("/api/impersonation/stop", http.HandlerFunc(handleImpersonationStop))
	http.Handle("/api/impersonation/whoami", http.HandlerFunc(handleImpersonationWhoami))

	slog.Info("registerImpersonationRoutes", "routes", "/start (superadmin), /stop, /whoami")
}

// handleImpersonationStart — POST /api/impersonation/start.
func handleImpersonationStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	actorID := doctorIDFromCtx(r)
	actorRole, ok := doctorRoleFromCtx(r)
	if actorID == "" || !ok {
		writeUnauthorized(w)
		return
	}
	// Double-check (requireRole уже отработал, но handler может быть
	// вызван и в тестах напрямую).
	if actorRole != RoleSuperadmin {
		writeForbidden(w)
		return
	}

	var req impersonationStartReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}
	req.DoctorID = strings.TrimSpace(req.DoctorID)
	if req.DoctorID == "" {
		http.Error(w, `{"error":"doctor_id required"}`, http.StatusBadRequest)
		return
	}
	if req.DoctorID == actorID {
		http.Error(w, `{"error":"cannot impersonate yourself"}`, http.StatusBadRequest)
		return
	}

	if db == nil {
		http.Error(w, `{"error":"db not ready"}`, http.StatusInternalServerError)
		return
	}
	target, err := getDoctorRowByID(db, req.DoctorID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"doctor not found"}`, http.StatusNotFound)
			return
		}
		slog.Error("impersonation: get target", "err", err, "doctor_id", req.DoctorID)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	if !target.Active {
		http.Error(w, `{"error":"target doctor is inactive"}`, http.StatusBadRequest)
		return
	}
	// B20.4.1: запрет имперсонировать superadmin'а даже супер-админом.
	// UI скрывает кнопку, но API можно дёрнуть напрямую — нужен guard
	// на бэкенде (defense-in-depth, часть Q6 «запрет эскалации»).
	if target.Role == string(RoleSuperadmin) {
		http.Error(w, `{"error":"cannot impersonate superadmin"}`, http.StatusBadRequest)
		return
	}

	tok, err := IssueImpersonationToken(actorID, actorRole, target.ID)
	if err != nil {
		slog.Error("impersonation: issue token", "err", err)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	// Только access; refresh НЕ трогаем (refresh actor'а остаётся).
	setAuthCookies(w, tok, "")

	auditLog(db, actorID, "impersonation.start", "doctor", target.ID, clientIP(r), map[string]any{
		"actor_role":  string(actorRole),
		"target_role": target.Role,
	})

	slog.Info("impersonation: start",
		"actor_id", actorID, "actor_role", actorRole,
		"target_id", target.ID, "target_role", target.Role)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(impersonationStartResp{
		OK: true,
		Target: doctorBrief{
			ID:   target.ID,
			Name: target.Name,
			Role: target.Role,
		},
		TTLSeconds: int(ImpersonationTTL.Seconds()),
	})
}

// handleImpersonationStop — POST /api/impersonation/stop.
// Идемпотентно: если сессия не impersonation — 200 {"ok":true,"restored":false}.
func handleImpersonationStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	actorID, isImpersonating := actorIDFromCtx(r)
	if !isImpersonating {
		// noop: уже не в impersonation.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(impersonationStopResp{OK: true, Restored: false})
		return
	}

	actorRole, ok := actorRoleFromCtx(r)
	if !ok {
		// Теоретически не бывает (withActor кладёт обе), но защитимся.
		slog.Error("impersonation: stop, actor_role missing", "actor_id", actorID)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}

	targetID := doctorIDFromCtx(r)

	newTok, err := IssueAccessToken(actorID, actorRole)
	if err != nil {
		slog.Error("impersonation: restore access", "err", err, "actor_id", actorID)
		http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
		return
	}
	setAuthCookies(w, newTok, "")

	if db != nil {
		auditLog(db, actorID, "impersonation.stop", "doctor", targetID, clientIP(r), map[string]any{
			"actor_role": string(actorRole),
		})
	}

	slog.Info("impersonation: stop", "actor_id", actorID, "target_id", targetID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(impersonationStopResp{OK: true, Restored: true})
}

// handleImpersonationWhoami — GET /api/impersonation/whoami.
//
// Всегда 200 для валидной сессии.
//   - обычная сессия: {"impersonating": false}
//   - impersonation:  {"impersonating": true, actor_id, actor_role,
//     target_id, target_name, target_role}
//
// Нужен, чтобы UI в doctor_cabinet.html мог показать баннер:
// access-токен лежит в HttpOnly-cookie, JS его не читает.
func handleImpersonationWhoami(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	actorID, isImpersonating := actorIDFromCtx(r)
	resp := impersonationWhoamiResp{Impersonating: false}
	if isImpersonating {
		actorRole, _ := actorRoleFromCtx(r)
		targetID := doctorIDFromCtx(r)
		resp.Impersonating = true
		resp.ActorID = actorID
		resp.ActorRole = string(actorRole)
		resp.TargetID = targetID

		if db != nil && targetID != "" {
			if row, err := getDoctorRowByID(db, targetID); err == nil {
				resp.TargetName = row.Name
				resp.TargetRole = row.Role
			} else {
				slog.Warn("impersonation: whoami target lookup", "err", err, "target_id", targetID)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

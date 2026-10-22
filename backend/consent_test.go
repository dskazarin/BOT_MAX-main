package main

// consent_test.go — B30.3 (вариант B): тесты consent-контура.
//
// Стиль — httptest (как audit_test.go): собираем *http.Request с ролью
// в контексте через withDoctorID/withPatientID, вызываем хендлер напрямую.
//
// Покрытие:
//   - request   : doctor-only, 403 для пациента, 404 на несуществующего пациента,
//                 default scope = vitals_read
//   - approve   : patient-only, pending -> approved, 409 из не-pending
//   - deny      : patient-only, pending -> denied
//   - revoke    : обе стороны, approved -> revoked, 409 из pending
//   - re-request: UPSERT denied -> pending, approved_at/revoked_at = NULL (1a)
//   - forbidden : чужой consent -> 403
//   - audit     : запись в consents_audit после действия
//   - conflict  : transitionConsent -> ErrConsentConflict

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------
// Хелперы
// ---------------------------------------------------------------

// reqConsentRequest — POST /api/consents/request с ролью doctor.
func reqConsentRequest(doctorID, body string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodPost, "/api/consents/request", strings.NewReader(body)).WithContext(ctx)
}

// reqConsentTransitionPatient — POST /api/consents/{id}/{action} с ролью patient.
func reqConsentTransitionPatient(patientID, consentID, action string) *http.Request {
	ctx := withPatientID(context.Background(), patientID)
	return httptest.NewRequest(http.MethodPost, "/api/consents/"+consentID+"/"+action, nil).WithContext(ctx)
}

// reqConsentTransitionDoctor — то же, но с ролью doctor.
func reqConsentTransitionDoctor(doctorID, consentID, action string) *http.Request {
	ctx := withDoctorID(context.Background(), doctorID, RoleDoctor)
	return httptest.NewRequest(http.MethodPost, "/api/consents/"+consentID+"/"+action, nil).WithContext(ctx)
}

// setupConsentDB — подменяет пакетную db (см. main.go) на свежую :memory:,
// восстанавливая прежнее значение через t.Cleanup. Паттерн — 1:1 с
// doctors_test.go:TestHandleDoctorsList_RoleFilter.
func setupConsentDB(t *testing.T) {
	t.Helper()
	saved := db
	db = newTestDB(t)
	t.Cleanup(func() { db = saved })
}

// setupConsentEnv — пациент + врач + consent в заданном статусе.
//
//	Возвращает: (db, doctorID, patientID, consentID).
func setupConsentEnv(t *testing.T, status string) (*sql.DB, string, string, string) {
	t.Helper()
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test Patient")

	id, err := upsertConsentRequest(db, "PAT-1", "DOC-1", "vitals_read")
	if err != nil {
		t.Fatalf("setup upsertConsentRequest: %v", err)
	}

	switch status {
	case "pending":
		// уже pending
	case "approved":
		if err := transitionConsent(db, id, "pending", "approved"); err != nil {
			t.Fatalf("setup transition to approved: %v", err)
		}
	case "denied":
		if err := transitionConsent(db, id, "pending", "denied"); err != nil {
			t.Fatalf("setup transition to denied: %v", err)
		}
	case "revoked":
		if err := transitionConsent(db, id, "pending", "approved"); err != nil {
			t.Fatalf("setup transition to approved: %v", err)
		}
		if err := transitionConsent(db, id, "approved", "revoked"); err != nil {
			t.Fatalf("setup transition to revoked: %v", err)
		}
	default:
		t.Fatalf("unknown setup status: %q", status)
	}
	return db, "DOC-1", "PAT-1", id
}

// ---------------------------------------------------------------
// request
// ---------------------------------------------------------------

func TestConsentRequest_Doctor(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test")

	req := reqConsentRequest("DOC-1", `{"patientId":"PAT-1","scope":"vitals_read"}`)
	w := httptest.NewRecorder()
	handleConsentRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM consents WHERE patient_id=? AND doctor_id=?`, "PAT-1", "DOC-1").Scan(&status); err != nil {
		t.Fatalf("select consents: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
}

func TestConsentRequest_NotDoctor(t *testing.T) {
	setupConsentDB(t)
	seedPatient(t, db, "PAT-1", "Test")

	ctx := withPatientID(context.Background(), "PAT-1")
	req := httptest.NewRequest(http.MethodPost, "/api/consents/request",
		strings.NewReader(`{"patientId":"PAT-1"}`)).WithContext(ctx)
	w := httptest.NewRecorder()
	handleConsentRequest(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

func TestConsentRequest_PatientNotFound(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")

	req := reqConsentRequest("DOC-1", `{"patientId":"NOPE","scope":"vitals_read"}`)
	w := httptest.NewRecorder()
	handleConsentRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body=%s", w.Code, w.Body.String())
	}
}

func TestConsentRequest_DefaultScope(t *testing.T) {
	setupConsentDB(t)
	seedDoctorForRefresh(t, db, "DOC-1")
	seedPatient(t, db, "PAT-1", "Test")

	// scope в теле не задан — должен стать "vitals_read".
	req := reqConsentRequest("DOC-1", `{"patientId":"PAT-1"}`)
	w := httptest.NewRecorder()
	handleConsentRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var scope string
	if err := db.QueryRow(`SELECT scope FROM consents WHERE patient_id=?`, "PAT-1").Scan(&scope); err != nil {
		t.Fatalf("select scope: %v", err)
	}
	if scope != "vitals_read" {
		t.Errorf("scope = %q, want vitals_read", scope)
	}
}

// ---------------------------------------------------------------
// approve / deny / revoke
// ---------------------------------------------------------------

func TestConsentApprove_Patient(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "pending")

	req := reqConsentTransitionPatient("PAT-1", id, "approve")
	w := httptest.NewRecorder()
	handleConsentApprove(w, req, id)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var (
		status     string
		approvedAt sql.NullString
	)
	if err := db.QueryRow(`SELECT status, approved_at FROM consents WHERE id=?`, id).Scan(&status, &approvedAt); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "approved" {
		t.Errorf("status = %q, want approved", status)
	}
	if !approvedAt.Valid {
		t.Errorf("approved_at = NULL, want timestamp")
	}
}

func TestConsentDeny_Patient(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "pending")

	req := reqConsentTransitionPatient("PAT-1", id, "deny")
	w := httptest.NewRecorder()
	handleConsentDeny(w, req, id)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM consents WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "denied" {
		t.Errorf("status = %q, want denied", status)
	}
}

func TestConsentRevoke_FromApproved(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "approved")

	req := reqConsentTransitionPatient("PAT-1", id, "revoke")
	w := httptest.NewRecorder()
	handleConsentRevoke(w, req, id)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var (
		status    string
		revokedAt sql.NullString
	)
	if err := db.QueryRow(`SELECT status, revoked_at FROM consents WHERE id=?`, id).Scan(&status, &revokedAt); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "revoked" {
		t.Errorf("status = %q, want revoked", status)
	}
	if !revokedAt.Valid {
		t.Errorf("revoked_at = NULL, want timestamp")
	}
}

func TestConsentRevoke_FromPending_Conflict(t *testing.T) {
	_, _, _, id := setupConsentEnv(t, "pending")

	req := reqConsentTransitionPatient("PAT-1", id, "revoke")
	w := httptest.NewRecorder()
	handleConsentRevoke(w, req, id)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409, body=%s", w.Code, w.Body.String())
	}
}

func TestConsentRevoke_ByDoctor(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "approved")

	// Doctor тоже может revoke (решение 2a).
	req := reqConsentTransitionDoctor("DOC-1", id, "revoke")
	w := httptest.NewRecorder()
	handleConsentRevoke(w, req, id)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}
	var status string
	if err := db.QueryRow(`SELECT status FROM consents WHERE id=?`, id).Scan(&status); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "revoked" {
		t.Errorf("status = %q, want revoked", status)
	}
}

// ---------------------------------------------------------------
// re-request после deny (UPSERT, решение 1a)
// ---------------------------------------------------------------

func TestConsentRequest_ReRequestAfterDeny(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "denied")

	req := reqConsentRequest("DOC-1", `{"patientId":"PAT-1","scope":"vitals_read"}`)
	w := httptest.NewRecorder()
	handleConsentRequest(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", w.Code, w.Body.String())
	}

	var (
		status     string
		approvedAt sql.NullString
		revokedAt  sql.NullString
	)
	if err := db.QueryRow(
		`SELECT status, approved_at, revoked_at FROM consents WHERE id=?`, id,
	).Scan(&status, &approvedAt, &revokedAt); err != nil {
		t.Fatalf("select: %v", err)
	}
	if status != "pending" {
		t.Errorf("status = %q, want pending (re-request)", status)
	}
	if approvedAt.Valid {
		t.Errorf("approved_at должен быть NULL после re-request, got %v", approvedAt)
	}
	if revokedAt.Valid {
		t.Errorf("revoked_at должен быть NULL после re-request, got %v", revokedAt)
	}
}

// ---------------------------------------------------------------
// forbidden: чужой consent
// ---------------------------------------------------------------

func TestConsentApprove_Forbidden_OtherPatient(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "pending")
	// Второй пациент — свой, не PAT-1.
	seedPatient(t, db, "PAT-2", "Other")

	req := reqConsentTransitionPatient("PAT-2", id, "approve")
	w := httptest.NewRecorder()
	handleConsentApprove(w, req, id)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

// Нет аутентификации — 401, а не 403. 403 зарезервирован за «чужой consent»
// (см. TestConsentApprove_Forbidden_OtherPatient): 401 = не аутентифицирован,
// 403 = аутентифицирован, но не свой.
func TestConsentApprove_NoAuth_401(t *testing.T) {
	setupConsentEnv(t, "pending")

	// Запрос без withPatientID/withDoctorID — аналог «нет токена».
	req := httptest.NewRequest(http.MethodPost, "/api/consents/CONSENT-1/approve", nil)
	w := httptest.NewRecorder()
	handleConsentApprove(w, req, "CONSENT-1")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", w.Code, w.Body.String())
	}
}

func TestConsentRevoke_Forbidden_OtherDoctor(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "approved")
	// Второй врач — свой, не DOC-1.
	seedDoctorForRefresh(t, db, "DOC-2")

	req := reqConsentTransitionDoctor("DOC-2", id, "revoke")
	w := httptest.NewRecorder()
	handleConsentRevoke(w, req, id)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403, body=%s", w.Code, w.Body.String())
	}
}

// ---------------------------------------------------------------
// audit
// ---------------------------------------------------------------

func TestConsentAudit_Written(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "pending")

	req := reqConsentTransitionPatient("PAT-1", id, "approve")
	w := httptest.NewRecorder()
	handleConsentApprove(w, req, id)

	if w.Code != http.StatusOK {
		t.Fatalf("approve failed: %d", w.Code)
	}

	var action, actorType string
	if err := db.QueryRow(
		`SELECT action, actor_type FROM consents_audit WHERE consent_id=? ORDER BY created DESC LIMIT 1`, id,
	).Scan(&action, &actorType); err != nil {
		t.Fatalf("select consents_audit: %v", err)
	}
	if action != "approve" {
		t.Errorf("action = %q, want approve", action)
	}
	if actorType != "patient" {
		t.Errorf("actor_type = %q, want patient", actorType)
	}
}

// ---------------------------------------------------------------
// conflict sentinel
// ---------------------------------------------------------------

func TestTransitionConsent_ConflictErrSentinel(t *testing.T) {
	db, _, _, id := setupConsentEnv(t, "approved")

	// approved -> approved невозможно (from=pending), ждём ErrConsentConflict.
	err := transitionConsent(db, id, "pending", "approved")
	if !errors.Is(err, ErrConsentConflict) {
		t.Errorf("err = %v, want ErrConsentConflict", err)
	}
}

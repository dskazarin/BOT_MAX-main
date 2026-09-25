package main

// doctors.go — подпатч 6.14 (B5): admin CRUD по докторам.
//
// B5.1: только store-функции (list/get/create/update/delete).
// HTTP-хендлеры и роуты — в B5.2, guard requireRole — в B5.3.
//
// Модель:
//   * DELETE — soft (active=0), чтобы не рушить FK patients.doctor_id;
//   * PATCH — частичное обновление (указатель на поле = «не менять»);
//   * роль/email/password через PATCH НЕ меняем (отдельные эндпоинты в будущем).
//
// Замечание про тип Doctor: в db.go он МИНИМАЛЬНЫЙ (6 полей — только для
// login/refresh). Для CRUD нам нужен более широкий набор, поэтому
// заводим DoctorRow — НЕ трогая базовый Doctor (чтобы не сломать auth).

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// DoctorRow — расширенное представление врача для админ-CRUD.
// (Базовый Doctor в db.go — только ID/Email/PasswordHash/Name/Role/Active.)
type DoctorRow struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Specialty    string `json:"specialty"`
	Phone        string `json:"phone,omitempty"`
	ClinicID     string `json:"clinicId,omitempty"`
	Created      string `json:"created"`
	LastLogin    string `json:"lastLogin,omitempty"`
	Active       bool   `json:"active"`
	// PasswordHash — не отдаём наружу (json:"-"), но нужен для внутренних проверок.
	PasswordHash string `json:"-"`
}

// CreateDoctorInput — вход для createDoctor.
type CreateDoctorInput struct {
	Email     string
	Password  string
	Name      string
	Specialty string
	Role      Role
	Phone     string
}

// UpdateDoctorInput — вход для updateDoctor.
// nil-указатель = «не менять поле».
type UpdateDoctorInput struct {
	Name      *string
	Specialty *string
	Phone     *string
	Active    *bool
}

// ---------------------------------------------------------------
// Store
// ---------------------------------------------------------------

// listDoctors возвращает список врачей.
// includeInactive=false → только active=1.
// Сортировка: superadmin → admin → doctor → прочие, затем created DESC.
func listDoctors(db *sql.DB, includeInactive bool) ([]*DoctorRow, error) {
	q := `SELECT id, email, name, role, specialty, phone, clinic_id, created, last_login, active, password_hash
	      FROM doctors`
	if !includeInactive {
		q += ` WHERE active = 1`
	}
	q += ` ORDER BY
	          CASE role
	            WHEN 'superadmin' THEN 0
	            WHEN 'admin' THEN 1
	            WHEN 'doctor' THEN 2
	            ELSE 3
	          END,
	          created DESC`

	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("list doctors: %w", err)
	}
	defer rows.Close()

	var out []*DoctorRow
	for rows.Next() {
		d, err := scanDoctorRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list doctors rows: %w", err)
	}
	return out, nil
}

// getDoctorRowByID — полная запись по id.
func getDoctorRowByID(db *sql.DB, id string) (*DoctorRow, error) {
	const q = `SELECT id, email, name, role, specialty, phone, clinic_id, created, last_login, active, password_hash
	           FROM doctors WHERE id = ?`
	rows, err := db.Query(q, id)
	if err != nil {
		return nil, fmt.Errorf("get doctor row: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	return scanDoctorRow(rows)
}

// createDoctor создаёт нового врача с bcrypt-паролем.
func createDoctor(db *sql.DB, in CreateDoctorInput) (*DoctorRow, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	if in.Email == "" || in.Password == "" || in.Name == "" {
		return nil, fmt.Errorf("email, password, name — обязательны")
	}
	if in.Specialty == "" {
		in.Specialty = "lor"
	}
	if in.Role == "" {
		in.Role = RoleDoctor
	}
	if !in.Role.Valid() {
		return nil, fmt.Errorf("недопустимая роль %q", in.Role)
	}

	hash, err := HashPassword(in.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	id, err := newDoctorID()
	if err != nil {
		return nil, fmt.Errorf("new doctor id: %w", err)
	}

	_, err = db.Exec(
		`INSERT INTO doctors (id, clinic_id, email, password_hash, name, specialty, role, phone, created, active)
		 VALUES (?, NULL, ?, ?, ?, ?, ?, ?, datetime('now'), 1)`,
		id, in.Email, hash, in.Name, in.Specialty, string(in.Role), nullIfEmpty(in.Phone),
	)
	if err != nil {
		return nil, fmt.Errorf("insert doctor: %w", err)
	}

	return getDoctorRowByID(db, id)
}

// updateDoctor частично обновляет доктора.
func updateDoctor(db *sql.DB, id string, in UpdateDoctorInput) (*DoctorRow, error) {
	if id == "" {
		return nil, fmt.Errorf("empty doctor id")
	}

	sets := []string{}
	args := []any{}
	if in.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, strings.TrimSpace(*in.Name))
	}
	if in.Specialty != nil {
		sets = append(sets, "specialty = ?")
		args = append(args, strings.TrimSpace(*in.Specialty))
	}
	if in.Phone != nil {
		sets = append(sets, "phone = ?")
		args = append(args, nullIfEmpty(*in.Phone))
	}
	if in.Active != nil {
		sets = append(sets, "active = ?")
		if *in.Active {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	// Проверим существование — до UPDATE.
	if _, err := getDoctorRowByID(db, id); err != nil {
		return nil, err
	}

	if len(sets) > 0 {
		args = append(args, id)
		q := "UPDATE doctors SET " + strings.Join(sets, ", ") + " WHERE id = ?"
		if _, err := db.Exec(q, args...); err != nil {
			return nil, fmt.Errorf("update doctor: %w", err)
		}
	}

	return getDoctorRowByID(db, id)
}

// deleteDoctorByID — soft delete: active = 0.
// Идемпотентно: если уже active=0 — тоже nil.
// ErrNoRows, если такого id нет.
func deleteDoctorByID(db *sql.DB, id string) error {
	if id == "" {
		return fmt.Errorf("empty doctor id")
	}
	if _, err := getDoctorRowByID(db, id); err != nil {
		return err
	}
	_, err := db.Exec(`UPDATE doctors SET active = 0 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("soft delete doctor: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------
// Хелперы
// ---------------------------------------------------------------

// newDoctorID генерирует id вида "DOC-<unix>-<8 hex>".
// Не пересекается с DOC-001/ADMIN-001/SUPER-001 (seed).
func newDoctorID() (string, error) {
	suffix, err := newJTI()
	if err != nil {
		return "", err
	}
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	return fmt.Sprintf("DOC-%d-%s", time.Now().Unix(), suffix), nil
}

// nullIfEmpty — для NULL-able TEXT-колонок.
func nullIfEmpty(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

// scanDoctorRow — rows.Scan для DoctorRow.
func scanDoctorRow(rows *sql.Rows) (*DoctorRow, error) {
	var (
		d         DoctorRow
		phone     sql.NullString
		clinicID  sql.NullString
		created   sql.NullString
		lastLogin sql.NullString
		activeInt int
	)
	if err := rows.Scan(
		&d.ID, &d.Email, &d.Name, &d.Role, &d.Specialty, &phone, &clinicID,
		&created, &lastLogin, &activeInt, &d.PasswordHash,
	); err != nil {
		return nil, fmt.Errorf("scan doctor row: %w", err)
	}
	if phone.Valid {
		d.Phone = phone.String
	}
	if clinicID.Valid {
		d.ClinicID = clinicID.String
	}
	if created.Valid {
		d.Created = created.String
	}
	if lastLogin.Valid {
		d.LastLogin = lastLogin.String
	}
	d.Active = activeInt != 0
	return &d, nil
}

// ---------------------------------------------------------------
// HTTP — B5.2: handlers + registerDoctorRoutes
// ---------------------------------------------------------------
//
// Все роуты защищены requireRole(RoleAdmin, RoleSuperadmin).
// Обёртка requireRole — из auth.go; применяется так же, как в patients.go.
//
//   GET    /api/doctors          — список (по умолчанию только active)
//   POST   /api/doctors          — создать
//   GET    /api/doctors/{id}     — один
//   PATCH  /api/doctors/{id}     — частичное обновление
//   DELETE /api/doctors/{id}     — soft delete (active=0)

// registerDoctorRoutes регистрирует роуты /api/doctors/*.
// Вызывается из main() рядом с registerPatientRoutes() и registerAuthRoutes().
func registerDoctorRoutes() {
	guard := requireRole(RoleAdmin, RoleSuperadmin)

	http.Handle("/api/doctors", guard(http.HandlerFunc(handleDoctors)))
	http.Handle("/api/doctors/", guard(http.HandlerFunc(handleDoctorByID)))

	slog.Info("registerDoctorRoutes", "routes", "/api/doctors, /api/doctors/{id}")
}

// handleDoctors — /api/doctors (без /{id}).
//   GET  → список
//   POST → создать
func handleDoctors(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleDoctorsList(w, r)
	case http.MethodPost:
		handleDoctorCreate(w, r)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleDoctorsList — GET /api/doctors.
// Query: ?include_inactive=1 — включить неактивных.
func handleDoctorsList(w http.ResponseWriter, r *http.Request) {
	includeInactive := r.URL.Query().Get("include_inactive") == "1"

	docs, err := listDoctors(db, includeInactive)
	if err != nil {
		slog.Warn("GET /api/doctors", "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if docs == nil {
		docs = []*DoctorRow{}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(docs); err != nil {
		slog.Warn("GET /api/doctors encode", "err", err)
	}
}

// createDoctorReq — тело POST /api/doctors.
type createDoctorReq struct {
	Email     string `json:"email"`
	Password  string `json:"password"`
	Name      string `json:"name"`
	Specialty string `json:"specialty"`
	Role      string `json:"role"`
	Phone     string `json:"phone"`
}

// handleDoctorCreate — POST /api/doctors.
// 201 + созданный DoctorRow.
// 400 при пустых обязательных полях.
// 409 при дубликате email (UNIQUE).
func handleDoctorCreate(w http.ResponseWriter, r *http.Request) {
	var req createDoctorReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// B5.3: admin не может создавать admin/superadmin
	actorRole, _ := doctorRoleFromCtx(r)
	wantRole := Role(strings.ToLower(strings.TrimSpace(req.Role)))
	if wantRole == "" {
		wantRole = RoleDoctor
	}
	if actorRole != RoleSuperadmin && (wantRole == RoleAdmin || wantRole == RoleSuperadmin) {
		http.Error(w, `{"error":"only superadmin can create admin/superadmin"}`, http.StatusForbidden)
		return
	}

	in := CreateDoctorInput{
		Email:     req.Email,
		Password:  req.Password,
		Name:      req.Name,
		Specialty: req.Specialty,
		Role:      Role(strings.ToLower(strings.TrimSpace(req.Role))),
		Phone:     req.Phone,
	}

	doc, err := createDoctor(db, in)
	if err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "обязательны"),
			strings.Contains(msg, "недопустимая роль"):
			slog.Warn("POST /api/doctors", "err", err)
			http.Error(w, `{"error":"`+jsonEscape(msg)+`"}`, http.StatusBadRequest)
		case strings.Contains(msg, "UNIQUE"),
			strings.Contains(msg, "constraint failed"):
			slog.Warn("POST /api/doctors: конфликт email", "err", err)
			http.Error(w, `{"error":"email already exists"}`, http.StatusConflict)
		default:
			slog.Warn("POST /api/doctors", "err", err)
			http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		}
		return
	}

	// B9.3: audit
	auditLog(db, doctorIDFromCtx(r), "doctor.create", "doctor", doc.ID, clientIP(r), map[string]string{"email": doc.Email, "role": doc.Role})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(doc); err != nil {
		slog.Warn("POST /api/doctors encode", "err", err)
	}
}

// handleDoctorByID — /api/doctors/{id}.
//   GET    → один
//   PATCH  → частичное обновление
//   DELETE → soft delete
func handleDoctorByID(w http.ResponseWriter, r *http.Request) {
	id := extractPathID(r.URL.Path, "/api/doctors/")
	if id == "" {
		http.Error(w, `{"error":"missing id"}`, http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		handleDoctorGet(w, r, id)
	case http.MethodPatch:
		handleDoctorUpdate(w, r, id)
	case http.MethodDelete:
		handleDoctorDelete(w, r, id)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// handleDoctorGet — GET /api/doctors/{id}.
func handleDoctorGet(w http.ResponseWriter, r *http.Request, id string) {
	doc, err := getDoctorRowByID(db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		slog.Warn("GET /api/doctors/{id}", "id", id, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(doc); err != nil {
		slog.Warn("GET /api/doctors/{id} encode", "id", id, "err", err)
	}
}

// updateDoctorReq — тело PATCH /api/doctors/{id}.
// Все поля — указатели, чтобы отличать «не прислали» от «прислали пусто».
type updateDoctorReq struct {
	Name      *string `json:"name"`
	Specialty *string `json:"specialty"`
	Phone     *string `json:"phone"`
	Active    *bool   `json:"active"`
}

// handleDoctorUpdate — PATCH /api/doctors/{id}.
func handleDoctorUpdate(w http.ResponseWriter, r *http.Request, id string) {
	var req updateDoctorReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	// B5.3: бизнес-правила
	selfID, _ := resolveDoctorID(r)
	if id == selfID && req.Active != nil && !*req.Active {
		http.Error(w, `{"error":"cannot deactivate yourself"}`, http.StatusBadRequest)
		return
	}
	actorRole, _ := doctorRoleFromCtx(r)
	target, err := getDoctorRowByID(db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if target.Role == string(RoleSuperadmin) && actorRole != RoleSuperadmin {
		http.Error(w, `{"error":"only superadmin can modify superadmin"}`, http.StatusForbidden)
		return
	}

	in := UpdateDoctorInput{
		Name:      req.Name,
		Specialty: req.Specialty,
		Phone:     req.Phone,
		Active:    req.Active,
	}

	doc, err := updateDoctor(db, id, in)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		slog.Warn("PATCH /api/doctors/{id}", "id", id, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// B9.3: audit
	auditLog(db, doctorIDFromCtx(r), "doctor.update", "doctor", doc.ID, clientIP(r), nil)

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(doc); err != nil {
		slog.Warn("PATCH /api/doctors/{id} encode", "id", id, "err", err)
	}
}

// handleDoctorDelete — DELETE /api/doctors/{id} (soft delete).
func handleDoctorDelete(w http.ResponseWriter, r *http.Request, id string) {
	// B5.3: бизнес-правила
	selfID, _ := resolveDoctorID(r)
	if id == selfID {
		http.Error(w, `{"error":"cannot delete yourself"}`, http.StatusBadRequest)
		return
	}
	actorRole, _ := doctorRoleFromCtx(r)
	target, err := getDoctorRowByID(db, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}
	if target.Role == string(RoleSuperadmin) && actorRole != RoleSuperadmin {
		http.Error(w, `{"error":"only superadmin can modify superadmin"}`, http.StatusForbidden)
		return
	}

	if err = deleteDoctorByID(db, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		slog.Warn("DELETE /api/doctors/{id}", "id", id, "err", err)
		http.Error(w, `{"error":"internal"}`, http.StatusInternalServerError)
		return
	}

	// B9.3: audit
	auditLog(db, doctorIDFromCtx(r), "doctor.delete", "doctor", id, clientIP(r), nil)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"ok": true, "id": id})
}

// ---------------------------------------------------------------
// Хелперы B5.2
// ---------------------------------------------------------------

// extractPathID — вытащить id из пути (обрезает префикс).
// Пустой результат — значит «нет id».
func extractPathID(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(path, prefix))
}

// jsonEscape — грубая замена для безопасной вставки сообщения в JSON-строку.
// Достаточно для наших статических строк (не пропускает кавычки, \n).
func jsonEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

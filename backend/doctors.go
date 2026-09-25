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
	"fmt"
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

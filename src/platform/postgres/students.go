package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/students"
)

// StudentsStore implements students.Store on top of PostgreSQL.
type StudentsStore struct {
	db *DB
}

// NewStudentsStore returns a students.Store backed by db.
func NewStudentsStore(db *DB) *StudentsStore { return &StudentsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ students.Store = (*StudentsStore)(nil)

// studentColumns is the full column list used by every student SELECT.
const studentColumns = `id, names, surnames, class_id,
	document_id, phone, address, birthplace, birthdate, email,
	vision_has, vision_detail, hearing_has, hearing_detail,
	blood_type, is_new, previous_school, transfer_reason, repeat_count,
	mother_name, mother_document, mother_phone, mother_occupation, mother_address,
	father_name, father_document, father_phone, father_occupation, father_address,
	caregiver_name, caregiver_document, caregiver_phone, caregiver_occupation, caregiver_address,
	lives_with, siblings, medical_report, diversity_condition,
	specialist_has, specialist_detail, status`

// scanStudent maps the current row (in studentColumns order) onto a Student.
func scanStudent(scan func(dest ...any) error) (students.Student, error) {
	var st students.Student
	var birthdate *time.Time
	var siblingsJSON []byte
	var status string
	err := scan(
		&st.ID, &st.Names, &st.Surnames, &st.ClassID,
		&st.DocumentID, &st.Phone, &st.Address, &st.Birthplace, &birthdate, &st.Email,
		&st.Vision.Has, &st.Vision.Detail, &st.Hearing.Has, &st.Hearing.Detail,
		&st.BloodType, &st.IsNew, &st.PreviousSchool, &st.TransferReason, &st.RepeatCount,
		&st.Mother.Names, &st.Mother.DocumentID, &st.Mother.Phone, &st.Mother.Occupation, &st.Mother.Address,
		&st.Father.Names, &st.Father.DocumentID, &st.Father.Phone, &st.Father.Occupation, &st.Father.Address,
		&st.Caregiver.Names, &st.Caregiver.DocumentID, &st.Caregiver.Phone, &st.Caregiver.Occupation, &st.Caregiver.Address,
		&st.LivesWith, &siblingsJSON, &st.MedicalReport, &st.DiversityCondition,
		&st.SpecialistReport.Has, &st.SpecialistReport.Detail, &status,
	)
	if err != nil {
		return students.Student{}, err
	}
	st.Status = students.Status(status)
	if birthdate != nil {
		st.Birthdate = *birthdate
	}
	if len(siblingsJSON) > 0 {
		if err := json.Unmarshal(siblingsJSON, &st.Siblings); err != nil {
			return students.Student{}, fmt.Errorf("postgres: decode siblings: %w", err)
		}
	}
	return st, nil
}

// studentArgs flattens st into the INSERT/UPDATE argument order.
func studentArgs(st students.Student) []any {
	var birthdate *time.Time
	if !st.Birthdate.IsZero() {
		t := st.Birthdate
		birthdate = &t
	}
	siblingsJSON, _ := json.Marshal(st.Siblings)
	if len(siblingsJSON) == 0 {
		siblingsJSON = []byte("[]")
	}
	return []any{
		st.ID, st.Names, st.Surnames, st.ClassID,
		st.DocumentID, st.Phone, st.Address, st.Birthplace, birthdate, st.Email,
		st.Vision.Has, st.Vision.Detail, st.Hearing.Has, st.Hearing.Detail,
		st.BloodType, st.IsNew, st.PreviousSchool, st.TransferReason, st.RepeatCount,
		st.Mother.Names, st.Mother.DocumentID, st.Mother.Phone, st.Mother.Occupation, st.Mother.Address,
		st.Father.Names, st.Father.DocumentID, st.Father.Phone, st.Father.Occupation, st.Father.Address,
		st.Caregiver.Names, st.Caregiver.DocumentID, st.Caregiver.Phone, st.Caregiver.Occupation, st.Caregiver.Address,
		st.LivesWith, string(siblingsJSON), st.MedicalReport, st.DiversityCondition,
		st.SpecialistReport.Has, st.SpecialistReport.Detail, string(st.Status),
	}
}

// Create implements students.Store.
func (s *StudentsStore) Create(ctx context.Context, st students.Student) (students.Student, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO students (id, names, surnames, class_id,
			document_id, phone, address, birthplace, birthdate, email,
			vision_has, vision_detail, hearing_has, hearing_detail,
			blood_type, is_new, previous_school, transfer_reason, repeat_count,
			mother_name, mother_document, mother_phone, mother_occupation, mother_address,
			father_name, father_document, father_phone, father_occupation, father_address,
			caregiver_name, caregiver_document, caregiver_phone, caregiver_occupation, caregiver_address,
			lives_with, siblings, medical_report, diversity_condition,
			specialist_has, specialist_detail, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19,
			$20, $21, $22, $23, $24, $25, $26, $27, $28, $29,
			$30, $31, $32, $33, $34, $35, $36, $37, $38, $39, $40, $41)
		RETURNING `+studentColumns,
		studentArgs(st)...)
	out, err := scanStudent(row.Scan)
	if err != nil {
		return students.Student{}, fmt.Errorf("postgres: create student: %w", err)
	}
	return out, nil
}

// ByID implements students.Store.
func (s *StudentsStore) ByID(ctx context.Context, id string) (students.Student, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+studentColumns+` FROM students WHERE id = $1`, id)
	out, err := scanStudent(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return students.Student{}, students.ErrNotFound
	}
	if err != nil {
		return students.Student{}, fmt.Errorf("postgres: get student: %w", err)
	}
	return out, nil
}

// ListByClass implements students.Store.
func (s *StudentsStore) ListByClass(ctx context.Context, classID string) ([]students.Student, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+studentColumns+`
		FROM students
		WHERE class_id = $1
		ORDER BY lower(surnames), lower(names), id`,
		classID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list students: %w", err)
	}
	defer rows.Close()

	out := make([]students.Student, 0)
	for rows.Next() {
		st, err := scanStudent(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan student: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Update implements students.Store.
func (s *StudentsStore) Update(ctx context.Context, st students.Student) (students.Student, error) {
	args := studentArgs(st)
	row := s.db.pool.QueryRow(ctx, `
		UPDATE students
		SET names = $2, surnames = $3, class_id = $4,
			document_id = $5, phone = $6, address = $7, birthplace = $8, birthdate = $9, email = $10,
			vision_has = $11, vision_detail = $12, hearing_has = $13, hearing_detail = $14,
			blood_type = $15, is_new = $16, previous_school = $17, transfer_reason = $18, repeat_count = $19,
			mother_name = $20, mother_document = $21, mother_phone = $22, mother_occupation = $23, mother_address = $24,
			father_name = $25, father_document = $26, father_phone = $27, father_occupation = $28, father_address = $29,
			caregiver_name = $30, caregiver_document = $31, caregiver_phone = $32, caregiver_occupation = $33, caregiver_address = $34,
			lives_with = $35, siblings = $36, medical_report = $37, diversity_condition = $38,
			specialist_has = $39, specialist_detail = $40, status = $41, updated_at = now()
		WHERE id = $1
		RETURNING `+studentColumns,
		args...)
	out, err := scanStudent(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return students.Student{}, students.ErrNotFound
	}
	if err != nil {
		return students.Student{}, fmt.Errorf("postgres: update student: %w", err)
	}
	return out, nil
}

// Delete implements students.Store. Dependent records cascade.
func (s *StudentsStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM students WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete student: %w", err)
	}
	return nil
}

// PutPhoto implements students.Store.
func (s *StudentsStore) PutPhoto(ctx context.Context, studentID string, photo students.Photo) error {
	var data []byte
	if len(photo.Data) > 0 {
		data = photo.Data
	}
	tag, err := s.db.pool.Exec(ctx,
		`UPDATE students SET photo = $2, updated_at = now() WHERE id = $1`,
		studentID, data)
	if err != nil {
		return fmt.Errorf("postgres: put photo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return students.ErrNotFound
	}
	return nil
}

// Photo implements students.Store.
func (s *StudentsStore) Photo(ctx context.Context, studentID string) (students.Photo, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT photo FROM students WHERE id = $1`, studentID)
	var data []byte
	err := row.Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return students.Photo{}, students.ErrNotFound
	}
	if err != nil {
		return students.Photo{}, fmt.Errorf("postgres: get photo: %w", err)
	}
	return students.Photo{Data: data}, nil
}

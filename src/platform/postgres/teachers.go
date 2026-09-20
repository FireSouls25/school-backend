package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/teachers"
)

// TeachersStore implements teachers.Store on top of PostgreSQL.
type TeachersStore struct {
	db *DB
}

// NewTeachersStore returns a teachers.Store backed by db.
func NewTeachersStore(db *DB) *TeachersStore { return &TeachersStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ teachers.Store = (*TeachersStore)(nil)

// teacherColumns is the full column list used by every teacher SELECT.
const teacherColumns = `id, names, surnames, document_id, phone, address,
	birthplace, birthdate, email, medical_conditions, homeroom_class_id, active`

// scanTeacher maps the current row (in teacherColumns order) onto a Teacher.
func scanTeacher(scan func(dest ...any) error) (teachers.Teacher, error) {
	var t teachers.Teacher
	var birthdate *time.Time
	err := scan(
		&t.ID, &t.Names, &t.Surnames, &t.DocumentID, &t.Phone, &t.Address,
		&t.Birthplace, &birthdate, &t.Email, &t.MedicalConditions, &t.HomeroomClassID, &t.Active,
	)
	if err != nil {
		return teachers.Teacher{}, err
	}
	if birthdate != nil {
		t.Birthdate = *birthdate
	}
	return t, nil
}

// teacherArgs flattens t into the INSERT/UPDATE argument order.
func teacherArgs(t teachers.Teacher) []any {
	var birthdate *time.Time
	if !t.Birthdate.IsZero() {
		b := t.Birthdate
		birthdate = &b
	}
	return []any{
		t.ID, t.Names, t.Surnames, t.DocumentID, t.Phone, t.Address,
		t.Birthplace, birthdate, t.Email, t.MedicalConditions, t.HomeroomClassID, t.Active,
	}
}

// Create implements teachers.Store.
func (s *TeachersStore) Create(ctx context.Context, t teachers.Teacher) (teachers.Teacher, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO teachers (id, names, surnames, document_id, phone, address,
			birthplace, birthdate, email, medical_conditions, homeroom_class_id, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING `+teacherColumns,
		teacherArgs(t)...)
	out, err := scanTeacher(row.Scan)
	if err != nil {
		return teachers.Teacher{}, fmt.Errorf("postgres: create teacher: %w", err)
	}
	return out, nil
}

// ByID implements teachers.Store.
func (s *TeachersStore) ByID(ctx context.Context, id string) (teachers.Teacher, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+teacherColumns+` FROM teachers WHERE id = $1`, id)
	out, err := scanTeacher(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return teachers.Teacher{}, teachers.ErrNotFound
	}
	if err != nil {
		return teachers.Teacher{}, fmt.Errorf("postgres: get teacher: %w", err)
	}
	return out, nil
}

// List implements teachers.Store.
func (s *TeachersStore) List(ctx context.Context) ([]teachers.Teacher, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+teacherColumns+`
		FROM teachers
		ORDER BY lower(surnames), lower(names), id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list teachers: %w", err)
	}
	defer rows.Close()

	out := make([]teachers.Teacher, 0)
	for rows.Next() {
		t, err := scanTeacher(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan teacher: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Update implements teachers.Store.
func (s *TeachersStore) Update(ctx context.Context, t teachers.Teacher) (teachers.Teacher, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE teachers
		SET names = $2, surnames = $3, document_id = $4, phone = $5, address = $6,
			birthplace = $7, birthdate = $8, email = $9, medical_conditions = $10,
			homeroom_class_id = $11, active = $12, updated_at = now()
		WHERE id = $1
		RETURNING `+teacherColumns,
		teacherArgs(t)...)
	out, err := scanTeacher(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return teachers.Teacher{}, teachers.ErrNotFound
	}
	if err != nil {
		return teachers.Teacher{}, fmt.Errorf("postgres: update teacher: %w", err)
	}
	return out, nil
}

// Delete implements teachers.Store. Subject assignments cascade.
func (s *TeachersStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM teachers WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete teacher: %w", err)
	}
	return nil
}

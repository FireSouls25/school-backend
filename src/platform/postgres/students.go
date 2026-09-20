package postgres

import (
	"context"
	"errors"
	"fmt"

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

// Create implements students.Store.
func (s *StudentsStore) Create(ctx context.Context, st students.Student) (students.Student, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO students (id, names, surnames, class_id)
		VALUES ($1, $2, $3, $4)
		RETURNING id, names, surnames, class_id`,
		st.ID, st.Names, st.Surnames, st.ClassID)
	var out students.Student
	if err := row.Scan(&out.ID, &out.Names, &out.Surnames, &out.ClassID); err != nil {
		return students.Student{}, fmt.Errorf("postgres: create student: %w", err)
	}
	return out, nil
}

// ByID implements students.Store.
func (s *StudentsStore) ByID(ctx context.Context, id string) (students.Student, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT id, names, surnames, class_id FROM students WHERE id = $1`, id)
	var out students.Student
	err := row.Scan(&out.ID, &out.Names, &out.Surnames, &out.ClassID)
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
		SELECT id, names, surnames, class_id
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
		var st students.Student
		if err := rows.Scan(&st.ID, &st.Names, &st.Surnames, &st.ClassID); err != nil {
			return nil, fmt.Errorf("postgres: scan student: %w", err)
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// Update implements students.Store.
func (s *StudentsStore) Update(ctx context.Context, st students.Student) (students.Student, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE students
		SET names = $2, surnames = $3, class_id = $4, updated_at = now()
		WHERE id = $1
		RETURNING id, names, surnames, class_id`,
		st.ID, st.Names, st.Surnames, st.ClassID)
	var out students.Student
	err := row.Scan(&out.ID, &out.Names, &out.Surnames, &out.ClassID)
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

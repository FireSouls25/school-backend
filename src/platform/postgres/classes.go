package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"grade/src/core/classes"
)

// ClassesStore implements classes.Store on top of PostgreSQL.
type ClassesStore struct {
	db *DB
}

// NewClassesStore returns a classes.Store backed by db.
func NewClassesStore(db *DB) *ClassesStore { return &ClassesStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ classes.Store = (*ClassesStore)(nil)

// classColumns is the full column list used by every group SELECT.
const classColumns = `id, school_year_id, grade, group_no`

// scanClass maps the current row (in classColumns order) onto a ClassGroup.
func scanClass(scan func(dest ...any) error) (classes.ClassGroup, error) {
	var g classes.ClassGroup
	if err := scan(&g.ID, &g.SchoolYearID, &g.Grade, &g.GroupNo); err != nil {
		return classes.ClassGroup{}, err
	}
	return g, nil
}

// Create implements classes.Store.
func (s *ClassesStore) Create(ctx context.Context, g classes.ClassGroup) (classes.ClassGroup, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO class_groups (id, school_year_id, grade, group_no)
		VALUES ($1, $2, $3, $4)
		RETURNING `+classColumns,
		g.ID, g.SchoolYearID, g.Grade, g.GroupNo)
	out, err := scanClass(row.Scan)
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "school_year"):
			return classes.ClassGroup{}, classes.ErrInvalidSchoolYear
		case isUniqueViolationOn(err, "class_groups"):
			return classes.ClassGroup{}, classes.ErrDuplicateClass
		}
		return classes.ClassGroup{}, fmt.Errorf("postgres: create class: %w", err)
	}
	return out, nil
}

// ByID implements classes.Store.
func (s *ClassesStore) ByID(ctx context.Context, id string) (classes.ClassGroup, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+classColumns+` FROM class_groups WHERE id = $1`, id)
	out, err := scanClass(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return classes.ClassGroup{}, classes.ErrNotFound
	}
	if err != nil {
		return classes.ClassGroup{}, fmt.Errorf("postgres: get class: %w", err)
	}
	return out, nil
}

// ListByYear implements classes.Store.
func (s *ClassesStore) ListByYear(ctx context.Context, schoolYearID string) ([]classes.ClassGroup, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+classColumns+`
		FROM class_groups
		WHERE school_year_id = $1
		ORDER BY grade, group_no, id`,
		schoolYearID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list classes: %w", err)
	}
	defer rows.Close()

	out := make([]classes.ClassGroup, 0)
	for rows.Next() {
		g, err := scanClass(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan class: %w", err)
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// Update implements classes.Store.
func (s *ClassesStore) Update(ctx context.Context, g classes.ClassGroup) (classes.ClassGroup, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE class_groups
		SET school_year_id = $2, grade = $3, group_no = $4, updated_at = now()
		WHERE id = $1
		RETURNING `+classColumns,
		g.ID, g.SchoolYearID, g.Grade, g.GroupNo)
	out, err := scanClass(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return classes.ClassGroup{}, classes.ErrNotFound
	}
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "school_year"):
			return classes.ClassGroup{}, classes.ErrInvalidSchoolYear
		case isUniqueViolationOn(err, "class_groups"):
			return classes.ClassGroup{}, classes.ErrDuplicateClass
		}
		return classes.ClassGroup{}, fmt.Errorf("postgres: update class: %w", err)
	}
	return out, nil
}

// Delete implements classes.Store. Groups with enrollments are protected
// by the database.
func (s *ClassesStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM class_groups WHERE id = $1`, id); err != nil {
		if isForeignKeyViolationOn(err, "enrollments") {
			return classes.ErrHasEnrollments
		}
		return fmt.Errorf("postgres: delete class: %w", err)
	}
	return nil
}

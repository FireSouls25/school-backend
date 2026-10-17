package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/subjects"
)

// SubjectsStore implements subjects.Store on top of PostgreSQL.
type SubjectsStore struct {
	db *DB
}

// NewSubjectsStore returns a subjects.Store backed by db.
func NewSubjectsStore(db *DB) *SubjectsStore { return &SubjectsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ subjects.Store = (*SubjectsStore)(nil)

// subjectColumns is the full column list used by every subject SELECT.
const subjectColumns = `id, name, code, description, grades, color, active`

// gradesToPG converts domain grades to the INTEGER[] driver type.
func gradesToPG(grades []int) []int32 {
	out := make([]int32, 0, len(grades))
	for _, g := range grades {
		out = append(out, int32(g))
	}
	return out
}

// scanSubject maps the current row (in subjectColumns order) onto a Subject.
func scanSubject(scan func(dest ...any) error) (subjects.Subject, error) {
	var subj subjects.Subject
	var grades []int32
	err := scan(&subj.ID, &subj.Name, &subj.Code, &subj.Description, &grades, &subj.Color, &subj.Active)
	if err != nil {
		return subjects.Subject{}, err
	}
	subj.Grades = make([]int, 0, len(grades))
	for _, g := range grades {
		subj.Grades = append(subj.Grades, int(g))
	}
	return subj, nil
}

// assignmentColumns is the full column list used by every assignment SELECT.
const assignmentColumns = `id, teacher_id, subject_id, started_at, ended_at`

// scanAssignment maps the current row (in assignmentColumns order) onto
// an Assignment.
func scanAssignment(scan func(dest ...any) error) (subjects.Assignment, error) {
	var a subjects.Assignment
	var endedAt *time.Time
	err := scan(&a.ID, &a.TeacherID, &a.SubjectID, &a.StartedAt, &endedAt)
	if err != nil {
		return subjects.Assignment{}, err
	}
	if endedAt != nil {
		a.EndedAt = *endedAt
	}
	return a, nil
}

// CreateSubject implements subjects.Store.
func (s *SubjectsStore) CreateSubject(ctx context.Context, subj subjects.Subject) (subjects.Subject, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO subjects (id, name, code, description, grades, color, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+subjectColumns,
		subj.ID, subj.Name, subj.Code, subj.Description, gradesToPG(subj.Grades), subj.Color, subj.Active)
	out, err := scanSubject(row.Scan)
	if err != nil {
		if isUniqueViolationOn(err, "subjects") {
			return subjects.Subject{}, subjects.ErrDuplicateSubject
		}
		return subjects.Subject{}, fmt.Errorf("postgres: create subject: %w", err)
	}
	return out, nil
}

// SubjectByID implements subjects.Store.
func (s *SubjectsStore) SubjectByID(ctx context.Context, id string) (subjects.Subject, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+subjectColumns+` FROM subjects WHERE id = $1`, id)
	out, err := scanSubject(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return subjects.Subject{}, subjects.ErrNotFound
	}
	if err != nil {
		return subjects.Subject{}, fmt.Errorf("postgres: get subject: %w", err)
	}
	return out, nil
}

// ListSubjects implements subjects.Store.
func (s *SubjectsStore) ListSubjects(ctx context.Context) ([]subjects.Subject, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+subjectColumns+`
		FROM subjects
		ORDER BY lower(name), id`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list subjects: %w", err)
	}
	defer rows.Close()

	out := make([]subjects.Subject, 0)
	for rows.Next() {
		subj, err := scanSubject(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan subject: %w", err)
		}
		out = append(out, subj)
	}
	return out, rows.Err()
}

// UpdateSubject implements subjects.Store.
func (s *SubjectsStore) UpdateSubject(ctx context.Context, subj subjects.Subject) (subjects.Subject, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE subjects
		SET name = $2, code = $3, description = $4, grades = $5, color = $6, active = $7, updated_at = now()
		WHERE id = $1
		RETURNING `+subjectColumns,
		subj.ID, subj.Name, subj.Code, subj.Description, gradesToPG(subj.Grades), subj.Color, subj.Active)
	out, err := scanSubject(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return subjects.Subject{}, subjects.ErrNotFound
	}
	if err != nil {
		if isUniqueViolationOn(err, "subjects") {
			return subjects.Subject{}, subjects.ErrDuplicateSubject
		}
		return subjects.Subject{}, fmt.Errorf("postgres: update subject: %w", err)
	}
	return out, nil
}

// DeleteSubject implements subjects.Store.
func (s *SubjectsStore) DeleteSubject(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM subjects WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete subject: %w", err)
	}
	return nil
}

// AddAssignment implements subjects.Store.
func (s *SubjectsStore) AddAssignment(ctx context.Context, a subjects.Assignment) (subjects.Assignment, error) {
	var endedAt *time.Time
	if !a.EndedAt.IsZero() {
		e := a.EndedAt
		endedAt = &e
	}
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO subject_assignments (id, teacher_id, subject_id, started_at, ended_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+assignmentColumns,
		a.ID, a.TeacherID, a.SubjectID, a.StartedAt.Format("2006-01-02"), endedAt)
	out, err := scanAssignment(row.Scan)
	if err != nil {
		return subjects.Assignment{}, fmt.Errorf("postgres: add assignment: %w", err)
	}
	return out, nil
}

// AssignmentByID implements subjects.Store.
func (s *SubjectsStore) AssignmentByID(ctx context.Context, id string) (subjects.Assignment, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+assignmentColumns+` FROM subject_assignments WHERE id = $1`, id)
	out, err := scanAssignment(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return subjects.Assignment{}, subjects.ErrAssignmentNotFound
	}
	if err != nil {
		return subjects.Assignment{}, fmt.Errorf("postgres: get assignment: %w", err)
	}
	return out, nil
}

// AssignmentsForTeacher implements subjects.Store.
func (s *SubjectsStore) AssignmentsForTeacher(ctx context.Context, teacherID string) ([]subjects.Assignment, error) {
	return s.assignmentsWhere(ctx,
		`WHERE teacher_id = $1 ORDER BY started_at, id`, teacherID)
}

// AssignmentsForSubject implements subjects.Store.
func (s *SubjectsStore) AssignmentsForSubject(ctx context.Context, subjectID string) ([]subjects.Assignment, error) {
	return s.assignmentsWhere(ctx,
		`WHERE subject_id = $1 ORDER BY started_at, id`, subjectID)
}

func (s *SubjectsStore) assignmentsWhere(ctx context.Context, clause, arg string) ([]subjects.Assignment, error) {
	rows, err := s.db.pool.Query(ctx,
		`SELECT `+assignmentColumns+` FROM subject_assignments `+clause, arg)
	if err != nil {
		return nil, fmt.Errorf("postgres: list assignments: %w", err)
	}
	defer rows.Close()

	out := make([]subjects.Assignment, 0)
	for rows.Next() {
		a, err := scanAssignment(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan assignment: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// EndAssignment implements subjects.Store.
func (s *SubjectsStore) EndAssignment(ctx context.Context, id string, endedAt time.Time) (subjects.Assignment, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE subject_assignments
		SET ended_at = $2
		WHERE id = $1
		RETURNING `+assignmentColumns,
		id, endedAt.Format("2006-01-02"))
	out, err := scanAssignment(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return subjects.Assignment{}, subjects.ErrAssignmentNotFound
	}
	if err != nil {
		return subjects.Assignment{}, fmt.Errorf("postgres: end assignment: %w", err)
	}
	return out, nil
}

// RemoveAssignment implements subjects.Store.
func (s *SubjectsStore) RemoveAssignment(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM subject_assignments WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: remove assignment: %w", err)
	}
	return nil
}

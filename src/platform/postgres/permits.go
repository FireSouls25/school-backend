package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/permits"
)

// PermitsStore implements permits.Store on top of PostgreSQL.
type PermitsStore struct {
	db *DB
}

// NewPermitsStore returns a permits.Store backed by db.
func NewPermitsStore(db *DB) *PermitsStore { return &PermitsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ permits.Store = (*PermitsStore)(nil)

// permitColumns is the full column list used by every permit SELECT.
const permitColumns = `id, student_id, class_group_id, subject_id, day,
	reason, created_by, created_at`

// scanPermit maps the current row (in permitColumns order) onto a
// Permit.
func scanPermit(scan func(dest ...any) error) (permits.Permit, error) {
	var p permits.Permit
	var day time.Time
	err := scan(&p.ID, &p.StudentID, &p.ClassGroupID, &p.SubjectID, &day,
		&p.Reason, &p.CreatedBy, &p.CreatedAt)
	if err != nil {
		return permits.Permit{}, err
	}
	p.Date = permits.NormalizeDay(day)
	return p, nil
}

// permitDay renders a day as the DATE column value.
func permitDay(day time.Time) string { return day.Format("2006-01-02") }

// Create implements permits.Store.
func (s *PermitsStore) Create(ctx context.Context, p permits.Permit) (permits.Permit, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO attendance_permits (id, student_id, class_group_id,
			subject_id, day, reason, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+permitColumns,
		p.ID, p.StudentID, p.ClassGroupID, p.SubjectID, permitDay(p.Date),
		p.Reason, p.CreatedBy, p.CreatedAt)
	out, err := scanPermit(row.Scan)
	if err != nil {
		return permits.Permit{}, fmt.Errorf("postgres: create permit: %w", err)
	}
	return out, nil
}

// ByID implements permits.Store.
func (s *PermitsStore) ByID(ctx context.Context, id string) (permits.Permit, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+permitColumns+` FROM attendance_permits WHERE id = $1`, id)
	out, err := scanPermit(row.Scan)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return permits.Permit{}, permits.ErrNotFound
		}
		return permits.Permit{}, fmt.Errorf("postgres: fetch permit: %w", err)
	}
	return out, nil
}

// ForGroupOnDate implements permits.Store.
func (s *PermitsStore) ForGroupOnDate(ctx context.Context, classGroupID string, day time.Time) ([]permits.Permit, error) {
	return s.list(ctx, `
		SELECT `+permitColumns+`
		FROM attendance_permits
		WHERE class_group_id = $1 AND day = $2
		ORDER BY day DESC, id`, classGroupID, permitDay(day))
}

// ForStudentOnDate implements permits.Store.
func (s *PermitsStore) ForStudentOnDate(ctx context.Context, studentID string, day time.Time) ([]permits.Permit, error) {
	return s.list(ctx, `
		SELECT `+permitColumns+`
		FROM attendance_permits
		WHERE student_id = $1 AND day = $2
		ORDER BY day DESC, id`, studentID, permitDay(day))
}

// ForStudent implements permits.Store.
func (s *PermitsStore) ForStudent(ctx context.Context, studentID string) ([]permits.Permit, error) {
	return s.list(ctx, `
		SELECT `+permitColumns+`
		FROM attendance_permits
		WHERE student_id = $1
		ORDER BY day DESC, id`, studentID)
}

// list runs a SELECT and folds every row into a Permit.
func (s *PermitsStore) list(ctx context.Context, query string, args ...any) ([]permits.Permit, error) {
	rows, err := s.db.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list permits: %w", err)
	}
	defer rows.Close()
	out := make([]permits.Permit, 0)
	for rows.Next() {
		p, err := scanPermit(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan permit: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Delete implements permits.Store.
func (s *PermitsStore) Delete(ctx context.Context, id string) error {
	tag, err := s.db.pool.Exec(ctx,
		`DELETE FROM attendance_permits WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("postgres: delete permit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return permits.ErrNotFound
	}
	return nil
}

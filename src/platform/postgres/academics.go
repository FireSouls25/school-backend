package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/academics"
)

// AcademicsStore implements academics.Store on top of PostgreSQL.
type AcademicsStore struct {
	db *DB
}

// NewAcademicsStore returns an academics.Store backed by db.
func NewAcademicsStore(db *DB) *AcademicsStore { return &AcademicsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ academics.Store = (*AcademicsStore)(nil)

func scanAlert(row interface {
	Scan(dest ...any) error
}, into *academics.Alert) error {
	var status string
	var reviewedAt *time.Time
	var reviewedBy *string
	if err := row.Scan(
		&into.ID, &into.StudentID, &into.TeacherID, &into.Reason, &status,
		&into.CreatedAt, &reviewedAt, &reviewedBy,
	); err != nil {
		return err
	}
	into.Status = academics.Status(status)
	if reviewedAt != nil {
		into.ReviewedAt = *reviewedAt
	}
	if reviewedBy != nil {
		into.ReviewedBy = *reviewedBy
	}
	return nil
}

// Add implements academics.Store.
func (s *AcademicsStore) Add(ctx context.Context, a academics.Alert) (academics.Alert, error) {
	var out academics.Alert
	err := scanAlert(s.db.pool.QueryRow(ctx, `
		INSERT INTO academic_alerts (id, student_id, teacher_id, reason, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, student_id, teacher_id, reason, status, created_at, reviewed_at, reviewed_by`,
		a.ID, a.StudentID, a.TeacherID, a.Reason, string(a.Status), a.CreatedAt,
	), &out)
	if err != nil {
		return academics.Alert{}, fmt.Errorf("postgres: add academic alert: %w", err)
	}
	return out, nil
}

// ByID implements academics.Store.
func (s *AcademicsStore) ByID(ctx context.Context, id string) (academics.Alert, error) {
	var out academics.Alert
	err := scanAlert(s.db.pool.QueryRow(ctx, `
		SELECT id, student_id, teacher_id, reason, status, created_at, reviewed_at, reviewed_by
		FROM academic_alerts WHERE id = $1`, id,
	), &out)
	if errors.Is(err, pgx.ErrNoRows) {
		return academics.Alert{}, academics.ErrNotFound
	}
	if err != nil {
		return academics.Alert{}, fmt.Errorf("postgres: academic alert: %w", err)
	}
	return out, nil
}

// Open implements academics.Store.
func (s *AcademicsStore) Open(ctx context.Context, studentID string) ([]academics.Alert, error) {
	if studentID == "" {
		return s.alertsWhere(ctx, `WHERE status = 'open' ORDER BY created_at DESC, id DESC`)
	}
	return s.alertsWhere(ctx,
		`WHERE status = 'open' AND student_id = $1 ORDER BY created_at DESC, id DESC`, studentID)
}

// ForStudent implements academics.Store.
func (s *AcademicsStore) ForStudent(ctx context.Context, studentID string) ([]academics.Alert, error) {
	return s.alertsWhere(ctx,
		`WHERE student_id = $1 ORDER BY created_at DESC, id DESC`, studentID)
}

func (s *AcademicsStore) alertsWhere(ctx context.Context, clause string, args ...any) ([]academics.Alert, error) {
	rows, err := s.db.pool.Query(ctx,
		`SELECT id, student_id, teacher_id, reason, status, created_at, reviewed_at, reviewed_by
		FROM academic_alerts `+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list academic alerts: %w", err)
	}
	defer rows.Close()

	out := make([]academics.Alert, 0)
	for rows.Next() {
		var a academics.Alert
		if err := scanAlert(rows, &a); err != nil {
			return nil, fmt.Errorf("postgres: scan academic alert: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Update implements academics.Store.
func (s *AcademicsStore) Update(ctx context.Context, a academics.Alert) (academics.Alert, error) {
	var out academics.Alert
	err := scanAlert(s.db.pool.QueryRow(ctx, `
		UPDATE academic_alerts
		SET status = $2, reviewed_at = $3, reviewed_by = $4
		WHERE id = $1
		RETURNING id, student_id, teacher_id, reason, status, created_at, reviewed_at, reviewed_by`,
		a.ID, string(a.Status), a.ReviewedAt, a.ReviewedBy,
	), &out)
	if err != nil {
		return academics.Alert{}, fmt.Errorf("postgres: update academic alert: %w", err)
	}
	return out, nil
}

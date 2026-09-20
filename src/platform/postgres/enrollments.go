package postgres

import (
	"context"
	"fmt"
	"time"

	"grade/src/core/enrollments"
)

// EnrollmentsStore implements enrollments.Store on top of PostgreSQL.
type EnrollmentsStore struct {
	db *DB
}

// NewEnrollmentsStore returns an enrollments.Store backed by db.
func NewEnrollmentsStore(db *DB) *EnrollmentsStore { return &EnrollmentsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ enrollments.Store = (*EnrollmentsStore)(nil)

// enrollmentColumns is the full column list used by every enrollment SELECT.
const enrollmentColumns = `id, student_id, class_group_id`

// promotionColumns is the full column list used by every promotion SELECT.
const promotionColumns = `id, student_id, from_group_id, to_group_id,
	decision, decided_by, decided_at`

// scanPromotion maps the current row (in promotionColumns order) onto
// a Promotion.
func scanPromotion(scan func(dest ...any) error) (enrollments.Promotion, error) {
	var p enrollments.Promotion
	var toGroupID *string
	var decision string
	err := scan(&p.ID, &p.StudentID, &p.FromGroupID, &toGroupID,
		&decision, &p.DecidedBy, &p.DecidedAt)
	if err != nil {
		return enrollments.Promotion{}, err
	}
	p.Decision = enrollments.Decision(decision)
	if toGroupID != nil {
		p.ToGroupID = *toGroupID
	}
	return p, nil
}

// AddEnrollment implements enrollments.Store.
func (s *EnrollmentsStore) AddEnrollment(ctx context.Context, e enrollments.Enrollment) (enrollments.Enrollment, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO enrollments (id, student_id, class_group_id)
		VALUES ($1, $2, $3)
		RETURNING `+enrollmentColumns,
		e.ID, e.StudentID, e.ClassGroupID)
	var out enrollments.Enrollment
	if err := row.Scan(&out.ID, &out.StudentID, &out.ClassGroupID); err != nil {
		switch {
		case isForeignKeyViolationOn(err, "_student_id_"):
			return enrollments.Enrollment{}, enrollments.ErrInvalidStudent
		case isForeignKeyViolationOn(err, "_class_group_id_"):
			return enrollments.Enrollment{}, enrollments.ErrInvalidClassGroup
		case isUniqueViolationOn(err, "enrollments"):
			return enrollments.Enrollment{}, enrollments.ErrDuplicateEnrollment
		}
		return enrollments.Enrollment{}, fmt.Errorf("postgres: add enrollment: %w", err)
	}
	return out, nil
}

// EnrollmentsForGroup implements enrollments.Store.
func (s *EnrollmentsStore) EnrollmentsForGroup(ctx context.Context, classGroupID string) ([]enrollments.Enrollment, error) {
	return s.enrollmentsWhere(ctx,
		`WHERE class_group_id = $1 ORDER BY created_at, id`, classGroupID)
}

// EnrollmentsForStudent implements enrollments.Store.
func (s *EnrollmentsStore) EnrollmentsForStudent(ctx context.Context, studentID string) ([]enrollments.Enrollment, error) {
	return s.enrollmentsWhere(ctx,
		`WHERE student_id = $1 ORDER BY created_at, id`, studentID)
}

func (s *EnrollmentsStore) enrollmentsWhere(ctx context.Context, clause, arg string) ([]enrollments.Enrollment, error) {
	rows, err := s.db.pool.Query(ctx,
		`SELECT `+enrollmentColumns+` FROM enrollments `+clause, arg)
	if err != nil {
		return nil, fmt.Errorf("postgres: list enrollments: %w", err)
	}
	defer rows.Close()

	out := make([]enrollments.Enrollment, 0)
	for rows.Next() {
		var e enrollments.Enrollment
		if err := rows.Scan(&e.ID, &e.StudentID, &e.ClassGroupID); err != nil {
			return nil, fmt.Errorf("postgres: scan enrollment: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RemoveEnrollment implements enrollments.Store.
func (s *EnrollmentsStore) RemoveEnrollment(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM enrollments WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: remove enrollment: %w", err)
	}
	return nil
}

// RecordPromotion implements enrollments.Store.
func (s *EnrollmentsStore) RecordPromotion(ctx context.Context, p enrollments.Promotion) (enrollments.Promotion, error) {
	var toGroupID *string
	if p.ToGroupID != "" {
		toGroupID = &p.ToGroupID
	}
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO promotions (id, student_id, from_group_id, to_group_id,
			decision, decided_by, decided_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+promotionColumns,
		p.ID, p.StudentID, p.FromGroupID, toGroupID,
		string(p.Decision), p.DecidedBy, p.DecidedAt.Format(time.RFC3339))
	out, err := scanPromotion(row.Scan)
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "_student_id_"):
			return enrollments.Promotion{}, enrollments.ErrInvalidStudent
		case isForeignKeyViolationOn(err, "_class_group_id_"),
			isForeignKeyViolationOn(err, "_from_group_id_"),
			isForeignKeyViolationOn(err, "_to_group_id_"):
			return enrollments.Promotion{}, enrollments.ErrInvalidClassGroup
		}
		return enrollments.Promotion{}, fmt.Errorf("postgres: record promotion: %w", err)
	}
	return out, nil
}

// PromotionsForStudent implements enrollments.Store.
func (s *EnrollmentsStore) PromotionsForStudent(ctx context.Context, studentID string) ([]enrollments.Promotion, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+promotionColumns+`
		FROM promotions
		WHERE student_id = $1
		ORDER BY decided_at, id`,
		studentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list promotions: %w", err)
	}
	defer rows.Close()

	out := make([]enrollments.Promotion, 0)
	for rows.Next() {
		p, err := scanPromotion(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan promotion: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

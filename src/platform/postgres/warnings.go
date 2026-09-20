package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/warnings"
)

// WarningsStore implements warnings.Store on top of PostgreSQL.
type WarningsStore struct {
	db *DB
}

// NewWarningsStore returns a warnings.Store backed by db.
func NewWarningsStore(db *DB) *WarningsStore { return &WarningsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ warnings.Store = (*WarningsStore)(nil)

// warningColumns is the full column list used by every warning SELECT.
const warningColumns = `id, student_id, class_id, teacher_id, happened_at,
	gravity, title, description,
	snap_names, snap_surnames, snap_document_id, snap_class_id,
	snap_birthdate, snap_age, snap_caregiver_name, snap_caregiver_phone`

// scanWarning maps the current row (in warningColumns order) onto a Warning.
func scanWarning(scan func(dest ...any) error) (warnings.Warning, error) {
	var w warnings.Warning
	var gravity string
	var birthdate *time.Time
	err := scan(
		&w.ID, &w.StudentID, &w.ClassID, &w.TeacherID, &w.HappenedAt,
		&gravity, &w.Title, &w.Description,
		&w.Snapshot.Names, &w.Snapshot.Surnames, &w.Snapshot.DocumentID, &w.Snapshot.ClassID,
		&birthdate, &w.Snapshot.Age, &w.Snapshot.CaregiverName, &w.Snapshot.CaregiverPhone,
	)
	if err != nil {
		return warnings.Warning{}, err
	}
	w.Gravity = warnings.Gravity(gravity)
	if birthdate != nil {
		w.Snapshot.Birthdate = *birthdate
	}
	return w, nil
}

// warningArgs flattens w into the INSERT argument order.
func warningArgs(w warnings.Warning) []any {
	var birthdate *time.Time
	if !w.Snapshot.Birthdate.IsZero() {
		t := w.Snapshot.Birthdate
		birthdate = &t
	}
	return []any{
		w.ID, w.StudentID, w.ClassID, w.TeacherID, w.HappenedAt,
		string(w.Gravity), w.Title, w.Description,
		w.Snapshot.Names, w.Snapshot.Surnames, w.Snapshot.DocumentID, w.Snapshot.ClassID,
		birthdate, w.Snapshot.Age, w.Snapshot.CaregiverName, w.Snapshot.CaregiverPhone,
	}
}

// Add implements warnings.Store.
func (s *WarningsStore) Add(ctx context.Context, w warnings.Warning) (warnings.Warning, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO warnings (id, student_id, class_id, teacher_id, happened_at,
			gravity, title, description,
			snap_names, snap_surnames, snap_document_id, snap_class_id,
			snap_birthdate, snap_age, snap_caregiver_name, snap_caregiver_phone)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING `+warningColumns,
		warningArgs(w)...)
	out, err := scanWarning(row.Scan)
	if err != nil {
		return warnings.Warning{}, fmt.Errorf("postgres: add warning: %w", err)
	}
	return out, nil
}

// ForStudent implements warnings.Store.
func (s *WarningsStore) ForStudent(ctx context.Context, studentID string) ([]warnings.Warning, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+warningColumns+`
		FROM warnings
		WHERE student_id = $1
		ORDER BY happened_at DESC, id DESC`,
		studentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list warnings: %w", err)
	}
	defer rows.Close()

	out := make([]warnings.Warning, 0)
	for rows.Next() {
		w, err := scanWarning(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan warning: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// Delete implements warnings.Store.
func (s *WarningsStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx,
		`DELETE FROM warnings WHERE id = $1`, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("postgres: delete warning: %w", err)
	}
	return nil
}

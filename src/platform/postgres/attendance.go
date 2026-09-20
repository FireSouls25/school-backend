package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"grade/src/core/attendance"
)

// AttendanceStore implements attendance.Store on top of PostgreSQL.
type AttendanceStore struct {
	db *DB
}

// NewAttendanceStore returns an attendance.Store backed by db.
func NewAttendanceStore(db *DB) *AttendanceStore { return &AttendanceStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ attendance.Store = (*AttendanceStore)(nil)

// Add implements attendance.Store.
func (s *AttendanceStore) Add(ctx context.Context, r attendance.Record) (attendance.Record, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO attendance_records (id, student_id, class_id, date, reason)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, student_id, class_id, date, reason`,
		r.ID, r.StudentID, r.ClassID, r.Date.Format("2006-01-02"), string(r.Reason))
	var out attendance.Record
	var reason string
	if err := row.Scan(&out.ID, &out.StudentID, &out.ClassID, &out.Date, &reason); err != nil {
		return attendance.Record{}, fmt.Errorf("postgres: add attendance record: %w", err)
	}
	out.Reason = attendance.Reason(reason)
	return out, nil
}

// ForStudent implements attendance.Store.
func (s *AttendanceStore) ForStudent(ctx context.Context, studentID string) ([]attendance.Record, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id, student_id, class_id, date, reason
		FROM attendance_records
		WHERE student_id = $1
		ORDER BY date DESC, id DESC`,
		studentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list attendance: %w", err)
	}
	defer rows.Close()

	out := make([]attendance.Record, 0)
	for rows.Next() {
		var r attendance.Record
		var reason string
		if err := rows.Scan(&r.ID, &r.StudentID, &r.ClassID, &r.Date, &reason); err != nil {
			return nil, fmt.Errorf("postgres: scan attendance record: %w", err)
		}
		r.Reason = attendance.Reason(reason)
		out = append(out, r)
	}
	return out, rows.Err()
}

// Delete implements attendance.Store.
func (s *AttendanceStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx,
		`DELETE FROM attendance_records WHERE id = $1`, id); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("postgres: delete attendance record: %w", err)
	}
	return nil
}

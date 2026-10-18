package postgres

import (
	"context"
	"fmt"
	"time"

	"grade/src/core/tattendance"
)

// TAttendanceStore implements tattendance.Store on top of PostgreSQL.
type TAttendanceStore struct {
	db *DB
}

// NewTAttendanceStore returns a tattendance.Store backed by db.
func NewTAttendanceStore(db *DB) *TAttendanceStore { return &TAttendanceStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ tattendance.Store = (*TAttendanceStore)(nil)

const tattendanceColumns = `teacher_id, day, status, note, recorded_by, recorded_at`

func scanTAttendance(scan func(dest ...any) error) (tattendance.Mark, error) {
	var m tattendance.Mark
	var status string
	err := scan(&m.TeacherID, &m.Day, &status, &m.Note, &m.RecordedBy, &m.RecordedAt)
	if err != nil {
		return tattendance.Mark{}, err
	}
	m.Status = tattendance.Status(status)
	return m, nil
}

// Upsert implements tattendance.Store: recording the day again corrects it.
func (s *TAttendanceStore) Upsert(ctx context.Context, m tattendance.Mark) (tattendance.Mark, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO teacher_attendance (teacher_id, day, status, note, recorded_by, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (teacher_id, day)
		DO UPDATE SET status = EXCLUDED.status, note = EXCLUDED.note,
			recorded_by = EXCLUDED.recorded_by, recorded_at = EXCLUDED.recorded_at
		RETURNING `+tattendanceColumns,
		m.TeacherID, m.Day, string(m.Status), m.Note, m.RecordedBy, m.RecordedAt)
	out, err := scanTAttendance(row.Scan)
	if err != nil {
		return tattendance.Mark{}, fmt.Errorf("postgres: upsert teacher mark: %w", err)
	}
	return out, nil
}

// ForDay implements tattendance.Store.
func (s *TAttendanceStore) ForDay(ctx context.Context, day time.Time) ([]tattendance.Mark, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+tattendanceColumns+`
		FROM teacher_attendance
		WHERE day = $1
		ORDER BY teacher_id`,
		day)
	if err != nil {
		return nil, fmt.Errorf("postgres: list teacher day: %w", err)
	}
	defer rows.Close()

	out := make([]tattendance.Mark, 0)
	for rows.Next() {
		m, err := scanTAttendance(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan teacher mark: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ForTeacher implements tattendance.Store.
func (s *TAttendanceStore) ForTeacher(ctx context.Context, teacherID string, from, to time.Time) ([]tattendance.Mark, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+tattendanceColumns+`
		FROM teacher_attendance
		WHERE teacher_id = $1 AND day >= $2 AND day <= $3
		ORDER BY day DESC`,
		teacherID, from, to)
	if err != nil {
		return nil, fmt.Errorf("postgres: list teacher history: %w", err)
	}
	defer rows.Close()

	out := make([]tattendance.Mark, 0)
	for rows.Next() {
		m, err := scanTAttendance(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan teacher mark: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

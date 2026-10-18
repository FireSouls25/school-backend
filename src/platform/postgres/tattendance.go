package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

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

// AddRevision implements tattendance.Store.
func (s *TAttendanceStore) AddRevision(ctx context.Context, r tattendance.Revision) (tattendance.Revision, error) {
	if _, err := s.db.pool.Exec(ctx, `
		INSERT INTO teacher_attendance_revisions (id, teacher_id, day, old_status, new_status, note, recorded_by, recorded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		uuid.NewString(), r.TeacherID, r.Day, string(r.OldStatus), string(r.NewStatus),
		r.Note, r.RecordedBy, r.RecordedAt); err != nil {
		return tattendance.Revision{}, fmt.Errorf("postgres: add teacher revision: %w", err)
	}
	return r, nil
}

const tattendanceRevisionColumns = `teacher_id, day, old_status, new_status, note, recorded_by, recorded_at`

func scanTAttendanceRevision(scan func(dest ...any) error) (tattendance.Revision, error) {
	var r tattendance.Revision
	var oldStatus, newStatus string
	err := scan(&r.TeacherID, &r.Day, &oldStatus, &newStatus, &r.Note, &r.RecordedBy, &r.RecordedAt)
	if err != nil {
		return tattendance.Revision{}, err
	}
	r.OldStatus = tattendance.Status(oldStatus)
	r.NewStatus = tattendance.Status(newStatus)
	return r, nil
}

// RevisionsForTeacher implements tattendance.Store.
func (s *TAttendanceStore) RevisionsForTeacher(ctx context.Context, teacherID string, from, to time.Time) ([]tattendance.Revision, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+tattendanceRevisionColumns+`
		FROM teacher_attendance_revisions
		WHERE teacher_id = $1 AND day >= $2 AND day <= $3
		ORDER BY recorded_at DESC`,
		teacherID, from, to)
	if err != nil {
		return nil, fmt.Errorf("postgres: list teacher revisions: %w", err)
	}
	defer rows.Close()

	out := make([]tattendance.Revision, 0)
	for rows.Next() {
		r, err := scanTAttendanceRevision(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan teacher revision: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ForRange implements tattendance.Store.
func (s *TAttendanceStore) ForRange(ctx context.Context, from, to time.Time) ([]tattendance.Mark, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+tattendanceColumns+`
		FROM teacher_attendance
		WHERE day >= $1 AND day <= $2
		ORDER BY day DESC, teacher_id`,
		from, to)
	if err != nil {
		return nil, fmt.Errorf("postgres: list teacher range: %w", err)
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

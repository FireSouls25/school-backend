package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"grade/src/core/schedules"
)

// SchedulesStore implements schedules.Store on top of PostgreSQL.
type SchedulesStore struct {
	db *DB
}

// NewSchedulesStore returns a schedules.Store backed by db.
func NewSchedulesStore(db *DB) *SchedulesStore { return &SchedulesStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ schedules.Store = (*SchedulesStore)(nil)

// scheduleColumns is the full column list used by every entry SELECT.
const scheduleColumns = `id, class_group_id, teacher_id, subject_id,
	weekday, start_min, end_min`

// scanEntry maps the current row (in scheduleColumns order) onto an Entry.
func scanEntry(scan func(dest ...any) error) (schedules.Entry, error) {
	var e schedules.Entry
	var weekday int16
	var start, end int32
	err := scan(&e.ID, &e.ClassGroupID, &e.TeacherID, &e.SubjectID,
		&weekday, &start, &end)
	if err != nil {
		return schedules.Entry{}, err
	}
	e.Weekday = int(weekday)
	e.Start = schedules.Clock(start)
	e.End = schedules.Clock(end)
	return e, nil
}

// Create implements schedules.Store.
func (s *SchedulesStore) Create(ctx context.Context, e schedules.Entry) (schedules.Entry, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO schedule_entries (id, class_group_id, teacher_id, subject_id,
			weekday, start_min, end_min)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+scheduleColumns,
		e.ID, e.ClassGroupID, e.TeacherID, e.SubjectID,
		e.Weekday, int32(e.Start), int32(e.End))
	out, err := scanEntry(row.Scan)
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "_class_group_id_"):
			return schedules.Entry{}, schedules.ErrInvalidClassGroup
		case isForeignKeyViolationOn(err, "_teacher_id_"):
			return schedules.Entry{}, schedules.ErrInvalidTeacher
		case isForeignKeyViolationOn(err, "_subject_id_"):
			return schedules.Entry{}, schedules.ErrInvalidSubject
		}
		return schedules.Entry{}, fmt.Errorf("postgres: create schedule entry: %w", err)
	}
	return out, nil
}

// ByID implements schedules.Store.
func (s *SchedulesStore) ByID(ctx context.Context, id string) (schedules.Entry, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+scheduleColumns+` FROM schedule_entries WHERE id = $1`, id)
	out, err := scanEntry(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedules.Entry{}, schedules.ErrNotFound
	}
	if err != nil {
		return schedules.Entry{}, fmt.Errorf("postgres: get schedule entry: %w", err)
	}
	return out, nil
}

// EntriesForTeacher implements schedules.Store.
func (s *SchedulesStore) EntriesForTeacher(ctx context.Context, teacherID string) ([]schedules.Entry, error) {
	return s.entriesWhere(ctx,
		`WHERE teacher_id = $1 ORDER BY weekday, start_min, id`, teacherID)
}

// EntriesForGroup implements schedules.Store.
func (s *SchedulesStore) EntriesForGroup(ctx context.Context, classGroupID string) ([]schedules.Entry, error) {
	return s.entriesWhere(ctx,
		`WHERE class_group_id = $1 ORDER BY weekday, start_min, id`, classGroupID)
}

func (s *SchedulesStore) entriesWhere(ctx context.Context, clause, arg string) ([]schedules.Entry, error) {
	rows, err := s.db.pool.Query(ctx,
		`SELECT `+scheduleColumns+` FROM schedule_entries `+clause, arg)
	if err != nil {
		return nil, fmt.Errorf("postgres: list schedule entries: %w", err)
	}
	defer rows.Close()

	out := make([]schedules.Entry, 0)
	for rows.Next() {
		e, err := scanEntry(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan schedule entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Update implements schedules.Store.
func (s *SchedulesStore) Update(ctx context.Context, e schedules.Entry) (schedules.Entry, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE schedule_entries
		SET class_group_id = $2, teacher_id = $3, subject_id = $4,
			weekday = $5, start_min = $6, end_min = $7, updated_at = now()
		WHERE id = $1
		RETURNING `+scheduleColumns,
		e.ID, e.ClassGroupID, e.TeacherID, e.SubjectID,
		e.Weekday, int32(e.Start), int32(e.End))
	out, err := scanEntry(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return schedules.Entry{}, schedules.ErrNotFound
	}
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "_class_group_id_"):
			return schedules.Entry{}, schedules.ErrInvalidClassGroup
		case isForeignKeyViolationOn(err, "_teacher_id_"):
			return schedules.Entry{}, schedules.ErrInvalidTeacher
		case isForeignKeyViolationOn(err, "_subject_id_"):
			return schedules.Entry{}, schedules.ErrInvalidSubject
		}
		return schedules.Entry{}, fmt.Errorf("postgres: update schedule entry: %w", err)
	}
	return out, nil
}

// Delete implements schedules.Store.
func (s *SchedulesStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM schedule_entries WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete schedule entry: %w", err)
	}
	return nil
}

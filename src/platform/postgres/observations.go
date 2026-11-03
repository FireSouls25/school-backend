package postgres

import (
	"context"
	"time"

	"grade/src/core/observations"
)

// ObservationsStore implements observations.Store on top of PostgreSQL.
type ObservationsStore struct {
	db *DB
}

// NewObservationsStore returns an observations.Store backed by db.
func NewObservationsStore(db *DB) *ObservationsStore { return &ObservationsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ observations.Store = (*ObservationsStore)(nil)

// observationColumns is the full column list used by every observation SELECT.
const observationColumns = `id, student_id, snap_student_name, teacher_id,
	class_group_id, snap_class_label, situation_id, kind, note, happened_on`

// scanObservation maps the current row (in observationColumns order)
// onto an Observation.
func scanObservation(scan func(dest ...any) error) (observations.Observation, error) {
	var o observations.Observation
	var kind int16
	var day time.Time
	err := scan(&o.ID, &o.StudentID, &o.StudentName, &o.TeacherID,
		&o.ClassGroupID, &o.ClassLabel, &o.SituationID, &kind, &o.Note, &day)
	if err != nil {
		return observations.Observation{}, err
	}
	o.Kind = observations.Kind(kind)
	o.Date = day
	return o, nil
}

// Add implements observations.Store.
func (s *ObservationsStore) Add(ctx context.Context, o observations.Observation) (observations.Observation, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO observations (id, student_id, snap_student_name, teacher_id,
			class_group_id, snap_class_label, situation_id, kind, note, happened_on)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+observationColumns,
		o.ID, o.StudentID, o.StudentName, o.TeacherID,
		o.ClassGroupID, o.ClassLabel, o.SituationID, int16(o.Kind), o.Note, o.Date)
	out, err := scanObservation(row.Scan)
	if err != nil {
		return observations.Observation{}, err
	}
	return out, nil
}

// ForTeacher implements observations.Store.
func (s *ObservationsStore) ForTeacher(ctx context.Context, teacherID string) ([]observations.Observation, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+observationColumns+`
		FROM observations
		WHERE teacher_id = $1
		ORDER BY happened_on DESC, id`, teacherID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]observations.Observation, 0)
	for rows.Next() {
		o, err := scanObservation(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

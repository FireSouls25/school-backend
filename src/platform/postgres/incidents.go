package postgres

import (
	"context"
	"fmt"

	"grade/src/core/incidents"
)

// IncidentsStore implements incidents.Store on top of PostgreSQL.
type IncidentsStore struct {
	db *DB
}

// NewIncidentsStore returns an incidents.Store backed by db.
func NewIncidentsStore(db *DB) *IncidentsStore { return &IncidentsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ incidents.Store = (*IncidentsStore)(nil)

// Add implements incidents.Store.
func (s *IncidentsStore) Add(ctx context.Context, f incidents.Fault) (incidents.Fault, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO incident_faults (id, student_id, class_id, date, severity, description)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, student_id, class_id, date, severity, description`,
		f.ID, f.StudentID, f.ClassID, f.Date.Format("2006-01-02"), string(f.Severity), f.Description)
	var out incidents.Fault
	var severity string
	if err := row.Scan(&out.ID, &out.StudentID, &out.ClassID, &out.Date, &severity, &out.Description); err != nil {
		return incidents.Fault{}, fmt.Errorf("postgres: add fault: %w", err)
	}
	out.Severity = incidents.Severity(severity)
	return out, nil
}

// ForStudent implements incidents.Store.
func (s *IncidentsStore) ForStudent(ctx context.Context, studentID string) ([]incidents.Fault, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id, student_id, class_id, date, severity, description
		FROM incident_faults
		WHERE student_id = $1
		ORDER BY date DESC, id DESC`,
		studentID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list faults: %w", err)
	}
	defer rows.Close()

	out := make([]incidents.Fault, 0)
	for rows.Next() {
		var f incidents.Fault
		var severity string
		if err := rows.Scan(&f.ID, &f.StudentID, &f.ClassID, &f.Date, &severity, &f.Description); err != nil {
			return nil, fmt.Errorf("postgres: scan fault: %w", err)
		}
		f.Severity = incidents.Severity(severity)
		out = append(out, f)
	}
	return out, rows.Err()
}

// Delete implements incidents.Store.
func (s *IncidentsStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx,
		`DELETE FROM incident_faults WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete fault: %w", err)
	}
	return nil
}

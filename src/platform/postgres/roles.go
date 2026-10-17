package postgres

import (
	"context"
	"fmt"

	"grade/src/core/roles"
)

// RolesStore implements roles.Store on top of PostgreSQL, so grants
// survive restarts (the memory store they replace does not).
type RolesStore struct {
	db *DB
}

// NewRolesStore returns a roles.Store backed by db.
func NewRolesStore(db *DB) *RolesStore { return &RolesStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ roles.Store = (*RolesStore)(nil)

// AssignRole implements roles.Store: granting twice is a no-op.
func (s *RolesStore) AssignRole(ctx context.Context, subjectID string, role roles.Role) error {
	if _, err := s.db.pool.Exec(ctx, `
		INSERT INTO role_grants (subject_id, role)
		VALUES ($1, $2)
		ON CONFLICT (subject_id, role) DO NOTHING`,
		subjectID, string(role)); err != nil {
		return fmt.Errorf("postgres: assign role: %w", err)
	}
	return nil
}

// RemoveRole implements roles.Store: revoking absent grants is a no-op.
func (s *RolesStore) RemoveRole(ctx context.Context, subjectID string, role roles.Role) error {
	if _, err := s.db.pool.Exec(ctx, `
		DELETE FROM role_grants WHERE subject_id = $1 AND role = $2`,
		subjectID, string(role)); err != nil {
		return fmt.Errorf("postgres: remove role: %w", err)
	}
	return nil
}

// RolesFor implements roles.Store: unknown subjects hold nothing.
func (s *RolesStore) RolesFor(ctx context.Context, subjectID string) ([]roles.Role, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT role FROM role_grants WHERE subject_id = $1 ORDER BY role`,
		subjectID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list roles: %w", err)
	}
	defer rows.Close()

	out := make([]roles.Role, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("postgres: scan role: %w", err)
		}
		if r, err := roles.Parse(name); err == nil {
			out = append(out, r)
		}
	}
	return out, rows.Err()
}

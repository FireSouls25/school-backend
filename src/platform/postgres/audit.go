package postgres

import (
	"context"
	"time"

	"grade/src/core/audit"
)

// AuditStore implements audit.Store on top of PostgreSQL.
type AuditStore struct {
	db *DB
}

// NewAuditStore returns an audit.Store backed by db.
func NewAuditStore(db *DB) *AuditStore { return &AuditStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ audit.Store = (*AuditStore)(nil)

// auditColumns is the full column list used by every audit SELECT.
const auditColumns = `id, subject_id, action, resource, resource_id, status, happened_at`

func scanAudit(scan func(dest ...any) error) (audit.Entry, error) {
	var e audit.Entry
	var action, resource string
	var happened time.Time
	if err := scan(&e.ID, &e.SubjectID, &action, &resource, &e.ResourceID, &e.Status, &happened); err != nil {
		return audit.Entry{}, err
	}
	e.Action = audit.Action(action)
	e.Resource = audit.Resource(resource)
	e.HappenedAt = happened
	return e, nil
}

// Add implements audit.Store.
func (s *AuditStore) Add(ctx context.Context, e audit.Entry) (audit.Entry, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO audit_log (id, subject_id, action, resource, resource_id, status, happened_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+auditColumns,
		e.ID, e.SubjectID, string(e.Action), string(e.Resource), e.ResourceID, e.Status, e.HappenedAt)
	stored, err := scanAudit(row.Scan)
	if err != nil {
		return audit.Entry{}, err
	}
	return stored, nil
}

// ForStudent implements audit.Store.
func (s *AuditStore) ForStudent(ctx context.Context, studentID string) ([]audit.Entry, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+auditColumns+`
		FROM audit_log
		WHERE resource_id = $1
		ORDER BY happened_at DESC`, studentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]audit.Entry, 0)
	for rows.Next() {
		e, err := scanAudit(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Recent implements audit.Store.
func (s *AuditStore) Recent(ctx context.Context, limit int) ([]audit.Entry, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+auditColumns+`
		FROM audit_log
		ORDER BY happened_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]audit.Entry, 0)
	for rows.Next() {
		e, err := scanAudit(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

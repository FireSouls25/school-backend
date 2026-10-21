package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

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

// auditColumns is the full column list used by every trail SELECT.
const auditColumns = `id, subject_id, action, resource, resource_id, student_id, status, happened_at`

// scanAudit maps the current row (in auditColumns order) onto an Entry.
func scanAudit(scan func(dest ...any) error) (audit.Entry, error) {
	var e audit.Entry
	var action, resource string
	var resourceID *string
	var studentID *string
	var happenedAt time.Time
	err := scan(&e.ID, &e.SubjectID, &action, &resource, &resourceID, &studentID, &e.Status, &happenedAt)
	if err != nil {
		return audit.Entry{}, err
	}
	e.Action = audit.Action(action)
	e.Resource = audit.Resource(resource)
	if resourceID != nil {
		e.ResourceID = *resourceID
	}
	if studentID != nil {
		e.StudentID = *studentID
	}
	e.HappenedAt = happenedAt
	return e, nil
}

// studentIDArg maps the empty student link to a NULL UUID. pgtype is
// required: plain nil encodes as empty string, rejected by UUID columns.
func studentIDArg(studentID string) any {
	if studentID == "" {
		return pgtype.UUID{Valid: false}
	}
	return studentID
}

// uuidOrNull maps "" to NULL for nullable UUID columns (resource ids of
// failed writes, which have nothing to point at yet).
func uuidOrNull(id string) any {
	if id == "" {
		return pgtype.UUID{Valid: false}
	}
	return id
}

// Record implements audit.Store.
func (s *AuditStore) Record(ctx context.Context, e audit.Entry) (audit.Entry, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO audit_entries (id, subject_id, action, resource, resource_id, student_id, status, happened_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+auditColumns,
		e.ID, e.SubjectID, string(e.Action), string(e.Resource),
		uuidOrNull(e.ResourceID), studentIDArg(e.StudentID), e.Status, e.HappenedAt)
	out, err := scanAudit(row.Scan)
	if err != nil {
		return audit.Entry{}, fmt.Errorf("postgres: record audit: %w", err)
	}
	return out, nil
}

// List implements audit.Store: newest first, honoring limit and the
// optional student filter.
func (s *AuditStore) List(ctx context.Context, f audit.Filter) ([]audit.Entry, error) {
	clause := `ORDER BY happened_at DESC, id DESC LIMIT $1`
	args := []any{f.Limit}
	if f.StudentID != "" {
		clause = `WHERE student_id = $2 ` + clause
		args = append(args, f.StudentID)
	}
	rows, err := s.db.pool.Query(ctx,
		`SELECT `+auditColumns+` FROM audit_entries `+clause, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres: list audit: %w", err)
	}
	defer rows.Close()

	out := make([]audit.Entry, 0)
	for rows.Next() {
		e, err := scanAudit(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan audit: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

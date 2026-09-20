package postgres

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// pgErrorCode extracts the SQLSTATE code from a PostgreSQL error.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

// pgConstraint extracts the constraint or index name from a PostgreSQL error.
func pgConstraint(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.ConstraintName
	}
	return ""
}

// isUniqueViolationOn reports whether err violates a unique constraint
// whose name contains substr (table or index name fragment).
func isUniqueViolationOn(err error, substr string) bool {
	return pgErrorCode(err) == "23505" && strings.Contains(pgConstraint(err), substr)
}

// isForeignKeyViolationOn reports whether err violates a foreign key whose
// constraint name contains substr (column or table name fragment, e.g.
// "_student_id_" or "class_groups").
func isForeignKeyViolationOn(err error, substr string) bool {
	return pgErrorCode(err) == "23503" && strings.Contains(pgConstraint(err), substr)
}

// isForeignKeyViolation reports whether err is any foreign-key violation,
// used when every child table means the same protection.
func isForeignKeyViolation(err error) bool {
	return pgErrorCode(err) == "23503"
}

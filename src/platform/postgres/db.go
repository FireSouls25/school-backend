// Package postgres provides PostgreSQL adapters for the Store ports defined
// by the core capabilities. It is the production persistence layer, wired
// from the composition root.
package postgres

import (
	"context"
	"embed"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed schema.sql
var schemaFS embed.FS

// DB wraps a connection pool and owns the applied schema.
type DB struct {
	pool *pgxpool.Pool
}

// Connect opens a pool for dsn and applies the idempotent schema.
func Connect(ctx context.Context, dsn string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}
	if _, err := pool.Exec(ctx, string(mustSchema())); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: apply schema: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Close releases all pool resources.
func (db *DB) Close() { db.pool.Close() }

// Pool exposes the underlying pool to the adapters in this package.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

// Truncate empties tables (CASCADE) so integration tests start isolated.
// Identifiers are quoted; table names must be trusted constants, never
// user input.
func Truncate(ctx context.Context, db *DB, tables []string) error {
	if len(tables) == 0 {
		return nil
	}
	quoted := make([]string, len(tables))
	for i, t := range tables {
		quoted[i] = `"` + strings.ReplaceAll(t, `"`, `""`) + `"`
	}
	if _, err := db.pool.Exec(ctx, `TRUNCATE `+strings.Join(quoted, ", ")+` CASCADE`); err != nil {
		return fmt.Errorf("postgres: truncate: %w", err)
	}
	return nil
}

func mustSchema() []byte {
	b, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		panic(fmt.Sprintf("postgres: read embedded schema: %v", err))
	}
	return b
}

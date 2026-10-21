package postgres

import (
	"context"
	"fmt"
	"time"

	"grade/src/core/throttle"
)

// ThrottleStore implements throttle.Store on top of PostgreSQL.
type ThrottleStore struct {
	db *DB
}

// NewThrottleStore returns a throttle.Store backed by db.
func NewThrottleStore(db *DB) *ThrottleStore { return &ThrottleStore{db: db} }

// Compile-time check.
var _ throttle.Store = (*ThrottleStore)(nil)

// CountSince implements throttle.Store.
func (s *ThrottleStore) CountSince(ctx context.Context, key string, cut time.Time) (int, error) {
	var n int
	if err := s.db.pool.QueryRow(ctx,
		`SELECT count(*) FROM login_attempts WHERE bucket = $1 AND at > $2`, key, cut).Scan(&n); err != nil {
		return 0, fmt.Errorf("postgres: count login attempts: %w", err)
	}
	return n, nil
}

// Record implements throttle.Store.
func (s *ThrottleStore) Record(ctx context.Context, a throttle.Attempt) error {
	at := a.At
	if at.IsZero() {
		at = time.Now()
	}
	_, err := s.db.pool.Exec(ctx,
		`INSERT INTO login_attempts (bucket, ip, success, at) VALUES ($1, $2, $3, $4)`,
		a.Key, a.IP, a.Success, at)
	if err != nil {
		return fmt.Errorf("postgres: record login attempt: %w", err)
	}
	return nil
}

// DeleteBefore implements throttle.Store.
func (s *ThrottleStore) DeleteBefore(ctx context.Context, cut time.Time) error {
	_, err := s.db.pool.Exec(ctx, `DELETE FROM login_attempts WHERE at <= $1`, cut)
	if err != nil {
		return fmt.Errorf("postgres: purge login attempts: %w", err)
	}
	return nil
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/users"
)

// RecoveryStore implements users.RecoveryStore on top of PostgreSQL.
type RecoveryStore struct {
	db *DB
}

// NewRecoveryStore returns a users.RecoveryStore backed by db.
func NewRecoveryStore(db *DB) *RecoveryStore { return &RecoveryStore{db: db} }

// Compile-time check.
var _ users.RecoveryStore = (*RecoveryStore)(nil)

// Save implements users.RecoveryStore.
func (s *RecoveryStore) Save(ctx context.Context, rec users.RecoveryRec) error {
	_, err := s.db.pool.Exec(ctx, `
		INSERT INTO password_resets (id, user_id, token_hash, expires_at, used_at, created_at)
		VALUES ($1, $2, $3, $4, NULL, $5)`,
		rec.ID, rec.UserID, rec.TokenHash, rec.ExpiresAt, rec.CreatedAt)
	if err != nil {
		if isUniqueViolationOn(err, "password_resets_pkey") || isUniqueViolationOn(err, "password_resets_token_hash") {
			return users.ErrInvalidRecoveryToken
		}
		return fmt.Errorf("postgres: save password reset: %w", err)
	}
	return nil
}

// LookupByHash implements users.RecoveryStore.
func (s *RecoveryStore) LookupByHash(ctx context.Context, hash string) (users.RecoveryRec, error) {
	row := s.db.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, used_at, created_at
		FROM password_resets WHERE token_hash = $1`, hash)
	var rec users.RecoveryRec
	if err := row.Scan(&rec.ID, &rec.UserID, &rec.TokenHash, &rec.ExpiresAt, &rec.UsedAt, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return users.RecoveryRec{}, users.ErrNotFound
		}
		return users.RecoveryRec{}, fmt.Errorf("postgres: lookup password reset: %w", err)
	}
	return rec, nil
}

// MarkUsed implements users.RecoveryStore.
func (s *RecoveryStore) MarkUsed(ctx context.Context, id string) error {
	tag, err := s.db.pool.Exec(ctx,
		`UPDATE password_resets SET used_at = now() WHERE id = $1 AND used_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("postgres: consume password reset: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return users.ErrNotFound
	}
	return nil
}

// DeleteExpired implements users.RecoveryStore.
func (s *RecoveryStore) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.pool.Exec(ctx,
		`DELETE FROM password_resets WHERE used_at IS NOT NULL OR expires_at <= $1`, now)
	if err != nil {
		return fmt.Errorf("postgres: purge password resets: %w", err)
	}
	return nil
}

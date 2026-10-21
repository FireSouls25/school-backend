package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/auth"
)

// RefreshStore implements auth.Store on top of PostgreSQL.
type RefreshStore struct {
	db *DB
}

// NewRefreshStore returns an auth.Store backed by db.
func NewRefreshStore(db *DB) *RefreshStore { return &RefreshStore{db: db} }

// Compile-time check.
var _ auth.Store = (*RefreshStore)(nil)

// Save implements auth.Store.
func (s *RefreshStore) Save(ctx context.Context, rec auth.RefreshRec) error {
	_, err := s.db.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (jti, user_id, token_hash, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, NULL, $5)`,
		rec.JTI, rec.UserID, rec.TokenHash, rec.ExpiresAt, rec.CreatedAt)
	if err != nil {
		if isUniqueViolationOn(err, "refresh_tokens_pkey") {
			return auth.ErrInvalidToken
		}
		return fmt.Errorf("postgres: save refresh token: %w", err)
	}
	return nil
}

// Lookup implements auth.Store.
func (s *RefreshStore) Lookup(ctx context.Context, jti string) (auth.RefreshRec, error) {
	row := s.db.pool.QueryRow(ctx, `
		SELECT jti, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_tokens WHERE jti = $1`, jti)
	var rec auth.RefreshRec
	if err := row.Scan(&rec.JTI, &rec.UserID, &rec.TokenHash, &rec.ExpiresAt, &rec.RevokedAt, &rec.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.RefreshRec{}, auth.ErrInvalidToken
		}
		return auth.RefreshRec{}, fmt.Errorf("postgres: lookup refresh token: %w", err)
	}
	return rec, nil
}

// Revoke implements auth.Store.
func (s *RefreshStore) Revoke(ctx context.Context, jti string) error {
	_, err := s.db.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE jti = $1 AND revoked_at IS NULL`, jti)
	if err != nil {
		return fmt.Errorf("postgres: revoke refresh token: %w", err)
	}
	return nil
}

// RevokeAllForUser implements auth.Store.
func (s *RefreshStore) RevokeAllForUser(ctx context.Context, userID string) error {
	_, err := s.db.pool.Exec(ctx,
		`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	if err != nil {
		return fmt.Errorf("postgres: revoke user refresh tokens: %w", err)
	}
	return nil
}

// DeleteExpired implements auth.Store.
func (s *RefreshStore) DeleteExpired(ctx context.Context, now time.Time) error {
	_, err := s.db.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at <= $1`, now)
	if err != nil {
		return fmt.Errorf("postgres: delete expired refresh tokens: %w", err)
	}
	return nil
}

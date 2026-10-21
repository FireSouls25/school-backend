package auth

import (
	"context"
	"sync"
	"time"
)

// RefreshRec is one stored refresh token: only its hash persists, never
// the token itself. A DB read never yields a usable bearer.
type RefreshRec struct {
	JTI       string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Store is the persistence port for refresh tokens. Implementations must
// be safe for concurrent use.
type Store interface {
	// Save persists a fresh refresh record. JTI is unique; saving the
	// same JTI twice reports an error.
	Save(ctx context.Context, rec RefreshRec) error
	// Lookup returns the record for jti or ErrInvalidToken when absent.
	Lookup(ctx context.Context, jti string) (RefreshRec, error)
	// Revoke marks one refresh token revoked (logout, rotation).
	// Revoking an unknown JTI is a no-op; revoking twice stays revoked.
	Revoke(ctx context.Context, jti string) error
	// RevokeAllForUser marks every refresh token of the user revoked
	// (password rotation, disable). Users with no tokens succeed.
	RevokeAllForUser(ctx context.Context, userID string) error
	// DeleteExpired removes rows past expiry to keep the table small.
	DeleteExpired(ctx context.Context, now time.Time) error
}

// MemoryStore is an in-memory Store for tests and development.
type MemoryStore struct {
	mu   sync.Mutex
	rows map[string]RefreshRec
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rows: make(map[string]RefreshRec)}
}

// Compile-time check.
var _ Store = (*MemoryStore)(nil)

// Save implements Store.
func (s *MemoryStore) Save(_ context.Context, rec RefreshRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rows[rec.JTI]; ok {
		return ErrInvalidToken
	}
	s.rows[rec.JTI] = rec
	return nil
}

// Lookup implements Store.
func (s *MemoryStore) Lookup(_ context.Context, jti string) (RefreshRec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[jti]
	if !ok {
		return RefreshRec{}, ErrInvalidToken
	}
	return rec, nil
}

// Revoke implements Store.
func (s *MemoryStore) Revoke(_ context.Context, jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.rows[jti]
	if !ok {
		return nil
	}
	if rec.RevokedAt == nil {
		now := time.Now()
		rec.RevokedAt = &now
		s.rows[jti] = rec
	}
	return nil
}

// RevokeAllForUser implements Store.
func (s *MemoryStore) RevokeAllForUser(_ context.Context, userID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for jti, rec := range s.rows {
		if rec.UserID == userID && rec.RevokedAt == nil {
			rec.RevokedAt = &now
			s.rows[jti] = rec
		}
	}
	return nil
}

// DeleteExpired implements Store.
func (s *MemoryStore) DeleteExpired(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for jti, rec := range s.rows {
		if !rec.ExpiresAt.After(now) {
			delete(s.rows, jti)
		}
	}
	return nil
}

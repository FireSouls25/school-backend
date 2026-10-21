// Package users password recovery: single-use reset tokens for logins
// whose password is lost. Request never reveals whether the identifier
// exists (always 204); reset consumes the token once within its TTL.
// Delivery of the token (email/WhatsApp) is a future integration: the
// service returns the raw token to the caller and the HTTP layer logs it
// in development only, never in production.
package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultRecoveryTTL bounds a reset token: long enough to open the mail
// or message, short enough to limit replay.
const DefaultRecoveryTTL = time.Hour

// RecoveryRec is one reset token. Only its hash persists, never the raw
// token: a DB read yields nothing usable.
type RecoveryRec struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// RecoveryStore is the persistence port for reset tokens. Implementations
// must be safe for concurrent use.
type RecoveryStore interface {
	// Save persists a fresh record. TokenHash is unique; saving the same
	// hash twice reports an error.
	Save(ctx context.Context, rec RecoveryRec) error
	// LookupByHash returns the record for a token hash or ErrNotFound.
	LookupByHash(ctx context.Context, hash string) (RecoveryRec, error)
	// MarkUsed stamps a record consumed. Consuming twice stays consumed.
	MarkUsed(ctx context.Context, id string) error
	// DeleteExpired purges rows past expiry (plus used ones) to keep the
	// table small.
	DeleteExpired(ctx context.Context, now time.Time) error
}

// MemoryRecoveryStore is an in-memory RecoveryStore for tests.
type MemoryRecoveryStore struct {
	mu     sync.Mutex
	byID   map[string]RecoveryRec
	byHash map[string]string
}

// NewMemoryRecoveryStore returns an empty, ready-to-use store.
func NewMemoryRecoveryStore() *MemoryRecoveryStore {
	return &MemoryRecoveryStore{byID: make(map[string]RecoveryRec), byHash: make(map[string]string)}
}

// Compile-time check.
var _ RecoveryStore = (*MemoryRecoveryStore)(nil)

// Save implements RecoveryStore.
func (s *MemoryRecoveryStore) Save(_ context.Context, rec RecoveryRec) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byHash[rec.TokenHash]; ok {
		return ErrDuplicateEmail
	}
	s.byID[rec.ID] = rec
	s.byHash[rec.TokenHash] = rec.ID
	return nil
}

// LookupByHash implements RecoveryStore.
func (s *MemoryRecoveryStore) LookupByHash(_ context.Context, hash string) (RecoveryRec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byHash[hash]
	if !ok {
		return RecoveryRec{}, ErrNotFound
	}
	return s.byID[id], nil
}

// MarkUsed implements RecoveryStore.
func (s *MemoryRecoveryStore) MarkUsed(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	if rec.UsedAt == nil {
		now := time.Now()
		rec.UsedAt = &now
		s.byID[id] = rec
	}
	return nil
}

// DeleteExpired implements RecoveryStore.
func (s *MemoryRecoveryStore) DeleteExpired(_ context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, rec := range s.byID {
		if rec.UsedAt != nil || !rec.ExpiresAt.After(now) {
			delete(s.byHash, rec.TokenHash)
			delete(s.byID, id)
		}
	}
	return nil
}

// hashRecoveryToken binds a stored record to its token without keeping a
// usable bearer: SHA-256 hex over the raw token.
func hashRecoveryToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// RecoveryService issues and consumes single-use reset tokens over an
// account Service and a RecoveryStore.
type RecoveryService struct {
	users  *Service
	resets RecoveryStore
	ttl    time.Duration
}

// NewRecoveryService wires recovery onto the account service. A nil store
// gets memory; non-positive TTL defaults to one hour.
func NewRecoveryService(usersSvc *Service, store RecoveryStore, ttl time.Duration) *RecoveryService {
	if store == nil {
		store = NewMemoryRecoveryStore()
	}
	if ttl <= 0 {
		ttl = DefaultRecoveryTTL
	}
	return &RecoveryService{users: usersSvc, resets: store, ttl: ttl}
}

// Request mints a reset token for identifier (username or email when it
// contains "@"). Unknown identifiers and inactive accounts answer ("", nil):
// the HTTP layer always returns 204, so callers cannot enumerate accounts.
// The raw token is returned for delivery (dev log, future mailer).
func (s *RecoveryService) Request(ctx context.Context, identifier string) (string, error) {
	identifier = strings.TrimSpace(identifier)
	var (
		u   User
		err error
	)
	if strings.Contains(identifier, "@") {
		u, err = s.users.store.ByEmail(ctx, normalizeEmail(identifier))
	} else {
		u, err = s.users.store.ByUsername(ctx, normalizeUsername(identifier))
	}
	if err != nil || !u.Active {
		return "", nil
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("users: recovery token: %w", err)
	}
	token := hex.EncodeToString(raw[:])
	now := time.Now().UTC()
	rec := RecoveryRec{
		ID:        uuid.NewString(),
		UserID:    u.ID,
		TokenHash: hashRecoveryToken(token),
		ExpiresAt: now.Add(s.ttl),
		CreatedAt: now,
	}
	if err := s.resets.Save(ctx, rec); err != nil {
		return "", err
	}
	_ = s.resets.DeleteExpired(ctx, now)
	return token, nil
}

// Reset consumes a raw token and sets the new password: version bump
// kills bearers (service does it), the caller revokes refresh rows.
// Invalid, expired and reused tokens share ErrInvalidRecoveryToken.
func (s *RecoveryService) Reset(ctx context.Context, rawToken, newPassword string) (string, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return "", ErrInvalidRecoveryToken
	}
	rec, err := s.resets.LookupByHash(ctx, hashRecoveryToken(rawToken))
	if err != nil {
		return "", ErrInvalidRecoveryToken
	}
	if rec.UsedAt != nil || !rec.ExpiresAt.After(time.Now()) {
		return "", ErrInvalidRecoveryToken
	}
	if err := validatePassword(newPassword); err != nil {
		return "", err
	}
	if err := s.users.SetPassword(ctx, rec.UserID, newPassword); err != nil {
		if err == ErrNotFound {
			return "", ErrInvalidRecoveryToken
		}
		return "", err
	}
	if err := s.resets.MarkUsed(ctx, rec.ID); err != nil {
		return "", err
	}
	return rec.UserID, nil
}

package users

import (
	"context"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu    sync.Mutex
	users map[string]User
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{users: make(map[string]User)}
}

// Compile-time check that the adapter satisfies the port.
var _ Store = (*MemoryStore)(nil)

// Create implements Store, enforcing unique usernames and unique
// non-empty emails (case-insensitive).
func (s *MemoryStore) Create(_ context.Context, u User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.users {
		if strings.EqualFold(e.Username, u.Username) {
			return User{}, ErrDuplicateUsername
		}
		if u.Email != "" && strings.EqualFold(e.Email, u.Email) {
			return User{}, ErrDuplicateEmail
		}
	}
	s.users[u.ID] = u
	return u, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return User{}, ErrNotFound
	}
	return u, nil
}

// ByUsername implements Store (case-insensitive).
func (s *MemoryStore) ByUsername(_ context.Context, username string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if strings.EqualFold(u.Username, username) {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

// ByEmail implements Store (case-insensitive).
func (s *MemoryStore) ByEmail(_ context.Context, email string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if u.Email != "" && strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return User{}, ErrNotFound
}

// Update implements Store. It persists username, email and the active
// flag; the password hash is preserved from storage and can only change
// through SetPasswordHash.
func (s *MemoryStore) Update(_ context.Context, u User) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.users[u.ID]
	if !ok {
		return User{}, ErrNotFound
	}
	for _, e := range s.users {
		if e.ID == u.ID {
			continue
		}
		if strings.EqualFold(e.Username, u.Username) {
			return User{}, ErrDuplicateUsername
		}
		if u.Email != "" && strings.EqualFold(e.Email, u.Email) {
			return User{}, ErrDuplicateEmail
		}
	}
	u.PasswordHash = stored.PasswordHash
	if u.CreatedAt.IsZero() {
		u.CreatedAt = stored.CreatedAt
	}
	if u.UpdatedAt.IsZero() {
		u.UpdatedAt = time.Now()
	}
	s.users[u.ID] = u
	return u, nil
}

// SetPasswordHash implements Store.
func (s *MemoryStore) SetPasswordHash(_ context.Context, id, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	u.PasswordHash = hash
	u.UpdatedAt = time.Now()
	s.users[id] = u
	return nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.users, id)
	return nil
}

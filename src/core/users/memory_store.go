package users

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu     sync.Mutex
	users  map[string]User
	photos map[string][]byte
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		users:  make(map[string]User),
		photos: make(map[string][]byte),
	}
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
	u.TokenVersion = stored.TokenVersion
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

// BumpTokenVersion implements Store.
func (s *MemoryStore) BumpTokenVersion(_ context.Context, id string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return 0, ErrNotFound
	}
	u.TokenVersion++
	u.UpdatedAt = time.Now()
	s.users[id] = u
	return u.TokenVersion, nil
}

// SoftDelete implements Store: it stamps DeletedAt and disables the
// login without removing the row. Unknown users are a no-op;
// already deleted ones report ErrAlreadyDeleted.
func (s *MemoryStore) SoftDelete(_ context.Context, id string, at time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return nil
	}
	if u.DeletedAt != nil {
		return ErrAlreadyDeleted
	}
	u.DeletedAt = &at
	u.Active = false
	u.UpdatedAt = at
	s.users[id] = u
	return nil
}

// Restore implements Store: it clears the soft-delete mark and
// re-enables the login.
func (s *MemoryStore) Restore(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.users[id]
	if !ok {
		return ErrNotFound
	}
	if u.DeletedAt == nil {
		return ErrNotDeleted
	}
	u.DeletedAt = nil
	u.Active = true
	u.UpdatedAt = time.Now()
	s.users[id] = u
	return nil
}

// PurgeBefore implements Store: it physically removes users
// soft-deleted before cutoff and returns their ids.
func (s *MemoryStore) PurgeBefore(_ context.Context, cutoff time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id, u := range s.users {
		if u.DeletedAt != nil && !u.DeletedAt.After(cutoff) {
			delete(s.users, id)
			delete(s.photos, id)
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// List implements Store in username order. Deleted accounts are
// included only when includeDeleted is true.
func (s *MemoryStore) List(_ context.Context, includeDeleted bool) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]User, 0, len(s.users))
	for _, u := range s.users {
		if u.DeletedAt != nil && !includeDeleted {
			continue
		}
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Username != out[j].Username {
			return out[i].Username < out[j].Username
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// ForTeacher implements Store.
func (s *MemoryStore) ForTeacher(_ context.Context, teacherID string) ([]User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []User
	for _, u := range s.users {
		if u.TeacherID == teacherID {
			out = append(out, u.Sanitized())
		}
	}
	return out, nil
}

// Photo implements Store.
func (s *MemoryStore) Photo(_ context.Context, id string) (Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return Photo{}, ErrNotFound
	}
	return Photo{Data: s.photos[id]}, nil
}

// SetPhoto implements Store.
func (s *MemoryStore) SetPhoto(_ context.Context, id string, photo Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.users[id]; !ok {
		return ErrNotFound
	}
	if len(photo.Data) == 0 {
		delete(s.photos, id)
		return nil
	}
	s.photos[id] = photo.Data
	return nil
}

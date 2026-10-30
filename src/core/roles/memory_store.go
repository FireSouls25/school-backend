package roles

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu      sync.Mutex
	subject map[string]map[Role]struct{}
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{subject: make(map[string]map[Role]struct{})}
}

// AssignRole implements Store.
func (s *MemoryStore) AssignRole(_ context.Context, subjectID string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	roles, ok := s.subject[subjectID]
	if !ok {
		roles = make(map[Role]struct{})
		s.subject[subjectID] = roles
	}
	roles[role] = struct{}{}
	return nil
}

// RemoveRole implements Store.
func (s *MemoryStore) RemoveRole(_ context.Context, subjectID string, role Role) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if roles, ok := s.subject[subjectID]; ok {
		delete(roles, role)
	}
	return nil
}

// RolesFor implements Store.
func (s *MemoryStore) RolesFor(_ context.Context, subjectID string) ([]Role, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	roles := s.subject[subjectID]
	out := make([]Role, 0, len(roles))
	for r := range roles {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// SubjectsWithRole implements Store.
func (s *MemoryStore) SubjectsWithRole(_ context.Context, role Role) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for id, roles := range s.subject {
		if _, ok := roles[role]; ok {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out, nil
}

// RemoveAllForSubject implements Store.
func (s *MemoryStore) RemoveAllForSubject(_ context.Context, subjectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subject, subjectID)
	return nil
}

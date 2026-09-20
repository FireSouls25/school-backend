package classes

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu     sync.Mutex
	groups map[string]ClassGroup
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{groups: make(map[string]ClassGroup)}
}

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, g ClassGroup) (ClassGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groups[g.ID] = g
	return g, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (ClassGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g, ok := s.groups[id]
	if !ok {
		return ClassGroup{}, ErrNotFound
	}
	return g, nil
}

// ListByYear implements Store.
func (s *MemoryStore) ListByYear(_ context.Context, schoolYearID string) ([]ClassGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ClassGroup, 0)
	for _, g := range s.groups {
		if g.SchoolYearID == schoolYearID {
			out = append(out, g)
		}
	}
	sortGroups(out)
	return out, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, g ClassGroup) (ClassGroup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.groups[g.ID]; !ok {
		return ClassGroup{}, ErrNotFound
	}
	s.groups[g.ID] = g
	return g, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.groups, id)
	return nil
}

// sortGroups orders groups by grade, then group number, breaking ties by id.
func sortGroups(out []ClassGroup) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Grade != out[j].Grade {
			return out[i].Grade < out[j].Grade
		}
		if out[i].GroupNo != out[j].GroupNo {
			return out[i].GroupNo < out[j].GroupNo
		}
		return out[i].ID < out[j].ID
	})
}

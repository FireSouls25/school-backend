package teachers

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu       sync.Mutex
	teachers map[string]Teacher
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{teachers: make(map[string]Teacher)}
}

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, t Teacher) (Teacher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.teachers[t.ID] = t
	return t, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (Teacher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.teachers[id]
	if !ok {
		return Teacher{}, ErrNotFound
	}
	return t, nil
}

// List implements Store.
func (s *MemoryStore) List(_ context.Context) ([]Teacher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Teacher, 0, len(s.teachers))
	for _, t := range s.teachers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].FullName()) < strings.ToLower(out[j].FullName())
	})
	return out, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, t Teacher) (Teacher, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.teachers[t.ID]; !ok {
		return Teacher{}, ErrNotFound
	}
	s.teachers[t.ID] = t
	return t, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.teachers, id)
	return nil
}

package warnings

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu       sync.Mutex
	warnings map[string]Warning
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{warnings: make(map[string]Warning)}
}

// Add implements Store.
func (s *MemoryStore) Add(_ context.Context, w Warning) (Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warnings[w.ID] = w
	return w, nil
}

// ForStudent implements Store.
func (s *MemoryStore) ForStudent(_ context.Context, studentID string) ([]Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Warning, 0)
	for _, w := range s.warnings {
		if w.StudentID == studentID {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HappenedAt.After(out[j].HappenedAt) })
	return out, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.warnings, id)
	return nil
}

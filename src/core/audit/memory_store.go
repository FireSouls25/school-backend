package audit

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu      sync.Mutex
	entries []Entry
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

// Compile-time check that the adapter satisfies the port.
var _ Store = (*MemoryStore)(nil)

// Record implements Store.
func (s *MemoryStore) Record(_ context.Context, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return e, nil
}

// List implements Store: newest first, honoring limit and student filter.
func (s *MemoryStore) List(_ context.Context, f Filter) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0)
	for i := len(s.entries) - 1; i >= 0; i-- {
		e := s.entries[i]
		if f.StudentID != "" && e.StudentID != f.StudentID {
			continue
		}
		out = append(out, e)
		if len(out) == f.Limit {
			break
		}
	}
	return out, nil
}

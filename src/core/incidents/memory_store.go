package incidents

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu     sync.Mutex
	faults map[string]Fault
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{faults: make(map[string]Fault)}
}

// Add implements Store.
func (s *MemoryStore) Add(_ context.Context, f Fault) (Fault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.faults[f.ID] = f
	return f, nil
}

// ForStudent implements Store.
func (s *MemoryStore) ForStudent(_ context.Context, studentID string) ([]Fault, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Fault, 0)
	for _, f := range s.faults {
		if f.StudentID == studentID {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.faults, id)
	return nil
}

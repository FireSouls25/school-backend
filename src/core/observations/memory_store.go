package observations

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore is an in-memory observations.Store for tests and dev.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]Observation
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Observation)}
}

// Compile-time check that the adapter satisfies the port.
var _ Store = (*MemoryStore)(nil)

// Add implements Store.
func (s *MemoryStore) Add(_ context.Context, o Observation) (Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[o.ID] = o
	return o, nil
}

// ForTeacher implements Store.
func (s *MemoryStore) ForTeacher(_ context.Context, teacherID string) ([]Observation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Observation, 0)
	for _, o := range s.entries {
		if o.TeacherID == strings.TrimSpace(teacherID) {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].ID < out[j].ID
		}
		return out[i].Date.After(out[j].Date)
	})
	return out, nil
}

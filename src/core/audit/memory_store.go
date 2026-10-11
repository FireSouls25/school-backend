package audit

import (
	"context"
	"sort"
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

// Add implements Store.
func (s *MemoryStore) Add(_ context.Context, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, e)
	return e, nil
}

// ForStudent implements Store: entries that touched the given student are
// those whose resource is about that student or about one of its reports.
func (s *MemoryStore) ForStudent(_ context.Context, studentID string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0)
	for _, e := range s.entries {
		if e.ResourceID == studentID {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HappenedAt.After(out[j].HappenedAt) })
	return out, nil
}

// Recent implements Store.
func (s *MemoryStore) Recent(_ context.Context, limit int) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].HappenedAt.After(out[j].HappenedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

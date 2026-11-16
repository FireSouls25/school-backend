package permits

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory permits.Store for tests and dev.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]Permit
}

// NewMemoryStore returns an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Permit)}
}

// Compile-time check that the adapter satisfies the port.
var _ Store = (*MemoryStore)(nil)

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, p Permit) (Permit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[p.ID] = p
	return p, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (Permit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.entries[id]
	if !ok {
		return Permit{}, ErrNotFound
	}
	return p, nil
}

// ForGroupOnDate implements Store.
func (s *MemoryStore) ForGroupOnDate(_ context.Context, classGroupID string, day time.Time) ([]Permit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Permit, 0)
	for _, p := range s.entries {
		if p.ClassGroupID == classGroupID && sameDay(p.Date, day) {
			out = append(out, p)
		}
	}
	sortNewest(out)
	return out, nil
}

// ForStudentOnDate implements Store.
func (s *MemoryStore) ForStudentOnDate(_ context.Context, studentID string, day time.Time) ([]Permit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Permit, 0)
	for _, p := range s.entries {
		if p.StudentID == studentID && sameDay(p.Date, day) {
			out = append(out, p)
		}
	}
	sortNewest(out)
	return out, nil
}

// ForStudent implements Store.
func (s *MemoryStore) ForStudent(_ context.Context, studentID string) ([]Permit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Permit, 0)
	for _, p := range s.entries {
		if p.StudentID == studentID {
			out = append(out, p)
		}
	}
	sortNewest(out)
	return out, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[id]; !ok {
		return ErrNotFound
	}
	delete(s.entries, id)
	return nil
}

// sortNewest orders permits by day (newest first) and then by id, so
// listings stay stable across map iterations.
func sortNewest(out []Permit) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].ID < out[j].ID
		}
		return out[i].Date.After(out[j].Date)
	})
}

// sameDay reports whether both instants fall on the same calendar day.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

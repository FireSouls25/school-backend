package schedules

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]Entry
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]Entry)}
}

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[e.ID] = e
	return e, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[id]
	if !ok {
		return Entry{}, ErrNotFound
	}
	return e, nil
}

// EntriesForTeacher implements Store.
func (s *MemoryStore) EntriesForTeacher(_ context.Context, teacherID string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0)
	for _, e := range s.entries {
		if e.TeacherID == teacherID {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out, nil
}

// EntriesForGroup implements Store.
func (s *MemoryStore) EntriesForGroup(_ context.Context, classGroupID string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0)
	for _, e := range s.entries {
		if e.ClassGroupID == classGroupID {
			out = append(out, e)
		}
	}
	sortEntries(out)
	return out, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, e Entry) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.entries[e.ID]; !ok {
		return Entry{}, ErrNotFound
	}
	s.entries[e.ID] = e
	return e, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
	return nil
}

// sortEntries orders entries by weekday, then start time, breaking ties
// by id.
func sortEntries(out []Entry) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Weekday != out[j].Weekday {
			return out[i].Weekday < out[j].Weekday
		}
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].ID < out[j].ID
	})
}

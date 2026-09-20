package schoolyears

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu    sync.Mutex
	years map[string]SchoolYear
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{years: make(map[string]SchoolYear)}
}

// clone returns a copy of y with its own holidays slice.
func clone(y SchoolYear) SchoolYear {
	y.Holidays = append([]time.Time(nil), y.Holidays...)
	return y
}

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, y SchoolYear) (SchoolYear, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	y = clone(y)
	s.years[y.ID] = y
	return clone(y), nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (SchoolYear, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	y, ok := s.years[id]
	if !ok {
		return SchoolYear{}, ErrNotFound
	}
	return clone(y), nil
}

// ByYear implements Store.
func (s *MemoryStore) ByYear(_ context.Context, year int) (SchoolYear, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, y := range s.years {
		if y.Year == year {
			return clone(y), nil
		}
	}
	return SchoolYear{}, ErrNotFound
}

// List implements Store.
func (s *MemoryStore) List(_ context.Context) ([]SchoolYear, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SchoolYear, 0, len(s.years))
	for _, y := range s.years {
		out = append(out, clone(y))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Year > out[j].Year })
	return out, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, y SchoolYear) (SchoolYear, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.years[y.ID]; !ok {
		return SchoolYear{}, ErrNotFound
	}
	y = clone(y)
	s.years[y.ID] = y
	return clone(y), nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.years, id)
	return nil
}

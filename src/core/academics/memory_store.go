package academics

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore keeps alerts in process memory. It is the reference
// implementation for tests and dev.
type MemoryStore struct {
	mu sync.RWMutex
	by map[string]Alert
}

// NewMemoryStore wires an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{by: make(map[string]Alert)}
}

// Add implements Store.
func (m *MemoryStore) Add(_ context.Context, a Alert) (Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.by[a.ID] = a
	return a, nil
}

// ByID implements Store.
func (m *MemoryStore) ByID(_ context.Context, id string) (Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.by[strings.TrimSpace(id)]
	if !ok {
		return Alert{}, ErrNotFound
	}
	return a, nil
}

// Open implements Store.
func (m *MemoryStore) Open(_ context.Context, studentID string) ([]Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Alert, 0)
	for _, a := range m.by {
		if a.Status != StatusOpen {
			continue
		}
		if studentID != "" && a.StudentID != strings.TrimSpace(studentID) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// ForStudent implements Store.
func (m *MemoryStore) ForStudent(_ context.Context, studentID string) ([]Alert, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Alert, 0)
	for _, a := range m.by {
		if a.StudentID != strings.TrimSpace(studentID) {
			continue
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Update implements Store.
func (m *MemoryStore) Update(_ context.Context, a Alert) (Alert, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.by[a.ID]; !ok {
		return Alert{}, ErrNotFound
	}
	m.by[a.ID] = a
	return a, nil
}

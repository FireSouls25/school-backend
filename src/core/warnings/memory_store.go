package warnings

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu       sync.Mutex
	warnings map[string]Warning
	revs     []Revision
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

// Fetch implements Store.
func (s *MemoryStore) Fetch(_ context.Context, id string) (Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.warnings[id]
	if !ok {
		return Warning{}, ErrNotFound
	}
	return w, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, w Warning) (Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.warnings[w.ID]; !ok {
		return Warning{}, ErrNotFound
	}
	s.warnings[w.ID] = w
	return w, nil
}

// AddRevision implements Store.
func (s *MemoryStore) AddRevision(_ context.Context, r Revision) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revs = append(s.revs, r)
	return r, nil
}

// RevisionsForWarning implements Store.
func (s *MemoryStore) RevisionsForWarning(_ context.Context, warningID string) ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []Revision
	for _, r := range s.revs {
		if r.WarningID == warningID {
			out = append(out, r)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].ChangedAt.After(out[i].ChangedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// Delete implements Store.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.warnings, id)
	return nil
}

// ForGroup implements Store.
func (s *MemoryStore) ForGroup(_ context.Context, groupID string) ([]Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Warning, 0)
	for _, w := range s.warnings {
		if w.GroupID == groupID && groupID != "" {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HappenedAt.Equal(out[j].HappenedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].HappenedAt.Before(out[j].HappenedAt)
	})
	return out, nil
}

// Recent implements Store: every warning happened at or after since,
// newest first.
func (s *MemoryStore) Recent(_ context.Context, since time.Time) ([]Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Warning, 0)
	for _, w := range s.warnings {
		if !w.HappenedAt.Before(since) {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HappenedAt.Equal(out[j].HappenedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].HappenedAt.After(out[j].HappenedAt)
	})
	return out, nil
}

// ForAuthor returns every warning issued by the given login subject,
// newest first. It backs the "made by this teacher" history.
func (s *MemoryStore) ForAuthor(_ context.Context, authorID string) ([]Warning, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Warning, 0)
	for _, w := range s.warnings {
		if w.TeacherID == authorID {
			out = append(out, w)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].HappenedAt.Equal(out[j].HappenedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].HappenedAt.After(out[j].HappenedAt)
	})
	return out, nil
}

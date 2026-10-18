package tattendance

import (
	"context"
	"sync"
	"time"
)

// MemoryStore keeps teacher marks in process memory, keyed by teacher
// and day. It is for development and tests; restart wipes it.
type MemoryStore struct {
	mu    sync.RWMutex
	marks map[string]Mark
}

// NewMemoryStore wires an empty in-memory teacher-attendance store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{marks: make(map[string]Mark)}
}

func key(teacherID string, day time.Time) string {
	return teacherID + "|" + day.UTC().Format("2006-01-02")
}

// Upsert implements Store.
func (s *MemoryStore) Upsert(_ context.Context, m Mark) (Mark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.marks[key(m.TeacherID, m.Day)] = m
	return m, nil
}

// ForDay implements Store.
func (s *MemoryStore) ForDay(_ context.Context, day time.Time) ([]Mark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	want := day.UTC().Format("2006-01-02")
	var out []Mark
	for _, m := range s.marks {
		if m.Day.UTC().Format("2006-01-02") == want {
			out = append(out, m)
		}
	}
	return out, nil
}

// ForTeacher implements Store.
func (s *MemoryStore) ForTeacher(_ context.Context, teacherID string, from, to time.Time) ([]Mark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []Mark
	for _, m := range s.marks {
		if m.TeacherID != teacherID {
			continue
		}
		d := m.Day.UTC()
		if (d.Equal(from) || d.After(from)) && (d.Equal(to) || d.Before(to)) {
			out = append(out, m)
		}
	}
	// Newest first.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Day.After(out[i].Day) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

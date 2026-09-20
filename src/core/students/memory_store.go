package students

import (
	"context"
	"sort"
	"strings"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu       sync.Mutex
	students map[string]Student
	photos   map[string][]byte
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		students: make(map[string]Student),
		photos:   make(map[string][]byte),
	}
}

// Create implements Store.
func (s *MemoryStore) Create(_ context.Context, st Student) (Student, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.students[st.ID] = st
	return st, nil
}

// ByID implements Store.
func (s *MemoryStore) ByID(_ context.Context, id string) (Student, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.students[id]
	if !ok {
		return Student{}, ErrNotFound
	}
	return st, nil
}

// ListByClass implements Store.
func (s *MemoryStore) ListByClass(_ context.Context, classID string) ([]Student, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Student, 0)
	for _, st := range s.students {
		if st.ClassID == classID {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].FullName()) < strings.ToLower(out[j].FullName())
	})
	return out, nil
}

// Update implements Store.
func (s *MemoryStore) Update(_ context.Context, st Student) (Student, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.students[st.ID]; !ok {
		return Student{}, ErrNotFound
	}
	s.students[st.ID] = st
	return st, nil
}

// Delete implements Store. Photos are dropped with the student.
func (s *MemoryStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.students, id)
	delete(s.photos, id)
	return nil
}

// PutPhoto implements Store.
func (s *MemoryStore) PutPhoto(_ context.Context, studentID string, photo Photo) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.students[studentID]; !ok {
		return ErrNotFound
	}
	s.photos[studentID] = append([]byte(nil), photo.Data...)
	return nil
}

// Photo implements Store.
func (s *MemoryStore) Photo(_ context.Context, studentID string) (Photo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Photo{Data: append([]byte(nil), s.photos[studentID]...)}, nil
}

package subjects

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu          sync.Mutex
	subjects    map[string]Subject
	assignments map[string]Assignment
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		subjects:    make(map[string]Subject),
		assignments: make(map[string]Assignment),
	}
}

// CreateSubject implements Store.
func (s *MemoryStore) CreateSubject(_ context.Context, subj Subject) (Subject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subjects[subj.ID] = subj
	return subj, nil
}

// SubjectByID implements Store.
func (s *MemoryStore) SubjectByID(_ context.Context, id string) (Subject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	subj, ok := s.subjects[id]
	if !ok {
		return Subject{}, ErrNotFound
	}
	return subj, nil
}

// ListSubjects implements Store.
func (s *MemoryStore) ListSubjects(_ context.Context) ([]Subject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Subject, 0, len(s.subjects))
	for _, subj := range s.subjects {
		out = append(out, subj)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// UpdateSubject implements Store.
func (s *MemoryStore) UpdateSubject(_ context.Context, subj Subject) (Subject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subjects[subj.ID]; !ok {
		return Subject{}, ErrNotFound
	}
	s.subjects[subj.ID] = subj
	return subj, nil
}

// DeleteSubject implements Store.
func (s *MemoryStore) DeleteSubject(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subjects, id)
	return nil
}

// AddAssignment implements Store.
func (s *MemoryStore) AddAssignment(_ context.Context, a Assignment) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignments[a.ID] = a
	return a, nil
}

// AssignmentByID implements Store.
func (s *MemoryStore) AssignmentByID(_ context.Context, id string) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[id]
	if !ok {
		return Assignment{}, ErrAssignmentNotFound
	}
	return a, nil
}

// AssignmentsForTeacher implements Store.
func (s *MemoryStore) AssignmentsForTeacher(_ context.Context, teacherID string) ([]Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.forTeacherLocked(teacherID), nil
}

// AssignmentsForSubject implements Store.
func (s *MemoryStore) AssignmentsForSubject(_ context.Context, subjectID string) ([]Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Assignment, 0)
	for _, a := range s.assignments {
		if a.SubjectID == subjectID {
			out = append(out, a)
		}
	}
	sortByStart(out)
	return out, nil
}

// EndAssignment implements Store.
func (s *MemoryStore) EndAssignment(_ context.Context, id string, endedAt time.Time) (Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.assignments[id]
	if !ok {
		return Assignment{}, ErrAssignmentNotFound
	}
	a.EndedAt = endedAt
	s.assignments[id] = a
	return a, nil
}

// RemoveAssignment implements Store.
func (s *MemoryStore) RemoveAssignment(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.assignments, id)
	return nil
}

func (s *MemoryStore) forTeacherLocked(teacherID string) []Assignment {
	out := make([]Assignment, 0)
	for _, a := range s.assignments {
		if a.TeacherID == teacherID {
			out = append(out, a)
		}
	}
	sortByStart(out)
	return out
}

// sortByStart orders assignments chronologically, breaking ties by id.
func sortByStart(out []Assignment) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
}

package enrollments

import (
	"context"
	"sort"
	"sync"
	"time"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu          sync.Mutex
	enrollments map[string]enrollmentRow
	promotions  map[string]Promotion
}

// enrollmentRow carries the insertion time used for stable ordering.
type enrollmentRow struct {
	Enrollment
	addedAt time.Time
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		enrollments: make(map[string]enrollmentRow),
		promotions:  make(map[string]Promotion),
	}
}

// AddEnrollment implements Store.
func (s *MemoryStore) AddEnrollment(_ context.Context, e Enrollment) (Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enrollments[e.ID] = enrollmentRow{Enrollment: e, addedAt: time.Now().UTC()}
	return e, nil
}

// EnrollmentsForGroup implements Store.
func (s *MemoryStore) EnrollmentsForGroup(_ context.Context, classGroupID string) ([]Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Enrollment, 0)
	for _, row := range s.enrollments {
		if row.ClassGroupID == classGroupID {
			out = append(out, row.Enrollment)
		}
	}
	s.sortByAddedLocked(out)
	return out, nil
}

// EnrollmentsForStudent implements Store.
func (s *MemoryStore) EnrollmentsForStudent(_ context.Context, studentID string) ([]Enrollment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Enrollment, 0)
	for _, row := range s.enrollments {
		if row.StudentID == studentID {
			out = append(out, row.Enrollment)
		}
	}
	s.sortByAddedLocked(out)
	return out, nil
}

// RemoveEnrollment implements Store.
func (s *MemoryStore) RemoveEnrollment(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.enrollments, id)
	return nil
}

// RecordPromotion implements Store.
func (s *MemoryStore) RecordPromotion(_ context.Context, p Promotion) (Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.promotions[p.ID] = p
	return p, nil
}

// PromotionsForStudent implements Store.
func (s *MemoryStore) PromotionsForStudent(_ context.Context, studentID string) ([]Promotion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Promotion, 0)
	for _, p := range s.promotions {
		if p.StudentID == studentID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DecidedAt.Equal(out[j].DecidedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].DecidedAt.Before(out[j].DecidedAt)
	})
	return out, nil
}

func (s *MemoryStore) sortByAddedLocked(out []Enrollment) {
	byID := make(map[string]time.Time, len(out))
	for _, row := range s.enrollments {
		byID[row.ID] = row.addedAt
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := byID[out[i].ID], byID[out[j].ID]
		if ai.Equal(aj) {
			return out[i].ID < out[j].ID
		}
		return ai.Before(aj)
	})
}

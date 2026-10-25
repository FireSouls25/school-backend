package academics

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies academic-alert policy and persists records through a
// Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Raise validates the input, assigns a fresh id and persists one open
// alert per call.
func (s *Service) Raise(ctx context.Context, in Input) (Alert, error) {
	a := Alert{
		ID:        uuid.NewString(),
		StudentID: strings.TrimSpace(in.StudentID),
		TeacherID: strings.TrimSpace(in.TeacherID),
		Reason:    strings.TrimSpace(in.Reason),
		Status:    StatusOpen,
		CreatedAt: time.Now().UTC(),
	}
	if err := validate(a); err != nil {
		return Alert{}, err
	}
	return s.store.Add(ctx, a)
}

// Review marks an open alert as checked by an admin. Reviewing twice is
// a no-op error, never a silent duplicate.
func (s *Service) Review(ctx context.Context, id, reviewerID string) (Alert, error) {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return Alert{}, ErrNotFound
	}
	a, err := s.store.ByID(ctx, id)
	if err != nil {
		return Alert{}, err
	}
	if a.Status == StatusReviewed {
		return Alert{}, ErrAlreadyReviewed
	}
	a.Status = StatusReviewed
	a.ReviewedAt = time.Now().UTC()
	a.ReviewedBy = strings.TrimSpace(reviewerID)
	return s.store.Update(ctx, a)
}

// Open returns unreviewed alerts, newest first, optionally filtered by
// student.
func (s *Service) Open(ctx context.Context, studentID string) ([]Alert, error) {
	return s.store.Open(ctx, strings.TrimSpace(studentID))
}

// ForStudent returns every alert of one student, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Alert, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, strings.TrimSpace(studentID))
}

func validate(a Alert) error {
	if _, err := uuid.Parse(a.StudentID); err != nil {
		return ErrInvalidStudent
	}
	if strings.TrimSpace(a.TeacherID) == "" {
		return ErrInvalidTeacher
	}
	if a.Reason == "" {
		return ErrEmptyReason
	}
	return nil
}

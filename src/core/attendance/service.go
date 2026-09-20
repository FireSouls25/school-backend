package attendance

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies attendance policy and persists records through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Record validates the entry and persists a new attendance record.
func (s *Service) Record(ctx context.Context, studentID, classID string, date time.Time, reason Reason) (Record, error) {
	r := Record{
		ID:        uuid.NewString(),
		StudentID: strings.TrimSpace(studentID),
		ClassID:   strings.TrimSpace(classID),
		Date:      date,
		Reason:    reason,
	}
	if err := validate(r); err != nil {
		return Record{}, err
	}
	return s.store.Add(ctx, r)
}

// ForStudent returns the full history of studentID, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Record, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, studentID)
}

// Remove deletes an attendance record.
func (s *Service) Remove(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	return s.store.Delete(ctx, id)
}

func validate(r Record) error {
	if r.StudentID == "" {
		return ErrInvalidStudent
	}
	if r.ClassID == "" {
		return ErrInvalidClass
	}
	if r.Date.IsZero() {
		return ErrInvalidDate
	}
	if !r.Reason.IsValid() {
		return ErrUnknownReason
	}
	return nil
}

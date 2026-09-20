package incidents

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies fault policy and persists records through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Report validates the fault and persists a new record.
func (s *Service) Report(ctx context.Context, studentID, classID string, date time.Time, severity Severity, description string) (Fault, error) {
	f := Fault{
		ID:          uuid.NewString(),
		StudentID:   strings.TrimSpace(studentID),
		ClassID:     strings.TrimSpace(classID),
		Date:        date,
		Severity:    severity,
		Description: strings.TrimSpace(description),
	}
	if err := validate(f); err != nil {
		return Fault{}, err
	}
	return s.store.Add(ctx, f)
}

// ForStudent returns the full history of studentID, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Fault, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, studentID)
}

// Remove deletes a fault record.
func (s *Service) Remove(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	return s.store.Delete(ctx, id)
}

func validate(f Fault) error {
	if f.StudentID == "" {
		return ErrInvalidStudent
	}
	if f.ClassID == "" {
		return ErrInvalidClass
	}
	if f.Date.IsZero() {
		return ErrInvalidDate
	}
	if !f.Severity.IsValid() {
		return ErrUnknownSeverity
	}
	if f.Description == "" {
		return ErrEmptyDescription
	}
	return nil
}

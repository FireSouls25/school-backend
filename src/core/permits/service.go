package permits

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store is the persistence port for permits. Implementations must be
// safe for concurrent use.
type Store interface {
	// Create persists p and returns the stored permit.
	Create(ctx context.Context, p Permit) (Permit, error)
	// ByID returns the permit with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (Permit, error)
	// ForGroupOnDate returns the permits of one class-group on one day.
	ForGroupOnDate(ctx context.Context, classGroupID string, day time.Time) ([]Permit, error)
	// ForStudentOnDate returns one student's permits on one day.
	ForStudentOnDate(ctx context.Context, studentID string, day time.Time) ([]Permit, error)
	// ForStudent returns one student's permits, newest day first.
	ForStudent(ctx context.Context, studentID string) ([]Permit, error)
	// Delete removes one permit; a missing id reports ErrNotFound.
	Delete(ctx context.Context, id string) error
}

// Service applies the permit policy and persists records through a
// Store. The granting office and the covered student are always the
// caller-provided references; ids and timestamps are assigned here.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Grant validates the draft, stamps a fresh id and persists the permit.
// The ID field of the draft is ignored. Missing dates mean today.
func (s *Service) Grant(ctx context.Context, d Draft) (Permit, error) {
	studentID := strings.TrimSpace(d.StudentID)
	if _, err := uuid.Parse(studentID); err != nil {
		return Permit{}, ErrInvalidStudent
	}
	classGroupID := strings.TrimSpace(d.ClassGroupID)
	if _, err := uuid.Parse(classGroupID); err != nil {
		return Permit{}, ErrInvalidClassGroup
	}
	reason := strings.TrimSpace(d.Reason)
	if reason == "" {
		return Permit{}, ErrEmptyReason
	}
	createdBy := strings.TrimSpace(d.CreatedBy)
	if createdBy == "" {
		return Permit{}, ErrInvalidActor
	}
	day, err := parseDay(d.Date)
	if err != nil {
		return Permit{}, err
	}
	return s.store.Create(ctx, Permit{
		ID:           uuid.NewString(),
		StudentID:    studentID,
		ClassGroupID: classGroupID,
		SubjectID:    strings.TrimSpace(d.SubjectID),
		Date:         day,
		Reason:       reason,
		CreatedBy:    createdBy,
		CreatedAt:    time.Now().UTC(),
	})
}

// ForGroupOnDate returns the permits covering one class-group on one
// day, oldest first.
func (s *Service) ForGroupOnDate(ctx context.Context, classGroupID string, day time.Time) ([]Permit, error) {
	if _, err := uuid.Parse(strings.TrimSpace(classGroupID)); err != nil {
		return nil, ErrInvalidClassGroup
	}
	return s.store.ForGroupOnDate(ctx, strings.TrimSpace(classGroupID), NormalizeDay(day))
}

// ForStudent returns one student's permits, newest day first. It feeds
// the student file, where the office reviews and revokes permits.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Permit, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, strings.TrimSpace(studentID))
}

// Revoke removes one permit by id. Removing an unknown permit reports
// ErrNotFound so the office sees the row was already gone.
func (s *Service) Revoke(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if _, err := uuid.Parse(id); err != nil {
		return ErrInvalidID
	}
	return s.store.Delete(ctx, id)
}

// PermitOf returns the permit covering the student in that class-group
// on that day, when one exists. The empty subject matches every permit
// that covers the group that day.
func (s *Service) PermitOf(ctx context.Context, studentID, classGroupID string, day time.Time) (Permit, bool, error) {
	list, err := s.ForGroupOnDate(ctx, classGroupID, day)
	if err != nil {
		return Permit{}, false, err
	}
	for _, p := range list {
		if p.StudentID == strings.TrimSpace(studentID) {
			return p, true, nil
		}
	}
	return Permit{}, false, nil
}

// NormalizeDay drops the clock so calendar comparisons stay exact.
func NormalizeDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// parseDay accepts an ISO day ("2006-01-02") or an RFC3339 instant and
// normalizes it; empty means today in UTC.
func parseDay(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return NormalizeDay(time.Now().UTC()), nil
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		return NormalizeDay(d), nil
	}
	if d, err := time.Parse(time.RFC3339, raw); err == nil {
		return NormalizeDay(d), nil
	}
	return time.Time{}, ErrInvalidDate
}

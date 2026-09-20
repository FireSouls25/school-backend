package warnings

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Service applies warning policy and persists records through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Issue validates the input, assigns a fresh id and persists the warning.
func (s *Service) Issue(ctx context.Context, in Input) (Warning, error) {
	w := Warning{
		ID:          uuid.NewString(),
		StudentID:   strings.TrimSpace(in.StudentID),
		ClassID:     strings.TrimSpace(in.ClassID),
		TeacherID:   strings.TrimSpace(in.TeacherID),
		HappenedAt:  in.HappenedAt,
		Gravity:     in.Gravity,
		Title:       strings.TrimSpace(in.Title),
		Description: strings.TrimSpace(in.Description),
		Snapshot:    normalizeSnapshot(in.Snapshot),
	}
	if err := validate(w); err != nil {
		return Warning{}, err
	}
	return s.store.Add(ctx, w)
}

// ForStudent returns the full warning history of studentID, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Warning, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, studentID)
}

// Remove deletes a warning record.
func (s *Service) Remove(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return ErrNotFound
	}
	return s.store.Delete(ctx, id)
}

func normalizeSnapshot(s StudentSnapshot) StudentSnapshot {
	trim := strings.TrimSpace
	s.Names = trim(s.Names)
	s.Surnames = trim(s.Surnames)
	s.DocumentID = trim(s.DocumentID)
	s.ClassID = trim(s.ClassID)
	s.CaregiverName = trim(s.CaregiverName)
	s.CaregiverPhone = trim(s.CaregiverPhone)
	return s
}

func validate(w Warning) error {
	if _, err := uuid.Parse(w.StudentID); err != nil {
		return ErrInvalidStudent
	}
	if w.ClassID == "" {
		return ErrInvalidClass
	}
	if w.TeacherID == "" {
		return ErrInvalidTeacher
	}
	if w.HappenedAt.IsZero() {
		return ErrInvalidDate
	}
	if !w.Gravity.IsValid() {
		return ErrUnknownGravity
	}
	if w.Title == "" {
		return ErrEmptyTitle
	}
	if w.Description == "" {
		return ErrEmptyDescription
	}
	if w.Snapshot.Names == "" || w.Snapshot.Surnames == "" ||
		w.Snapshot.DocumentID == "" || w.Snapshot.ClassID == "" {
		return ErrInvalidSnapshot
	}
	return nil
}

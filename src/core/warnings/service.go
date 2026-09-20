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
		GroupID:     strings.TrimSpace(in.GroupID),
	}
	if err := validate(w); err != nil {
		return Warning{}, err
	}
	return s.store.Add(ctx, w)
}

// IssueBatch issues one event against several students: every warning
// shares a fresh GroupID while each student keeps its own history. All
// items validate before anything persists.
func (s *Service) IssueBatch(ctx context.Context, in BatchInput) ([]Warning, error) {
	if len(in.Items) == 0 {
		return nil, ErrEmptyBatch
	}
	groupID := uuid.NewString()
	warnings := make([]Warning, 0, len(in.Items))
	for _, item := range in.Items {
		w := Warning{
			ID:          uuid.NewString(),
			StudentID:   strings.TrimSpace(item.StudentID),
			ClassID:     strings.TrimSpace(in.ClassID),
			TeacherID:   strings.TrimSpace(in.TeacherID),
			HappenedAt:  in.HappenedAt,
			Gravity:     in.Gravity,
			Title:       strings.TrimSpace(in.Title),
			Description: strings.TrimSpace(in.Description),
			Snapshot:    normalizeSnapshot(item.Snapshot),
			GroupID:     groupID,
		}
		if err := validate(w); err != nil {
			return nil, err
		}
		warnings = append(warnings, w)
	}
	out := make([]Warning, 0, len(warnings))
	for _, w := range warnings {
		stored, err := s.store.Add(ctx, w)
		if err != nil {
			return nil, err
		}
		out = append(out, stored)
	}
	return out, nil
}

// ForStudent returns the full warning history of studentID, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Warning, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, studentID)
}

// ForGroup returns every warning sharing a batch id, oldest first.
func (s *Service) ForGroup(ctx context.Context, groupID string) ([]Warning, error) {
	if _, err := uuid.Parse(strings.TrimSpace(groupID)); err != nil {
		return nil, ErrInvalidGroup
	}
	return s.store.ForGroup(ctx, strings.TrimSpace(groupID))
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
	if w.GroupID != "" {
		if _, err := uuid.Parse(w.GroupID); err != nil {
			return ErrInvalidGroup
		}
	}
	return nil
}

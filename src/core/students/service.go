package students

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Service applies profile policy and persists students through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates the profile, assigns a fresh UUID and persists it.
func (s *Service) Create(ctx context.Context, names, surnames, classID string) (Student, error) {
	st := Student{
		ID:       uuid.NewString(),
		Names:    strings.TrimSpace(names),
		Surnames: strings.TrimSpace(surnames),
		ClassID:  strings.TrimSpace(classID),
	}
	if err := validate(st); err != nil {
		return Student{}, err
	}
	return s.store.Create(ctx, st)
}

// ByID returns the student with the given id.
func (s *Service) ByID(ctx context.Context, id string) (Student, error) {
	if err := validateID(id); err != nil {
		return Student{}, err
	}
	return s.store.ByID(ctx, id)
}

// ListByClass returns every student in classID ordered alphabetically.
func (s *Service) ListByClass(ctx context.Context, classID string) ([]Student, error) {
	if strings.TrimSpace(classID) == "" {
		return nil, ErrInvalidClass
	}
	return s.store.ListByClass(ctx, classID)
}

// Update replaces the mutable fields of an existing student.
func (s *Service) Update(ctx context.Context, id, names, surnames, classID string) (Student, error) {
	st := Student{
		ID:       strings.TrimSpace(id),
		Names:    strings.TrimSpace(names),
		Surnames: strings.TrimSpace(surnames),
		ClassID:  strings.TrimSpace(classID),
	}
	if err := validate(st); err != nil {
		return Student{}, err
	}
	return s.store.Update(ctx, st)
}

// Delete removes a student and, by store contract, its dependent records.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// PutPhoto validates size limits and stores the student photo.
func (s *Service) PutPhoto(ctx context.Context, studentID string, photo Photo) error {
	if err := validateID(studentID); err != nil {
		return err
	}
	if len(photo.Data) > MaxPhotoSize {
		return ErrPhotoTooLarge
	}
	return s.store.PutPhoto(ctx, studentID, photo)
}

// Photo returns the stored photo of a student.
func (s *Service) Photo(ctx context.Context, studentID string) (Photo, error) {
	if err := validateID(studentID); err != nil {
		return Photo{}, err
	}
	return s.store.Photo(ctx, studentID)
}

func validate(st Student) error {
	if st.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(st.ID); err != nil {
		return ErrInvalidID
	}
	if st.Names == "" || st.Surnames == "" {
		return ErrInvalidName
	}
	if st.ClassID == "" {
		return ErrInvalidClass
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}

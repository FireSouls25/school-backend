package teachers

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies profile policy and persists teachers through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates the profile, assigns a fresh UUID and persists it.
// The ID field of t is ignored.
func (s *Service) Create(ctx context.Context, t Teacher) (Teacher, error) {
	t.ID = uuid.NewString()
	t = normalize(t)
	if err := validate(t); err != nil {
		return Teacher{}, err
	}
	return s.store.Create(ctx, t)
}

// ByID returns the teacher with the given id.
func (s *Service) ByID(ctx context.Context, id string) (Teacher, error) {
	if err := validateID(id); err != nil {
		return Teacher{}, err
	}
	return s.store.ByID(ctx, id)
}

// List returns every teacher ordered alphabetically.
func (s *Service) List(ctx context.Context) ([]Teacher, error) {
	return s.store.List(ctx)
}

// Update replaces the profile of an existing teacher.
func (s *Service) Update(ctx context.Context, t Teacher) (Teacher, error) {
	t.ID = strings.TrimSpace(t.ID)
	t = normalize(t)
	if err := validate(t); err != nil {
		return Teacher{}, err
	}
	return s.store.Update(ctx, t)
}

// Delete removes a teacher and, by store contract, its subject assignments.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// normalize trims every free-text field and lowercases the email so
// validation and storage see a canonical form.
func normalize(t Teacher) Teacher {
	trim := strings.TrimSpace
	t.Names = trim(t.Names)
	t.Surnames = trim(t.Surnames)
	t.DocumentID = trim(t.DocumentID)
	t.Phone = trim(t.Phone)
	t.Address = trim(t.Address)
	t.Birthplace = trim(t.Birthplace)
	t.Email = strings.ToLower(trim(t.Email))
	t.MedicalConditions = trim(t.MedicalConditions)
	t.HomeroomClassID = trim(t.HomeroomClassID)
	return t
}

func validate(t Teacher) error {
	if t.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(t.ID); err != nil {
		return ErrInvalidID
	}
	if t.Names == "" || t.Surnames == "" {
		return ErrInvalidName
	}
	if t.DocumentID == "" {
		return ErrInvalidDocument
	}
	if !t.Birthdate.IsZero() && t.Birthdate.After(time.Now()) {
		return ErrInvalidBirth
	}
	if t.Email != "" {
		if _, err := mail.ParseAddress(t.Email); err != nil {
			return ErrInvalidEmail
		}
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}

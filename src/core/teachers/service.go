package teachers

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"grade/src/core/slug"
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
// The ID field of t is ignored. New teachers are always active.
// mintSlug draws a fresh URL slug, retrying on the rare collision.
func (s *Service) mintSlug(ctx context.Context) string {
	for i := 0; i < 5; i++ {
		candidate := slug.New()
		if _, err := s.store.BySlug(ctx, candidate); err != nil {
			return candidate
		}
	}
	return slug.New()
}

// healSlug backfills the URL slug of legacy rows exactly once.
func (s *Service) healSlug(ctx context.Context, current Teacher) Teacher {
	if strings.TrimSpace(current.Slug) != "" {
		return current
	}
	current.Slug = s.mintSlug(ctx)
	if updated, err := s.store.Update(ctx, current); err == nil {
		return updated
	}
	return current
}

func (s *Service) Create(ctx context.Context, t Teacher) (Teacher, error) {
	t.ID = uuid.NewString()
	t.Slug = s.mintSlug(ctx)
	t.Active = true
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
	t, err := s.store.ByID(ctx, id)
	if err != nil {
		return Teacher{}, err
	}
	return s.healSlug(ctx, t), nil
}

// BySlug resolves a browser URL slug to its profile.
func (s *Service) BySlug(ctx context.Context, slug string) (Teacher, error) {
	if strings.TrimSpace(slug) == "" {
		return Teacher{}, ErrNotFound
	}
	return s.store.BySlug(ctx, strings.TrimSpace(slug))
}

// List returns every teacher ordered alphabetically.
func (s *Service) List(ctx context.Context) ([]Teacher, error) {
	return s.store.List(ctx)
}

// Update replaces the profile of an existing teacher.
func (s *Service) Update(ctx context.Context, t Teacher) (Teacher, error) {
	t.ID = strings.TrimSpace(t.ID)
	if strings.TrimSpace(t.Slug) == "" {
		if current, err := s.store.ByID(ctx, t.ID); err == nil {
			t.Slug = current.Slug
		}
	}
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

// PutPhoto validates size limits and stores the teacher photo.
func (s *Service) PutPhoto(ctx context.Context, teacherID string, photo Photo) error {
	if err := validateID(teacherID); err != nil {
		return err
	}
	if len(photo.Data) > MaxPhotoSize {
		return ErrPhotoTooLarge
	}
	return s.store.PutPhoto(ctx, teacherID, photo)
}

// Photo returns the stored photo of a teacher.
func (s *Service) Photo(ctx context.Context, teacherID string) (Photo, error) {
	if err := validateID(teacherID); err != nil {
		return Photo{}, err
	}
	return s.store.Photo(ctx, teacherID)
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
	t.Sex = strings.ToLower(trim(t.Sex))
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
	if t.Sex != "" && t.Sex != SexFemale && t.Sex != SexMale {
		return ErrInvalidSex
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}

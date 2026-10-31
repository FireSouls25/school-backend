package classes

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"grade/src/core/slug"
)

// Service applies class-group policy and persists groups through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates the group, assigns a fresh UUID and persists it.
// The ID field of g is ignored.
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
func (s *Service) healSlug(ctx context.Context, current ClassGroup) ClassGroup {
	if strings.TrimSpace(current.Slug) != "" {
		return current
	}
	current.Slug = s.mintSlug(ctx)
	if updated, err := s.store.Update(ctx, current); err == nil {
		return updated
	}
	return current
}

func (s *Service) Create(ctx context.Context, g ClassGroup) (ClassGroup, error) {
	g.ID = uuid.NewString()
	g.Slug = s.mintSlug(ctx)
	g.SchoolYearID = strings.TrimSpace(g.SchoolYearID)
	if err := validate(g); err != nil {
		return ClassGroup{}, err
	}
	if err := s.ensureUnique(ctx, g); err != nil {
		return ClassGroup{}, err
	}
	return s.store.Create(ctx, g)
}

// ByID returns the class-group with the given id.
func (s *Service) ByID(ctx context.Context, id string) (ClassGroup, error) {
	if err := validateID(id); err != nil {
		return ClassGroup{}, err
	}
	g, err := s.store.ByID(ctx, id)
	if err != nil {
		return ClassGroup{}, err
	}
	return s.healSlug(ctx, g), nil
}

// BySlug resolves a browser URL slug to its group.
func (s *Service) BySlug(ctx context.Context, slug string) (ClassGroup, error) {
	if strings.TrimSpace(slug) == "" {
		return ClassGroup{}, ErrNotFound
	}
	return s.store.BySlug(ctx, strings.TrimSpace(slug))
}

// ListByYear returns every group of a school year ordered by grade,
// then group number.
func (s *Service) ListByYear(ctx context.Context, schoolYearID string) ([]ClassGroup, error) {
	if err := validateSchoolYearID(schoolYearID); err != nil {
		return nil, err
	}
	return s.store.ListByYear(ctx, strings.TrimSpace(schoolYearID))
}

// Update replaces an existing class-group. The grade+group must stay
// unique within the year.
func (s *Service) Update(ctx context.Context, g ClassGroup) (ClassGroup, error) {
	g.ID = strings.TrimSpace(g.ID)
	if strings.TrimSpace(g.Slug) == "" {
		if current, err := s.store.ByID(ctx, g.ID); err == nil {
			g.Slug = current.Slug
		}
	}
	g.SchoolYearID = strings.TrimSpace(g.SchoolYearID)
	if err := validate(g); err != nil {
		return ClassGroup{}, err
	}
	if err := s.ensureUnique(ctx, g); err != nil {
		return ClassGroup{}, err
	}
	return s.store.Update(ctx, g)
}

// Delete removes a class-group. It is blocked while the group keeps
// enrollments; the database enforces this with ErrHasEnrollments.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func validate(g ClassGroup) error {
	if g.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(g.ID); err != nil {
		return ErrInvalidID
	}
	if err := validateSchoolYearID(g.SchoolYearID); err != nil {
		return err
	}
	if g.Grade < MinGrade || g.Grade > MaxGrade {
		return ErrInvalidGrade
	}
	if g.GroupNo < MinGroupNo {
		return ErrInvalidGroup
	}
	return nil
}

// ensureUnique rejects grade+group pairs already taken within the year.
func (s *Service) ensureUnique(ctx context.Context, g ClassGroup) error {
	all, err := s.store.ListByYear(ctx, g.SchoolYearID)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.ID != g.ID && other.Grade == g.Grade && other.GroupNo == g.GroupNo {
			return ErrDuplicateClass
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

func validateSchoolYearID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidSchoolYear
	}
	return nil
}

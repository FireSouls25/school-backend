package classes

import (
	"context"
	"strings"

	"github.com/google/uuid"
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
func (s *Service) Create(ctx context.Context, g ClassGroup) (ClassGroup, error) {
	g.ID = uuid.NewString()
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
	return s.store.ByID(ctx, id)
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

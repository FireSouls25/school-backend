package enrollments

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// Service applies enrollment policy and persists records through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Enroll places a student in a class-group and persists the enrollment.
// The ID field of e is ignored.
func (s *Service) Enroll(ctx context.Context, e Enrollment) (Enrollment, error) {
	e.ID = uuid.NewString()
	e.StudentID = strings.TrimSpace(e.StudentID)
	e.ClassGroupID = strings.TrimSpace(e.ClassGroupID)
	if err := validateRefs(e.StudentID, e.ClassGroupID); err != nil {
		return Enrollment{}, err
	}
	if err := s.ensureUnique(ctx, e); err != nil {
		return Enrollment{}, err
	}
	return s.store.AddEnrollment(ctx, e)
}

// Unenroll removes an enrollment, for corrections.
func (s *Service) Unenroll(ctx context.Context, id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return s.store.RemoveEnrollment(ctx, id)
}

// EnrollmentsForGroup returns every enrollment of a class-group in
// enrollment order.
func (s *Service) EnrollmentsForGroup(ctx context.Context, classGroupID string) ([]Enrollment, error) {
	if _, err := uuid.Parse(strings.TrimSpace(classGroupID)); err != nil {
		return nil, ErrInvalidClassGroup
	}
	return s.store.EnrollmentsForGroup(ctx, strings.TrimSpace(classGroupID))
}

// EnrollmentsForStudent returns every enrollment of a student in
// enrollment order.
func (s *Service) EnrollmentsForStudent(ctx context.Context, studentID string) ([]Enrollment, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.EnrollmentsForStudent(ctx, strings.TrimSpace(studentID))
}

// RecordPromotion persists an audited promotion decision. The ID field
// of p is ignored. Promotion records are append-only.
func (s *Service) RecordPromotion(ctx context.Context, p Promotion) (Promotion, error) {
	p.ID = uuid.NewString()
	p.StudentID = strings.TrimSpace(p.StudentID)
	p.FromGroupID = strings.TrimSpace(p.FromGroupID)
	p.ToGroupID = strings.TrimSpace(p.ToGroupID)
	p.Decision = normalizeDecision(string(p.Decision))
	p.DecidedBy = strings.TrimSpace(p.DecidedBy)
	if err := validatePromotion(p); err != nil {
		return Promotion{}, err
	}
	return s.store.RecordPromotion(ctx, p)
}

// PromotionsForStudent returns every promotion record of a student,
// oldest first.
func (s *Service) PromotionsForStudent(ctx context.Context, studentID string) ([]Promotion, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.PromotionsForStudent(ctx, strings.TrimSpace(studentID))
}

// validateRefs checks the opaque student and group references.
func validateRefs(studentID, classGroupID string) error {
	if _, err := uuid.Parse(studentID); err != nil {
		return ErrInvalidStudent
	}
	if _, err := uuid.Parse(classGroupID); err != nil {
		return ErrInvalidClassGroup
	}
	return nil
}

// ensureUnique rejects enrolling the same student twice in one group.
func (s *Service) ensureUnique(ctx context.Context, e Enrollment) error {
	all, err := s.store.EnrollmentsForGroup(ctx, e.ClassGroupID)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.StudentID == e.StudentID {
			return ErrDuplicateEnrollment
		}
	}
	return nil
}

func validatePromotion(p Promotion) error {
	if p.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(p.ID); err != nil {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(p.StudentID); err != nil {
		return ErrInvalidStudent
	}
	if _, err := uuid.Parse(p.FromGroupID); err != nil {
		return ErrInvalidClassGroup
	}
	if !p.Decision.IsValid() {
		return ErrInvalidDecision
	}
	if p.Decision == DecisionGraduate {
		if p.ToGroupID != "" {
			return ErrInvalidPromotion
		}
	} else if _, err := uuid.Parse(p.ToGroupID); err != nil {
		return ErrInvalidPromotion
	}
	if p.DecidedAt.IsZero() {
		return ErrInvalidDate
	}
	if p.DecidedBy == "" {
		return ErrInvalidActor
	}
	return nil
}

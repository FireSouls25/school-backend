package enrollments

import "context"

// Store is the persistence port for enrollments and promotion records.
// Implementations must be safe for concurrent use. Replace MemoryStore
// with the PostgreSQL adapter in production.
type Store interface {
	// AddEnrollment persists e and returns the stored enrollment.
	AddEnrollment(ctx context.Context, e Enrollment) (Enrollment, error)
	// EnrollmentsForGroup returns every enrollment of a class-group in
	// enrollment order. Alphabetical rosters are composed upstream by
	// joining student profiles.
	EnrollmentsForGroup(ctx context.Context, classGroupID string) ([]Enrollment, error)
	// EnrollmentsForStudent returns every enrollment of a student in
	// enrollment order.
	EnrollmentsForStudent(ctx context.Context, studentID string) ([]Enrollment, error)
	// RemoveEnrollment deletes the enrollment with the given id, for
	// corrections. Removing an unknown enrollment is a no-op.
	RemoveEnrollment(ctx context.Context, id string) error
	// RecordPromotion persists p and returns the stored record.
	// Promotion records are append-only: no update, no delete.
	RecordPromotion(ctx context.Context, p Promotion) (Promotion, error)
	// PromotionsForStudent returns every promotion record of a student
	// in decision order (oldest first).
	PromotionsForStudent(ctx context.Context, studentID string) ([]Promotion, error)
}

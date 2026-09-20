package classes

import "context"

// Store is the persistence port for class-groups. Implementations must be
// safe for concurrent use. Replace MemoryStore with the PostgreSQL adapter
// in production.
type Store interface {
	// Create persists g and returns the stored class-group.
	Create(ctx context.Context, g ClassGroup) (ClassGroup, error)
	// ByID returns the class-group with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (ClassGroup, error)
	// ListByYear returns every group of a school year ordered by
	// grade, then group number.
	ListByYear(ctx context.Context, schoolYearID string) ([]ClassGroup, error)
	// Update replaces the class-group with the given id.
	// Returns ErrNotFound when missing.
	Update(ctx context.Context, g ClassGroup) (ClassGroup, error)
	// Delete removes the class-group with the given id. Removing an
	// unknown group is a no-op. Groups with enrollments are protected
	// by the database (ErrHasEnrollments).
	Delete(ctx context.Context, id string) error
}

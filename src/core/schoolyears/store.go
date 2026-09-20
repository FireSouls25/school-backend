package schoolyears

import "context"

// Store is the persistence port for school years. Implementations must be
// safe for concurrent use. Replace MemoryStore with the PostgreSQL adapter
// in production.
type Store interface {
	// Create persists y and returns the stored school year.
	Create(ctx context.Context, y SchoolYear) (SchoolYear, error)
	// ByID returns the school year with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (SchoolYear, error)
	// ByYear returns the school year with the given calendar year or
	// ErrNotFound.
	ByYear(ctx context.Context, year int) (SchoolYear, error)
	// List returns every school year ordered by year, newest first.
	List(ctx context.Context) ([]SchoolYear, error)
	// Update replaces the school year with the given id.
	// Returns ErrNotFound when missing.
	Update(ctx context.Context, y SchoolYear) (SchoolYear, error)
	// Delete removes the school year with the given id. Removing an
	// unknown year is a no-op. Years with class-groups are protected by
	// the database (ErrHasClasses).
	Delete(ctx context.Context, id string) error
}

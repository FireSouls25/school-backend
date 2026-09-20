package teachers

import "context"

// Store is the persistence port for teacher profiles. Implementations must
// be safe for concurrent use. Replace MemoryStore with the PostgreSQL
// adapter in production.
type Store interface {
	// Create persists t and returns the stored profile.
	Create(ctx context.Context, t Teacher) (Teacher, error)
	// ByID returns the teacher with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (Teacher, error)
	// List returns every teacher sorted alphabetically by FullName.
	List(ctx context.Context) ([]Teacher, error)
	// Update replaces the profile of the teacher with the given id.
	// Returns ErrNotFound when missing.
	Update(ctx context.Context, t Teacher) (Teacher, error)
	// Delete removes the teacher with the given id. Removing an unknown
	// teacher is a no-op. Subject assignments cascade.
	Delete(ctx context.Context, id string) error
}

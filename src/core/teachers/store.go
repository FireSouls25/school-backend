package teachers

import "context"

// Store is the persistence port for teacher profiles. Implementations must
// be safe for concurrent use. Replace MemoryStore with the PostgreSQL
// adapter in production.
// Photo is a teacher picture as raw image bytes (JPEG or PNG).
type Photo struct {
	// Data holds the encoded image, at most MaxPhotoSize bytes.
	Data []byte
}

// MaxPhotoSize bounds the stored photo to 2 MiB.
const MaxPhotoSize = 2 << 20

type Store interface {
	// Create persists t and returns the stored profile.
	Create(ctx context.Context, t Teacher) (Teacher, error)
	// ByID returns the teacher with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (Teacher, error)
	// BySlug returns the teacher with the given URL slug or ErrNotFound.
	BySlug(ctx context.Context, slug string) (Teacher, error)
	// List returns every teacher sorted alphabetically by FullName.
	List(ctx context.Context) ([]Teacher, error)
	// Update replaces the profile of the teacher with the given id.
	// Returns ErrNotFound when missing.
	Update(ctx context.Context, t Teacher) (Teacher, error)
	// Delete removes the teacher with the given id. Removing an unknown
	// teacher is a no-op. Subject assignments cascade.
	Delete(ctx context.Context, id string) error
	// PutPhoto stores or replaces the photo of the teacher. Returns
	// ErrNotFound when missing.
	PutPhoto(ctx context.Context, teacherID string, photo Photo) error
	// Photo returns the photo of the teacher; empty Data when none stored.
	Photo(ctx context.Context, teacherID string) (Photo, error)
}

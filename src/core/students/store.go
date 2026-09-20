package students

import "context"

// Photo is a student picture as raw image bytes (JPEG or PNG).
type Photo struct {
	// Data holds the encoded image, at most MaxPhotoSize bytes.
	Data []byte
}

// Store is the persistence port for student profiles. Implementations must
// be safe for concurrent use. Replace MemoryStore with the PostgreSQL
// adapter in production.
type Store interface {
	// Create persists s and returns the stored profile.
	Create(ctx context.Context, s Student) (Student, error)
	// ByID returns the student with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (Student, error)
	// ListByClass returns every student in classID sorted alphabetically
	// by FullName.
	ListByClass(ctx context.Context, classID string) ([]Student, error)
	// Update replaces the mutable fields (names, surnames, class) of the
	// student with the given id. Returns ErrNotFound when missing.
	Update(ctx context.Context, s Student) (Student, error)
	// Delete removes the student with the given id. Removing an unknown
	// student is a no-op.
	Delete(ctx context.Context, id string) error
	// PutPhoto stores or replaces the photo of the student. Returns
	// ErrNotFound when the student does not exist.
	PutPhoto(ctx context.Context, studentID string, photo Photo) error
	// Photo returns the photo of the student; empty Data when none stored.
	Photo(ctx context.Context, studentID string) (Photo, error)
}

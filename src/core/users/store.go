package users

import (
	"context"
	"time"
)

// Photo is the account picture. It travels alone, never inside the
// account row, so reading or listing accounts never drags image bytes.
type Photo struct {
	// Data holds the encoded image, at most MaxPhotoSize bytes.
	Data []byte
}

// MaxPhotoSize bounds the stored photo to 2 MiB.
const MaxPhotoSize = 2 << 20

// Store is the persistence port for user accounts. Implementations must
// be safe for concurrent use. Username and email lookups are
// case-insensitive. Replace MemoryStore with the PostgreSQL adapter in
// production.
type Store interface {
	// Create persists u (including its password hash) and returns the
	// stored account. Implementations enforce unique usernames and
	// unique non-empty emails, reporting ErrDuplicateUsername or
	// ErrDuplicateEmail.
	Create(ctx context.Context, u User) (User, error)
	// ByID returns the user with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (User, error)
	// ByUsername returns the user with the given username
	// (case-insensitive) or ErrNotFound.
	ByUsername(ctx context.Context, username string) (User, error)
	// ByEmail returns the user with the given email (case-insensitive)
	// or ErrNotFound.
	ByEmail(ctx context.Context, email string) (User, error)
	// Update persists the username, email, active flag and teacher link
	// of the user with the given id. It never touches the password hash;
	// use SetPasswordHash for that. Returns ErrNotFound when missing and
	// the duplicate errors on collisions with other accounts.
	Update(ctx context.Context, u User) (User, error)
	// SetPasswordHash replaces the password hash of the user with the
	// given id. Returns ErrNotFound when missing.
	SetPasswordHash(ctx context.Context, id, hash string) error
	// BumpTokenVersion atomically increments the token version of the user
	// with the given id and returns the new version. Returns ErrNotFound
	// when missing. Callers bump after password changes and active flips
	// so previously minted bearers stop validating.
	BumpTokenVersion(ctx context.Context, id string) (int64, error)
	// SoftDelete stamps DeletedAt on the user with the given id and
	// disables the login, instead of removing the row, so history keeps
	// resolving and Restore can bring the account back inside the
	// retention window. Removing an unknown user is a no-op; deleting an
	// already deleted one reports ErrAlreadyDeleted.
	SoftDelete(ctx context.Context, id string, at time.Time) error
	// Restore clears the soft-delete mark, bringing the account back.
	// Returns ErrNotFound when missing.
	Restore(ctx context.Context, id string) error
	// PurgeBefore physically removes users soft-deleted before cutoff
	// and returns their ids so callers can clean related grants. It is
	// the only path that truly deletes an account.
	PurgeBefore(ctx context.Context, cutoff time.Time) ([]string, error)
	// List returns every account in store order. Deleted accounts are
	// included only when includeDeleted is true.
	List(ctx context.Context, includeDeleted bool) ([]User, error)
	// ForTeacher returns every login linked to the given teacher profile.
	// It backs the "made by this teacher" history on the teacher file.
	ForTeacher(ctx context.Context, teacherID string) ([]User, error)
	// Photo returns the stored picture of the user with the given id or
	// ErrNotFound when the account does not exist; Data is empty when the
	// account has none yet.
	Photo(ctx context.Context, id string) (Photo, error)
	// SetPhoto stores (or clears, when Data is empty) the picture of the
	// user with the given id, returning ErrNotFound when missing.
	SetPhoto(ctx context.Context, id string, photo Photo) error
}

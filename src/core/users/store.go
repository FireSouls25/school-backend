package users

import "context"

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
	// Update persists the username, email and active flag of the user
	// with the given id. It never touches the password hash; use
	// SetPasswordHash for that. Returns ErrNotFound when missing and
	// the duplicate errors on collisions with other accounts.
	Update(ctx context.Context, u User) (User, error)
	// SetPasswordHash replaces the password hash of the user with the
	// given id. Returns ErrNotFound when missing.
	SetPasswordHash(ctx context.Context, id, hash string) error
	// Delete removes the user with the given id. Removing an unknown
	// user is a no-op.
	Delete(ctx context.Context, id string) error
}

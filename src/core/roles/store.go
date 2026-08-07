package roles

import "context"

// Store is the persistence port for role assignments. Implementations must
// be safe for concurrent use. A subject is identified by an opaque id so this
// package stays decoupled from the users/auth features. Replace MemoryStore
// with a PostgreSQL adapter in production.
type Store interface {
	// AssignRole grants role to subjectID, overwriting any previous grant.
	AssignRole(ctx context.Context, subjectID string, role Role) error
	// RemoveRole revokes role from subjectID. Removing an absent role is a no-op.
	RemoveRole(ctx context.Context, subjectID string, role Role) error
	// RolesFor returns the roles held by subjectID, sorted, or an empty slice
	// when the subject is unknown.
	RolesFor(ctx context.Context, subjectID string) ([]Role, error)
}

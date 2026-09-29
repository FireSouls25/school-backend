// Package users keeps the login accounts of the system: a unique username,
// an optional unique email, a bcrypt password hash and an active flag.
//
// The User ID is the subject identifier consumed by the roles capability:
// granting a role to a user means assigning it to User.ID. Users never
// imports roles; the composition root wires both together.
package users

import (
	"time"
)

// User is one login account.
type User struct {
	// ID is the unique, immutable UUID of the user. It doubles as the
	// subject identifier for role assignments.
	ID string
	// Username is the unique login name (e.g. "admin",
	// "carlos.mendoza"). Stored lowercase.
	Username string
	// Email is the optional unique email address. Empty means none.
	Email string
	// PasswordHash is the bcrypt hash of the password. It is never
	// serialized (json:"-") and public Service methods always return
	// users with an empty hash; only Authenticate and SetPassword
	// touch it internally through the Store.
	PasswordHash string `json:"-"`
	// Active reports whether the user may log in. Disabling flips it to
	// false instead of deleting the account, so history keeps resolving.
	Active bool
	// CreatedAt is the account creation time.
	CreatedAt time.Time
	// UpdatedAt is the last modification time.
	UpdatedAt time.Time
}

// Sanitized returns a copy of u without the password hash, safe to hand
// to callers and (future) HTTP handlers.
func (u User) Sanitized() User {
	u.PasswordHash = ""
	return u
}

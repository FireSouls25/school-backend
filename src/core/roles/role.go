package roles

import (
	"fmt"
	"strings"
)

// Role identifies a position within the system.
// Role values are stable identifiers; user-facing names live in the i18n
// catalog keyed by MessageKey, not in this codebase.
type Role string

const (
	// RoleStudent is a student enrolled in the school.
	RoleStudent Role = "student"
	// RoleTeacher is a member of the teaching staff.
	RoleTeacher Role = "teacher"
	// RoleAdmin manages the system and its users.
	RoleAdmin Role = "admin"
)

// knownRoles is the canonical, alphabetically sorted list of roles.
var knownRoles = [...]Role{RoleAdmin, RoleStudent, RoleTeacher}

// Parse converts a case-insensitive, whitespace-trimmed string into a Role.
func Parse(s string) (Role, error) {
	r := Role(strings.ToLower(strings.TrimSpace(s)))
	if !r.IsValid() {
		return "", fmt.Errorf("roles: parse %q: %w", s, ErrUnknownRole)
	}
	return r, nil
}

// IsValid reports whether r is one of the known roles.
func (r Role) IsValid() bool {
	switch r {
	case RoleStudent, RoleTeacher, RoleAdmin:
		return true
	}
	return false
}

// String returns the stable identifier of r.
func (r Role) String() string {
	return string(r)
}

// MessageKey returns the i18n catalog key holding the user-facing name.
func (r Role) MessageKey() string {
	return "role." + r.String()
}

// KnownRoles returns a fresh, alphabetically sorted slice of all roles.
func KnownRoles() []Role {
	out := make([]Role, len(knownRoles))
	copy(out, knownRoles[:])
	return out
}

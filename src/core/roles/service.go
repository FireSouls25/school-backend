package roles

import (
	"context"
	"strings"
)

// Service applies role policy and persists assignments through a Store.
type Service struct {
	store Store
}

// Authorizer is the port downstream features (students, incidents,
// statistics) consume to check whether a subject may perform an action.
// It is implemented by Service and injected into feature handlers.
type Authorizer interface {
	// Can reports whether subjectID holds a role granting the permission.
	// Unknown subjects are denied.
	Can(ctx context.Context, subjectID string, permission Permission) (bool, error)
}

// RoleGetter exposes a subject's assigned roles to downstream features.
type RoleGetter interface {
	// Roles returns the roles held by subjectID in stable sorted order.
	Roles(ctx context.Context, subjectID string) ([]Role, error)
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Assign validates the input and grants the role to the subject.
func (s *Service) Assign(ctx context.Context, subjectID string, role Role) error {
	if err := validateAssignment(subjectID, role); err != nil {
		return err
	}
	return s.store.AssignRole(ctx, subjectID, role)
}

// Remove validates the input and revokes the role from the subject.
func (s *Service) Remove(ctx context.Context, subjectID string, role Role) error {
	if err := validateAssignment(subjectID, role); err != nil {
		return err
	}
	return s.store.RemoveRole(ctx, subjectID, role)
}

// Roles implements RoleGetter.
func (s *Service) Roles(ctx context.Context, subjectID string) ([]Role, error) {
	if strings.TrimSpace(subjectID) == "" {
		return nil, ErrInvalidSubjectID
	}
	return s.store.RolesFor(ctx, subjectID)
}

// HasRole reports whether the subject holds the given role.
func (s *Service) HasRole(ctx context.Context, subjectID string, role Role) (bool, error) {
	roles, err := s.Roles(ctx, subjectID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

// Can implements Authorizer.
func (s *Service) Can(ctx context.Context, subjectID string, permission Permission) (bool, error) {
	roles, err := s.Roles(ctx, subjectID)
	if err != nil {
		return false, err
	}
	for _, r := range roles {
		if r.HasPermission(permission) {
			return true, nil
		}
	}
	return false, nil
}

func validateAssignment(subjectID string, role Role) error {
	if strings.TrimSpace(subjectID) == "" {
		return ErrInvalidSubjectID
	}
	if !role.IsValid() {
		return ErrUnknownRole
	}
	return nil
}

// Compile-time checks that Service satisfies the ports it exposes.
var (
	_ Authorizer = (*Service)(nil)
	_ RoleGetter = (*Service)(nil)
)

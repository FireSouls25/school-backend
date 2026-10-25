package roles_test

import (
	"testing"

	"grade/src/core/roles"
)

// The secretary assists with records, schedules and lockout: a curated
// subset of the admin console, never user management, years or incidents.
func TestSecretaryPermissions(t *testing.T) {
	granted := map[roles.Permission]bool{}
	for _, p := range roles.RoleSecretary.Permissions() {
		granted[p] = true
	}
	for _, want := range []roles.Permission{
		roles.PermissionViewStudents,
		roles.PermissionManageStudents,
		roles.PermissionViewClassStats,
		roles.PermissionRecordAttendance,
		roles.PermissionManageTeachers,
		roles.PermissionViewTeachers,
		roles.PermissionBlockAccess,
		roles.PermissionManageSchedules,
		roles.PermissionViewWarnings,
	} {
		if !granted[want] {
			t.Errorf("secretary lacks %q", want)
		}
	}
	for _, denied := range []roles.Permission{
		roles.PermissionManageUsers,
		roles.PermissionManageRoles,
		roles.PermissionManageSystem,
		roles.PermissionRecordIncidents,
	} {
		if granted[denied] {
			t.Errorf("secretary must not hold %q", denied)
		}
	}
}

func TestSecretaryParses(t *testing.T) {
	r, err := roles.Parse("secretary")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if r != roles.RoleSecretary {
		t.Errorf("parsed = %q, want secretary", r)
	}
	if !r.IsValid() {
		t.Error("secretary must be valid")
	}
	found := false
	for _, r := range roles.KnownRoles() {
		if r == roles.RoleSecretary {
			found = true
		}
	}
	if !found {
		t.Error("secretary missing from known roles")
	}
}

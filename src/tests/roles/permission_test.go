package roles_test

import (
	"reflect"
	"testing"

	"grade/src/core/roles"
)

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		role roles.Role
		want []roles.Permission
	}{
		{
			role: roles.RoleStudent,
			want: []roles.Permission{roles.PermissionViewOwnHistory},
		},
		{
			role: roles.RoleTeacher,
			want: []roles.Permission{
				roles.PermissionRecordIncidents,
				roles.PermissionViewClassStats,
				roles.PermissionViewStudents,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.role.String(), func(t *testing.T) {
			got := tt.role.Permissions()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s.Permissions() = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestRolePermissionsSorted(t *testing.T) {
	for _, r := range roles.KnownRoles() {
		ps := r.Permissions()
		for i := 1; i < len(ps); i++ {
			if ps[i] < ps[i-1] {
				t.Errorf("%s.Permissions() not sorted: %v", r, ps)
			}
		}
	}
}

func TestAdminGrantsEveryPermission(t *testing.T) {
	admin := roles.RoleAdmin.Permissions()
	for _, p := range roles.AllPermissions() {
		if !roles.RoleAdmin.HasPermission(p) {
			t.Errorf("admin does not grant %q", p)
		}
	}
	if !reflect.DeepEqual(admin, roles.AllPermissions()) {
		t.Errorf("admin permissions %v != AllPermissions() %v", admin, roles.AllPermissions())
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		name string
		role roles.Role
		perm roles.Permission
		want bool
	}{
		{name: "student own history", role: roles.RoleStudent, perm: roles.PermissionViewOwnHistory, want: true},
		{name: "student cannot view students", role: roles.RoleStudent, perm: roles.PermissionViewStudents, want: false},
		{name: "student cannot manage users", role: roles.RoleStudent, perm: roles.PermissionManageUsers, want: false},
		{name: "teacher views students", role: roles.RoleTeacher, perm: roles.PermissionViewStudents, want: true},
		{name: "teacher records incidents", role: roles.RoleTeacher, perm: roles.PermissionRecordIncidents, want: true},
		{name: "teacher cannot manage roles", role: roles.RoleTeacher, perm: roles.PermissionManageRoles, want: false},
		{name: "teacher cannot see own history", role: roles.RoleTeacher, perm: roles.PermissionViewOwnHistory, want: false},
		{name: "admin manages system", role: roles.RoleAdmin, perm: roles.PermissionManageSystem, want: true},
		{name: "admin manages users", role: roles.RoleAdmin, perm: roles.PermissionManageUsers, want: true},
		{name: "admin views students", role: roles.RoleAdmin, perm: roles.PermissionViewStudents, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.role.HasPermission(tt.perm); got != tt.want {
				t.Errorf("%s.HasPermission(%q) = %v, want %v", tt.role, tt.perm, got, tt.want)
			}
		})
	}
}

func TestUnknownRoleHasNoPermissions(t *testing.T) {
	if roles.Role("principal").HasPermission(roles.PermissionViewStudents) {
		t.Error("unknown role should grant no permissions")
	}
	if len(roles.Role("principal").Permissions()) != 0 {
		t.Error("unknown role should expose no permissions")
	}
}

func TestAllPermissionsUnique(t *testing.T) {
	seen := make(map[roles.Permission]bool)
	for _, p := range roles.AllPermissions() {
		if seen[p] {
			t.Errorf("duplicate permission %q", p)
		}
		seen[p] = true
	}
}

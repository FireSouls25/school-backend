package roles

import "sort"

// Permission is a fine-grained action a subject may perform.
// A role grants a fixed set of permissions (see the permission matrix).
type Permission string

const (
	// PermissionViewOwnHistory lets a student see their own record.
	PermissionViewOwnHistory Permission = "view-own-history"
	// PermissionViewStudents lets a teacher view student information.
	PermissionViewStudents Permission = "view-students"
	// PermissionRecordIncidents lets a teacher register faults and misbehavior.
	PermissionRecordIncidents Permission = "record-incidents"
	// PermissionViewClassStats lets a teacher see class statistics.
	PermissionViewClassStats Permission = "view-class-statistics"
	// PermissionManageStudents lets an admin manage student records.
	PermissionManageStudents Permission = "manage-students"
	// PermissionManageUsers lets an admin manage system users.
	PermissionManageUsers Permission = "manage-users"
	// PermissionManageRoles lets an admin assign roles.
	PermissionManageRoles Permission = "manage-roles"
	// PermissionManageSystem lets an admin manage the whole system.
	PermissionManageSystem Permission = "manage-system"
)

// permissionCatalog is the complete set of permissions the system knows.
var permissionCatalog = []Permission{
	PermissionViewOwnHistory,
	PermissionViewStudents,
	PermissionRecordIncidents,
	PermissionViewClassStats,
	PermissionManageStudents,
	PermissionManageUsers,
	PermissionManageRoles,
	PermissionManageSystem,
}

// rolePermissions maps each role to the permissions it grants.
// RoleAdmin is expanded at init to the union of all permissions.
var rolePermissions = func() map[Role][]Permission {
	base := map[Role][]Permission{
		RoleStudent: {
			PermissionViewOwnHistory,
		},
		RoleTeacher: {
			PermissionViewStudents,
			PermissionRecordIncidents,
			PermissionViewClassStats,
		},
	}
	base[RoleAdmin] = append([]Permission(nil), permissionCatalog...)
	return base
}()

// Permissions returns the permissions granted by r as a fresh, sorted slice.
func (r Role) Permissions() []Permission {
	ps, ok := rolePermissions[r]
	if !ok {
		return []Permission{}
	}
	out := append([]Permission(nil), ps...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// HasPermission reports whether r grants the given permission.
func (r Role) HasPermission(p Permission) bool {
	for _, granted := range rolePermissions[r] {
		if granted == p {
			return true
		}
	}
	return false
}

// AllPermissions returns every permission the system defines, sorted.
func AllPermissions() []Permission {
	out := append([]Permission(nil), permissionCatalog...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// String returns the stable identifier of p.
func (p Permission) String() string {
	return string(p)
}

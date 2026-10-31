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
	// PermissionRecordAttendance lets a teacher take and correct roll calls.
	PermissionRecordAttendance Permission = "record-attendance"
	// PermissionViewClassStats lets a teacher see class statistics.
	PermissionViewClassStats Permission = "view-class-statistics"
	// PermissionManageStudents lets an admin manage student records.
	PermissionManageStudents Permission = "manage-students"
	// PermissionManageTeachers lets an admin manage teacher records and
	// take their daily roll.
	PermissionManageTeachers Permission = "manage-teachers"
	// PermissionManageUsers lets an admin manage system users.
	PermissionManageUsers Permission = "manage-users"
	// PermissionManageRoles lets an admin assign roles.
	PermissionManageRoles Permission = "manage-roles"
	// PermissionManageSystem lets an admin manage the whole system.
	PermissionManageSystem Permission = "manage-system"
	// PermissionViewTeachers lets a secretary see teacher records in
	// order to locate the lockout switch. It grants no edit.
	PermissionViewTeachers Permission = "view-teachers"
	// PermissionBlockAccess lets a secretary disable or re-enable a
	// login. Admin accounts stay out: only manage-users touches them.
	PermissionBlockAccess Permission = "block-access"
	// PermissionRecordDefenses lets admin and secretary record a
	// student's written defense against a fault ("descargo"). Teachers
	// don't grant them.
	PermissionRecordDefenses Permission = "record-defenses"
	// PermissionManageSchedules covers the timetable, its subjects and
	// their assignments, the same for admins and secretaries.
	PermissionManageSchedules Permission = "manage-schedules"
	// PermissionViewWarnings lets a secretary read the incident history
	// without registering or correcting anything.
	PermissionViewWarnings Permission = "view-warnings"
	// PermissionManagePromotions covers the year-end flow both admin and
	// secretary run: promotion decisions, enrollments and creating the
	// next year salons. Closing the year stays manage-users.
	PermissionManagePromotions Permission = "manage-promotions"
	// PermissionManageOwners is the exclusive owner capability: deleting
	// or restoring accounts, removing the admin or owner role from anyone,
	// and touching admin accounts (deactivation included). Admins hold
	// every other permission but never this one.
	PermissionManageOwners Permission = "manage-owners"
)

// permissionCatalog is the complete set of permissions the system knows.
var permissionCatalog = []Permission{
	PermissionViewOwnHistory,
	PermissionViewStudents,
	PermissionRecordIncidents,
	PermissionRecordAttendance,
	PermissionViewClassStats,
	PermissionManageStudents,
	PermissionManageTeachers,
	PermissionManageUsers,
	PermissionManageRoles,
	PermissionManageSystem,
	PermissionViewTeachers,
	PermissionBlockAccess,
	PermissionManageSchedules,
	PermissionViewWarnings,
	PermissionManagePromotions,
	PermissionManageOwners,
	PermissionRecordDefenses,
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
			PermissionRecordAttendance,
			PermissionViewClassStats,
		},
	}
	base[RoleSecretary] = []Permission{
		PermissionViewStudents,
		PermissionManageStudents,
		PermissionViewClassStats,
		PermissionRecordAttendance,
		PermissionManageTeachers,
		PermissionViewTeachers,
		PermissionBlockAccess,
		PermissionManageSchedules,
		PermissionViewWarnings,
		PermissionManagePromotions,
		PermissionRecordDefenses,
	}
	base[RoleAdmin] = append([]Permission(nil), permissionCatalog...)
	// The admin union above includes manage-owners by construction:
	// strip it so only owners hold the exclusive capability.
	base[RoleAdmin] = exclude(base[RoleAdmin], PermissionManageOwners)
	base[RoleOwner] = append([]Permission(nil), permissionCatalog...)
	return base
}()

// exclude returns ps without p, preserving order.
func exclude(ps []Permission, p Permission) []Permission {
	out := ps[:0]
	for _, q := range ps {
		if q != p {
			out = append(out, q)
		}
	}
	return out
}

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

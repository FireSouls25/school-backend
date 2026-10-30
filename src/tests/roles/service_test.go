package roles_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"grade/src/core/roles"
)

func newService() *roles.Service {
	return roles.NewService(roles.NewMemoryStore())
}

func TestServiceAssignValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	t.Run("empty subject", func(t *testing.T) {
		err := svc.Assign(ctx, "  ", roles.RoleStudent)
		if !errors.Is(err, roles.ErrInvalidSubjectID) {
			t.Errorf("Assign error = %v, want ErrInvalidSubjectID", err)
		}
	})

	t.Run("unknown role", func(t *testing.T) {
		err := svc.Assign(ctx, "s1", roles.Role("principal"))
		if !errors.Is(err, roles.ErrUnknownRole) {
			t.Errorf("Assign error = %v, want ErrUnknownRole", err)
		}
	})
}

func TestServiceAssignAndRoles(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.Assign(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	got, err := svc.Roles(ctx, "s1")
	if err != nil {
		t.Fatalf("Roles: %v", err)
	}
	if want := []roles.Role{roles.RoleTeacher}; !reflect.DeepEqual(got, want) {
		t.Errorf("Roles = %v, want %v", got, want)
	}

	has, err := svc.HasRole(ctx, "s1", roles.RoleTeacher)
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if !has {
		t.Error("HasRole should be true after Assign")
	}
}

func TestServiceRemove(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.Assign(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := svc.Remove(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	has, err := svc.HasRole(ctx, "s1", roles.RoleTeacher)
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if has {
		t.Error("HasRole should be false after Remove")
	}
}

func TestServiceRolesUnknownSubject(t *testing.T) {
	svc := newService()
	roles_, err := svc.Roles(context.Background(), "nobody")
	if err != nil {
		t.Fatalf("Roles: %v", err)
	}
	if len(roles_) != 0 {
		t.Errorf("Roles = %v, want empty", roles_)
	}
}

func TestServiceRolesInvalidSubject(t *testing.T) {
	svc := newService()
	if _, err := svc.Roles(context.Background(), ""); !errors.Is(err, roles.ErrInvalidSubjectID) {
		t.Errorf("Roles error = %v, want ErrInvalidSubjectID", err)
	}
}

func TestServiceCan(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	tests := []struct {
		name       string
		subject    string
		role       roles.Role
		permission roles.Permission
		want       bool
	}{
		{name: "teacher views students", subject: "t1", role: roles.RoleTeacher, permission: roles.PermissionViewStudents, want: true},
		{name: "teacher records incidents", subject: "t1", role: roles.RoleTeacher, permission: roles.PermissionRecordIncidents, want: true},
		{name: "teacher cannot manage roles", subject: "t1", role: roles.RoleTeacher, permission: roles.PermissionManageRoles, want: false},
		{name: "student views own history", subject: "st1", role: roles.RoleStudent, permission: roles.PermissionViewOwnHistory, want: true},
		{name: "student cannot view all students", subject: "st1", role: roles.RoleStudent, permission: roles.PermissionViewStudents, want: false},
		{name: "admin manages users", subject: "a1", role: roles.RoleAdmin, permission: roles.PermissionManageUsers, want: true},
		{name: "admin views students", subject: "a1", role: roles.RoleAdmin, permission: roles.PermissionViewStudents, want: true},
		{name: "unknown subject denied", subject: "ghost", role: "", permission: roles.PermissionViewStudents, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.role != "" {
				if err := svc.Assign(ctx, tt.subject, tt.role); err != nil {
					t.Fatalf("Assign: %v", err)
				}
			}
			got, err := svc.Can(ctx, tt.subject, tt.permission)
			if err != nil {
				t.Fatalf("Can: %v", err)
			}
			if got != tt.want {
				t.Errorf("Can(%q, %q) = %v, want %v", tt.subject, tt.permission, got, tt.want)
			}
		})
	}
}

func TestServiceCanAnyAssignedRoleGrants(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.Assign(ctx, "s1", roles.RoleStudent); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := svc.Assign(ctx, "s1", roles.RoleTeacher); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	got, err := svc.Can(ctx, "s1", roles.PermissionViewStudents)
	if err != nil {
		t.Fatalf("Can: %v", err)
	}
	if !got {
		t.Error("teacher role should grant view-students even when student role is also assigned")
	}
}

func TestServiceCanInvalidSubject(t *testing.T) {
	svc := newService()
	if _, err := svc.Can(context.Background(), "", roles.PermissionViewStudents); !errors.Is(err, roles.ErrInvalidSubjectID) {
		t.Errorf("Can error = %v, want ErrInvalidSubjectID", err)
	}
}

func TestServiceSubjectsWithRoleAndRemoveAll(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if err := svc.Assign(ctx, "s1", roles.RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	if err := svc.Assign(ctx, "s2", roles.RoleOwner); err != nil {
		t.Fatalf("Assign owner: %v", err)
	}
	if err := svc.Assign(ctx, "s2", roles.RoleAdmin); err != nil {
		t.Fatalf("Assign admin: %v", err)
	}
	if err := svc.Assign(ctx, "s3", roles.RoleAdmin); err != nil {
		t.Fatalf("Assign admin: %v", err)
	}

	got, err := svc.SubjectsWithRole(ctx, roles.RoleOwner)
	if err != nil {
		t.Fatalf("SubjectsWithRole: %v", err)
	}
	if want := []string{"s1", "s2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SubjectsWithRole(owner) = %v, want %v", got, want)
	}
	if _, err := svc.SubjectsWithRole(ctx, roles.Role("nope")); !errors.Is(err, roles.ErrUnknownRole) {
		t.Errorf("SubjectsWithRole unknown error = %v, want ErrUnknownRole", err)
	}

	if err := svc.RemoveAll(ctx, "s2"); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	has, err := svc.HasRole(ctx, "s2", roles.RoleOwner)
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if has {
		t.Error("HasRole should be false after RemoveAll")
	}
	got, err = svc.SubjectsWithRole(ctx, roles.RoleOwner)
	if err != nil {
		t.Fatalf("SubjectsWithRole: %v", err)
	}
	if want := []string{"s1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SubjectsWithRole(owner) = %v, want %v", got, want)
	}
	// Admin grants must survive an unrelated RemoveAll.
	has, err = svc.HasRole(ctx, "s3", roles.RoleAdmin)
	if err != nil {
		t.Fatalf("HasRole: %v", err)
	}
	if !has {
		t.Error("HasRole should stay true for untouched subject")
	}
}

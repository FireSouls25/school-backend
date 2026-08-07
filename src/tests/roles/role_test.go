package roles_test

import (
	"errors"
	"reflect"
	"testing"

	"grade/src/core/roles"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  roles.Role
	}{
		{name: "student", input: "student", want: roles.RoleStudent},
		{name: "teacher", input: "teacher", want: roles.RoleTeacher},
		{name: "admin", input: "admin", want: roles.RoleAdmin},
		{name: "uppercase", input: "ADMIN", want: roles.RoleAdmin},
		{name: "mixed case", input: "Teacher", want: roles.RoleTeacher},
		{name: "surrounding spaces", input: "  student  ", want: roles.RoleStudent},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := roles.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseUnknown(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "blank", input: "   "},
		{name: "unknown word", input: "principal"},
		{name: "misspelled", input: "studnet"},
		{name: "compound", input: "student-2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := roles.Parse(tt.input)
			if !errors.Is(err, roles.ErrUnknownRole) {
				t.Fatalf("Parse(%q) error = %v, want ErrUnknownRole", tt.input, err)
			}
		})
	}
}

func TestRoleIsValid(t *testing.T) {
	for _, r := range roles.KnownRoles() {
		if !r.IsValid() {
			t.Errorf("%q should be valid", r)
		}
	}
	for _, invalid := range []roles.Role{"", "principal", "admin2"} {
		if invalid.IsValid() {
			t.Errorf("%q should not be valid", invalid)
		}
	}
}

func TestRoleString(t *testing.T) {
	want := map[roles.Role]string{
		roles.RoleStudent: "student",
		roles.RoleTeacher: "teacher",
		roles.RoleAdmin:   "admin",
	}
	for r, s := range want {
		if r.String() != s {
			t.Errorf("%s.String() = %q, want %q", r, r.String(), s)
		}
	}
}

func TestRoleMessageKey(t *testing.T) {
	if got := roles.RoleStudent.MessageKey(); got != "role.student" {
		t.Errorf("MessageKey = %q, want %q", got, "role.student")
	}
}

func TestKnownRolesSorted(t *testing.T) {
	want := []roles.Role{roles.RoleAdmin, roles.RoleStudent, roles.RoleTeacher}
	got := roles.KnownRoles()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KnownRoles() = %v, want %v", got, want)
	}
}

func TestKnownRolesReturnsFreshSlice(t *testing.T) {
	first := roles.KnownRoles()
	first[0] = ""
	second := roles.KnownRoles()
	if second[0] == "" {
		t.Error("mutating the returned slice changed internal state")
	}
}

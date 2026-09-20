package classes_test

import (
	"context"
	"errors"
	"testing"

	"grade/src/core/classes"
)

func newService() *classes.Service {
	return classes.NewService(classes.NewMemoryStore())
}

const yearID = "11111111-1111-1111-1111-111111111111"

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name  string
		group classes.ClassGroup
		want  error
	}{
		{"bad year id", classes.ClassGroup{SchoolYearID: "nope", Grade: 7, GroupNo: 1}, classes.ErrInvalidSchoolYear},
		{"grade zero", classes.ClassGroup{SchoolYearID: yearID, Grade: 0, GroupNo: 1}, classes.ErrInvalidGrade},
		{"grade twelve", classes.ClassGroup{SchoolYearID: yearID, Grade: 12, GroupNo: 1}, classes.ErrInvalidGrade},
		{"group zero", classes.ClassGroup{SchoolYearID: yearID, Grade: 7, GroupNo: 0}, classes.ErrInvalidGroup},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, tc.group); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestLabel(t *testing.T) {
	g := classes.ClassGroup{Grade: 7, GroupNo: 1}
	if got, want := g.Label(), "7-1"; got != want {
		t.Errorf("Label = %q, want %q", got, want)
	}
}

func TestDuplicateAndOrdering(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	mk := func(grade, group int) classes.ClassGroup {
		return classes.ClassGroup{SchoolYearID: yearID, Grade: grade, GroupNo: group}
	}
	for _, g := range [][2]int{{9, 2}, {7, 1}, {9, 1}, {1, 3}} {
		if _, err := svc.Create(ctx, mk(g[0], g[1])); err != nil {
			t.Fatalf("Create %v: %v", g, err)
		}
	}
	if _, err := svc.Create(ctx, mk(7, 1)); !errors.Is(err, classes.ErrDuplicateClass) {
		t.Errorf("duplicate error = %v, want ErrDuplicateClass", err)
	}

	// Same label in another year is fine.
	other := mk(7, 1)
	other.SchoolYearID = "22222222-2222-2222-2222-222222222222"
	if _, err := svc.Create(ctx, other); err != nil {
		t.Fatalf("Create same label other year: %v", err)
	}

	list, err := svc.ListByYear(ctx, yearID)
	if err != nil {
		t.Fatalf("ListByYear: %v", err)
	}
	want := []string{"1-3", "7-1", "9-1", "9-2"}
	if len(list) != len(want) {
		t.Fatalf("len(list) = %d, want %d", len(list), len(want))
	}
	for i := range want {
		if list[i].Label() != want[i] {
			t.Errorf("list[%d] = %q, want %q", i, list[i].Label(), want[i])
		}
	}

	if _, err := svc.ListByYear(ctx, "nope"); !errors.Is(err, classes.ErrInvalidSchoolYear) {
		t.Errorf("bad year error = %v, want ErrInvalidSchoolYear", err)
	}
}

func TestUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	g, err := svc.Create(ctx, classes.ClassGroup{SchoolYearID: yearID, Grade: 6, GroupNo: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Removing 6-5 next year is just not creating it; renaming this one works.
	g.GroupNo = 4
	upd, err := svc.Update(ctx, g)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.Label() != "6-4" {
		t.Errorf("Update = %+v, want 6-4", upd)
	}

	ghost := classes.ClassGroup{ID: "00000000-0000-0000-0000-000000000001", SchoolYearID: yearID, Grade: 1, GroupNo: 1}
	if _, err := svc.Update(ctx, ghost); !errors.Is(err, classes.ErrNotFound) {
		t.Errorf("Update unknown error = %v, want ErrNotFound", err)
	}

	if err := svc.Delete(ctx, g.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, g.ID); !errors.Is(err, classes.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
}

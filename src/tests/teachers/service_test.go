package teachers_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"grade/src/core/teachers"
)

func newService() *teachers.Service {
	return teachers.NewService(teachers.NewMemoryStore())
}

// validProfile returns a valid teacher profile exercising every field.
func validProfile() teachers.Teacher {
	return teachers.Teacher{
		Names: "Carlos", Surnames: "Mendoza Ruiz",
		DocumentID: "79888777", Phone: "3205556677", Address: "Carrera 15 # 30-10",
		Birthplace: "Medellín", Birthdate: time.Date(1985, 6, 20, 0, 0, 0, 0, time.UTC),
		Email:             "carlos.mendoza@example.com",
		MedicalConditions: "Hipertensión controlada",
		HomeroomClassID:   "9-1",
	}
}

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*teachers.Teacher)
		want   error
	}{
		{"missing names", func(tc *teachers.Teacher) { tc.Names = "  " }, teachers.ErrInvalidName},
		{"missing surnames", func(tc *teachers.Teacher) { tc.Surnames = "" }, teachers.ErrInvalidName},
		{"missing document", func(tc *teachers.Teacher) { tc.DocumentID = "" }, teachers.ErrInvalidDocument},
		{"bad email", func(tc *teachers.Teacher) { tc.Email = "not-an-email" }, teachers.ErrInvalidEmail},
		{"future birthdate", func(tc *teachers.Teacher) {
			tc.Birthdate = time.Now().Add(24 * time.Hour)
		}, teachers.ErrInvalidBirth},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := validProfile()
			tc.mutate(&p)
			if _, err := svc.Create(ctx, p); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestServiceCreateAssignsUUID(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Create(ctx, validProfile())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	p := validProfile()
	p.Names, p.DocumentID = "Ana", "51999111"
	b, err := svc.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" || a.ID == b.ID {
		t.Errorf("IDs not unique: %q vs %q", a.ID, b.ID)
	}
	got, err := svc.ByID(ctx, a.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got.DocumentID != "79888777" || !got.IsHomeroomDirector() || got.HomeroomClassID != "9-1" {
		t.Errorf("ByID = %+v", got)
	}
	if !got.Active {
		t.Errorf("new teacher Active = false, want true")
	}

	// Create ignores an incoming inactive flag.
	p.Active = false
	c, err := svc.Create(ctx, p)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !c.Active {
		t.Errorf("Create with Active=false stayed inactive, want forced active")
	}
}

func TestHomeroomDirectorFlag(t *testing.T) {
	p := validProfile()
	if !p.IsHomeroomDirector() {
		t.Error("teacher with HomeroomClassID should direct a class")
	}
	p.HomeroomClassID = "  "
	if p.IsHomeroomDirector() {
		t.Error("teacher without HomeroomClassID should not direct a class")
	}
}

func TestFullNameAndAge(t *testing.T) {
	p := validProfile()
	if got, want := p.FullName(), "Mendoza Ruiz Carlos"; got != want {
		t.Errorf("FullName = %q, want %q", got, want)
	}
	age, ok := p.AgeAt(time.Date(2026, 6, 19, 0, 0, 0, 0, time.UTC))
	if !ok || age != 40 {
		t.Errorf("AgeAt = %d, %v; want 40, true", age, ok)
	}
	if _, ok := (teachers.Teacher{}).AgeAt(time.Now()); ok {
		t.Error("AgeAt without birthdate should report unknown")
	}
}

func TestListSortedAlphabetically(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	for _, p := range []struct{ names, surnames, document string }{
		{"Zoe", "Zapata", "1000000001"},
		{"Ana", "gomez", "1000000002"},
		{"Beto", "López", "1000000003"},
	} {
		prof := validProfile()
		prof.Names, prof.Surnames, prof.DocumentID = p.names, p.surnames, p.document
		if _, err := svc.Create(ctx, prof); err != nil {
			t.Fatalf("Create %s %s: %v", p.surnames, p.names, err)
		}
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("len(list) = %d, want 3", len(list))
	}
	for i := 1; i < len(list); i++ {
		prev := strings.ToLower(list[i-1].FullName())
		cur := strings.ToLower(list[i].FullName())
		if prev > cur {
			t.Errorf("list not alphabetical: %q before %q", prev, cur)
		}
	}
}

func TestUpdateAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	tc, err := svc.Create(ctx, validProfile())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	tc.HomeroomClassID = ""
	tc.MedicalConditions = "Ninguna"
	upd, err := svc.Update(ctx, tc)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.IsHomeroomDirector() || upd.MedicalConditions != "Ninguna" {
		t.Errorf("Update = %+v", upd)
	}

	// Retiring flips Active instead of deleting the profile.
	upd.Active = false
	retired, err := svc.Update(ctx, upd)
	if err != nil {
		t.Fatalf("Update retire: %v", err)
	}
	if retired.Active {
		t.Errorf("retired Active = true, want false")
	}

	ghost := validProfile()
	ghost.ID = "00000000-0000-0000-0000-000000000001"
	if _, err := svc.Update(ctx, ghost); !errors.Is(err, teachers.ErrNotFound) {
		t.Errorf("Update unknown error = %v, want ErrNotFound", err)
	}

	if err := svc.Delete(ctx, tc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, tc.ID); !errors.Is(err, teachers.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
}

package warnings_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/warnings"
)

func newService() *warnings.Service {
	return warnings.NewService(warnings.NewMemoryStore())
}

func validInput() warnings.Input {
	return warnings.Input{
		StudentID:   "11111111-1111-1111-1111-111111111111",
		ClassID:     "9-1",
		TeacherID:   "teacher-1",
		HappenedAt:  time.Date(2026, 8, 20, 10, 30, 0, 0, time.UTC),
		Gravity:     warnings.GravityModerate,
		Title:       "Interrupción reiterada",
		Description: "El estudiante interrumpió la clase en tres ocasiones.",
		Snapshot: warnings.StudentSnapshot{
			Names: "Ana", Surnames: "Gómez", DocumentID: "1234567890",
			ClassID: "9-1", Age: 14,
			CaregiverName: "María Gómez", CaregiverPhone: "3101112233",
		},
	}
}

func TestGravityParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want warnings.Gravity
	}{
		{"leve", warnings.GravityMild},
		{"MEDIO", warnings.GravityModerate},
		{"moderado", warnings.GravityModerate},
		{"grave", warnings.GravitySevere},
		{"mild", warnings.GravityMild},
	} {
		got, err := warnings.Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := warnings.Parse("normal"); err == nil {
		t.Error(`Parse("normal") expected error: warnings use leve/medio/grave`)
	}
	if gs := warnings.KnownGravities(); len(gs) != 3 {
		t.Errorf("KnownGravities length = %d, want 3", len(gs))
	}
}

func TestIssueValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*warnings.Input)
		want   error
	}{
		{"bad student", func(in *warnings.Input) { in.StudentID = "x" }, warnings.ErrInvalidStudent},
		{"missing class", func(in *warnings.Input) { in.ClassID = " " }, warnings.ErrInvalidClass},
		{"missing teacher", func(in *warnings.Input) { in.TeacherID = "" }, warnings.ErrInvalidTeacher},
		{"zero date", func(in *warnings.Input) { in.HappenedAt = time.Time{} }, warnings.ErrInvalidDate},
		{"bad gravity", func(in *warnings.Input) { in.Gravity = warnings.Gravity("huge") }, warnings.ErrUnknownGravity},
		{"empty title", func(in *warnings.Input) { in.Title = "  " }, warnings.ErrEmptyTitle},
		{"empty description", func(in *warnings.Input) { in.Description = "" }, warnings.ErrEmptyDescription},
		{"snapshot without document", func(in *warnings.Input) {
			in.Snapshot.DocumentID = ""
		}, warnings.ErrInvalidSnapshot},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validInput()
			tc.mutate(&in)
			if _, err := svc.Issue(ctx, in); !errors.Is(err, tc.want) {
				t.Errorf("Issue error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestSnapshotFrozenOnIssue(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	in := validInput()
	w, err := svc.Issue(ctx, in)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if w.ID == "" {
		t.Error("Issue did not assign an id")
	}
	if w.Snapshot.ClassID != "9-1" || w.Snapshot.DocumentID != "1234567890" {
		t.Errorf("snapshot not preserved: %+v", w.Snapshot)
	}

	got, err := svc.ForStudent(ctx, in.StudentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(got) != 1 || got[0].ID != w.ID {
		t.Fatalf("ForStudent = %+v", got)
	}

	// A later profile change (e.g. new class) must not rewrite history.
	in2 := validInput()
	in2.HappenedAt = in.HappenedAt.Add(48 * time.Hour)
	in2.Snapshot.ClassID = "10-1"
	in2.ClassID = "10-1"
	if _, err := svc.Issue(ctx, in2); err != nil {
		t.Fatalf("Issue second: %v", err)
	}
	hist, err := svc.ForStudent(ctx, in.StudentID)
	if err != nil {
		t.Fatalf("ForStudent: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("len(hist) = %d, want 2", len(hist))
	}
	if !hist[0].HappenedAt.After(hist[1].HappenedAt) {
		t.Error("warnings not ordered newest first")
	}
	if hist[1].Snapshot.ClassID != "9-1" {
		t.Errorf("old snapshot rewritten: %+v", hist[1].Snapshot)
	}
}

func TestForStudentRejectsBadID(t *testing.T) {
	if _, err := newService().ForStudent(context.Background(), "nope"); !errors.Is(err, warnings.ErrInvalidStudent) {
		t.Errorf("error = %v, want ErrInvalidStudent", err)
	}
}

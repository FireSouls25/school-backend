package subjects_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/subjects"
)

func newService() *subjects.Service {
	return subjects.NewService(subjects.NewMemoryStore())
}

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

func mustSubject(t *testing.T, svc *subjects.Service, name string) subjects.Subject {
	t.Helper()
	s, err := svc.CreateSubject(context.Background(), subjects.Subject{Name: name})
	if err != nil {
		t.Fatalf("CreateSubject(%q): %v", name, err)
	}
	return s
}

func TestSubjectCRUD(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	s, err := svc.CreateSubject(ctx, subjects.Subject{
		Name: "  Matemáticas ", Code: " mat ", Description: "Álgebra y geometría",
	})
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	if s.ID == "" || !s.Active {
		t.Errorf("CreateSubject = %+v, want id and active", s)
	}
	if s.Name != "Matemáticas" || s.Code != "MAT" {
		t.Errorf("CreateSubject not normalized: %+v", s)
	}

	if _, err := svc.CreateSubject(ctx, subjects.Subject{Name: "matemáticas"}); !errors.Is(err, subjects.ErrDuplicateSubject) {
		t.Errorf("duplicate name error = %v, want ErrDuplicateSubject", err)
	}
	if _, err := svc.CreateSubject(ctx, subjects.Subject{Name: "  "}); !errors.Is(err, subjects.ErrInvalidName) {
		t.Errorf("blank name error = %v, want ErrInvalidName", err)
	}

	got, err := svc.SubjectByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("SubjectByID: %v", err)
	}
	if got.Name != "Matemáticas" {
		t.Errorf("SubjectByID = %+v", got)
	}

	// Retire without losing history.
	s.Active = false
	upd, err := svc.UpdateSubject(ctx, s)
	if err != nil {
		t.Fatalf("UpdateSubject: %v", err)
	}
	if upd.Active {
		t.Errorf("UpdateSubject = %+v, want inactive", upd)
	}

	list, err := svc.ListSubjects(ctx)
	if err != nil {
		t.Fatalf("ListSubjects: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
}

func TestAssignValidatesRefs(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	math := mustSubject(t, svc, "Matemáticas")

	if _, err := svc.Assign(ctx, "nope", math.ID, date(2021, 2, 1)); !errors.Is(err, subjects.ErrInvalidTeacher) {
		t.Errorf("bad teacher error = %v, want ErrInvalidTeacher", err)
	}
	if _, err := svc.Assign(ctx, "11111111-1111-1111-1111-111111111111", "nope", date(2021, 2, 1)); !errors.Is(err, subjects.ErrInvalidSubject) {
		t.Errorf("bad subject error = %v, want ErrInvalidSubject", err)
	}
	unknown := "22222222-2222-2222-2222-222222222222"
	if _, err := svc.Assign(ctx, "11111111-1111-1111-1111-111111111111", unknown, date(2021, 2, 1)); !errors.Is(err, subjects.ErrNotFound) {
		t.Errorf("unknown subject error = %v, want ErrNotFound", err)
	}
	if _, err := svc.Assign(ctx, "11111111-1111-1111-1111-111111111111", math.ID, time.Time{}); !errors.Is(err, subjects.ErrInvalidDate) {
		t.Errorf("zero date error = %v, want ErrInvalidDate", err)
	}
}

// TestTimelineSwitchSubject covers the core story: 5 years of matemática,
// then a switch to física, keeping both periods.
func TestTimelineSwitchSubject(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	teacher := "11111111-1111-1111-1111-111111111111"
	math := mustSubject(t, svc, "Matemáticas")
	physics := mustSubject(t, svc, "Física")

	m, err := svc.Assign(ctx, teacher, math.ID, date(2021, 2, 1))
	if err != nil {
		t.Fatalf("Assign math: %v", err)
	}
	if !m.IsOpen() {
		t.Error("new assignment should be open")
	}

	// Same pair cannot open twice.
	if _, err := svc.Assign(ctx, teacher, math.ID, date(2022, 2, 1)); !errors.Is(err, subjects.ErrAlreadyAssigned) {
		t.Errorf("duplicate open error = %v, want ErrAlreadyAssigned", err)
	}

	// Close matemática, open física: both periods survive.
	closed, err := svc.EndAssignment(ctx, m.ID, date(2026, 1, 31))
	if err != nil {
		t.Fatalf("EndAssignment: %v", err)
	}
	if closed.IsOpen() {
		t.Error("ended assignment should be closed")
	}
	if _, err := svc.EndAssignment(ctx, m.ID, date(2026, 2, 1)); !errors.Is(err, subjects.ErrAlreadyEnded) {
		t.Errorf("re-end error = %v, want ErrAlreadyEnded", err)
	}

	p, err := svc.Assign(ctx, teacher, physics.ID, date(2026, 2, 1))
	if err != nil {
		t.Fatalf("Assign physics: %v", err)
	}

	timeline, err := svc.AssignmentsForTeacher(ctx, teacher)
	if err != nil {
		t.Fatalf("AssignmentsForTeacher: %v", err)
	}
	if len(timeline) != 2 {
		t.Fatalf("len(timeline) = %d, want 2", len(timeline))
	}
	if timeline[0].ID != m.ID || timeline[1].ID != p.ID {
		t.Errorf("timeline not chronological: %+v", timeline)
	}

	current, err := svc.CurrentForTeacher(ctx, teacher)
	if err != nil {
		t.Fatalf("CurrentForTeacher: %v", err)
	}
	if len(current) != 1 || current[0].SubjectID != physics.ID {
		t.Errorf("current = %+v, want only física", current)
	}
}

// TestTimelineOverlappingSubjects covers teaching física and informática
// at once: informática counts only from its own start date.
func TestTimelineOverlappingSubjects(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	teacher := "33333333-3333-3333-3333-333333333333"
	physics := mustSubject(t, svc, "Física")
	cs := mustSubject(t, svc, "Informática")

	if _, err := svc.Assign(ctx, teacher, physics.ID, date(2024, 2, 1)); err != nil {
		t.Fatalf("Assign physics: %v", err)
	}
	if _, err := svc.Assign(ctx, teacher, cs.ID, date(2025, 2, 1)); err != nil {
		t.Fatalf("Assign informática: %v", err)
	}

	current, err := svc.CurrentForTeacher(ctx, teacher)
	if err != nil {
		t.Fatalf("CurrentForTeacher: %v", err)
	}
	if len(current) != 2 {
		t.Fatalf("len(current) = %d, want 2 overlapping", len(current))
	}

	// Ending informática leaves física open.
	timeline, err := svc.AssignmentsForTeacher(ctx, teacher)
	if err != nil {
		t.Fatalf("AssignmentsForTeacher: %v", err)
	}
	var csAssign subjects.Assignment
	for _, a := range timeline {
		if a.SubjectID == cs.ID {
			csAssign = a
		}
	}
	if _, err := svc.EndAssignment(ctx, csAssign.ID, date(2026, 6, 30)); err != nil {
		t.Fatalf("EndAssignment: %v", err)
	}
	current, err = svc.CurrentForTeacher(ctx, teacher)
	if err != nil {
		t.Fatalf("CurrentForTeacher: %v", err)
	}
	if len(current) != 1 || current[0].SubjectID != physics.ID {
		t.Errorf("current = %+v, want only física", current)
	}
}

func TestEndAssignmentValidatesRange(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	teacher := "44444444-4444-4444-4444-444444444444"
	math := mustSubject(t, svc, "Matemáticas")

	a, err := svc.Assign(ctx, teacher, math.ID, date(2025, 2, 1))
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if _, err := svc.EndAssignment(ctx, a.ID, date(2024, 1, 1)); !errors.Is(err, subjects.ErrInvalidEnd) {
		t.Errorf("end-before-start error = %v, want ErrInvalidEnd", err)
	}
	if _, err := svc.EndAssignment(ctx, "55555555-5555-5555-5555-555555555555", date(2026, 1, 1)); !errors.Is(err, subjects.ErrAssignmentNotFound) {
		t.Errorf("unknown assignment error = %v, want ErrAssignmentNotFound", err)
	}
}

func TestDeleteSubjectBlockedWithHistory(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	teacher := "66666666-6666-6666-6666-666666666666"
	math := mustSubject(t, svc, "Matemáticas")

	a, err := svc.Assign(ctx, teacher, math.ID, date(2021, 2, 1))
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := svc.DeleteSubject(ctx, math.ID); !errors.Is(err, subjects.ErrHasAssignments) {
		t.Errorf("delete with history error = %v, want ErrHasAssignments", err)
	}

	// After removing the assignment (correction), deletion succeeds.
	if err := svc.RemoveAssignment(ctx, a.ID); err != nil {
		t.Fatalf("RemoveAssignment: %v", err)
	}
	if err := svc.DeleteSubject(ctx, math.ID); err != nil {
		t.Fatalf("DeleteSubject: %v", err)
	}
	if _, err := svc.SubjectByID(ctx, math.ID); !errors.Is(err, subjects.ErrNotFound) {
		t.Errorf("SubjectByID after delete error = %v, want ErrNotFound", err)
	}
}

func TestAssignmentsForSubject(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	t1 := "77777777-7777-7777-7777-777777777777"
	t2 := "88888888-8888-8888-8888-888888888888"
	math := mustSubject(t, svc, "Matemáticas")

	if _, err := svc.Assign(ctx, t1, math.ID, date(2020, 2, 1)); err != nil {
		t.Fatalf("Assign t1: %v", err)
	}
	if _, err := svc.Assign(ctx, t2, math.ID, date(2023, 2, 1)); err != nil {
		t.Fatalf("Assign t2: %v", err)
	}

	hist, err := svc.AssignmentsForSubject(ctx, math.ID)
	if err != nil {
		t.Fatalf("AssignmentsForSubject: %v", err)
	}
	if len(hist) != 2 || hist[0].TeacherID != t1 || hist[1].TeacherID != t2 {
		t.Errorf("history = %+v, want chronological t1 then t2", hist)
	}
}

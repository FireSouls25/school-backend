package permits_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/permits"
)

const (
	studentID = "11111111-1111-4111-8111-111111111111"
	groupID   = "22222222-2222-4222-8222-222222222222"
	otherStu  = "33333333-3333-4333-8333-333333333333"
)

func newSvc() *permits.Service {
	return permits.NewService(permits.NewMemoryStore())
}

func validDraft() permits.Draft {
	return permits.Draft{
		StudentID:    studentID,
		ClassGroupID: groupID,
		Date:         "2026-05-04",
		Reason:       "Cita médica",
		CreatedBy:    "secretaria",
	}
}

func TestGrantNormalizesAndStores(t *testing.T) {
	svc := newSvc()
	p, err := svc.Grant(context.Background(), validDraft())
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if p.ID == "" {
		t.Fatal("empty permit id")
	}
	if p.Date.Hour() != 0 || p.Date.Minute() != 0 || p.Date.Location() != time.UTC {
		t.Errorf("date not normalized: %v", p.Date)
	}
	if p.Date.Format("2006-01-02") != "2026-05-04" {
		t.Errorf("date = %s, want 2026-05-04", p.Date.Format("2006-01-02"))
	}

	got, ok, err := svc.PermitOf(context.Background(), studentID, groupID, p.Date)
	if err != nil || !ok {
		t.Fatalf("permit of: ok=%v err=%v", ok, err)
	}
	if got.ID != p.ID {
		t.Errorf("permit id = %q, want %q", got.ID, p.ID)
	}

	// Another student on the same day is not covered.
	if _, ok, _ := svc.PermitOf(context.Background(), otherStu, groupID, p.Date); ok {
		t.Error("other student unexpectedly covered")
	}
}

func TestGrantValidatesInput(t *testing.T) {
	svc := newSvc()
	cases := []struct {
		name string
		mut  func(*permits.Draft)
		want error
	}{
		{"student", func(d *permits.Draft) { d.StudentID = "nope" }, permits.ErrInvalidStudent},
		{"group", func(d *permits.Draft) { d.ClassGroupID = "" }, permits.ErrInvalidClassGroup},
		{"reason", func(d *permits.Draft) { d.Reason = "  " }, permits.ErrEmptyReason},
		{"actor", func(d *permits.Draft) { d.CreatedBy = "" }, permits.ErrInvalidActor},
		{"date", func(d *permits.Draft) { d.Date = "04/05/2026" }, permits.ErrInvalidDate},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := validDraft()
			tc.mut(&d)
			if _, err := svc.Grant(context.Background(), d); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestForStudentAndRevoke(t *testing.T) {
	svc := newSvc()
	ctx := context.Background()
	first, err := svc.Grant(ctx, validDraft())
	if err != nil {
		t.Fatalf("grant first: %v", err)
	}
	secondDraft := validDraft()
	secondDraft.Date = "2026-05-05"
	second, err := svc.Grant(ctx, secondDraft)
	if err != nil {
		t.Fatalf("grant second: %v", err)
	}

	list, err := svc.ForStudent(ctx, studentID)
	if err != nil {
		t.Fatalf("for student: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("permits len = %d, want 2", len(list))
	}
	// Newest day first.
	if list[0].ID != second.ID || list[1].ID != first.ID {
		t.Errorf("list order = %q,%q want %q,%q", list[0].ID, list[1].ID, second.ID, first.ID)
	}

	if err := svc.Revoke(ctx, second.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := svc.Revoke(ctx, second.ID); !errors.Is(err, permits.ErrNotFound) {
		t.Errorf("second revoke err = %v, want ErrNotFound", err)
	}
	list, _ = svc.ForStudent(ctx, studentID)
	if len(list) != 1 || list[0].ID != first.ID {
		t.Errorf("list after revoke = %+v", list)
	}
}

func TestAppliesToSubjectScope(t *testing.T) {
	wholeDay := permits.Permit{ClassGroupID: groupID}
	if !wholeDay.AppliesTo(groupID, "subject-a") {
		t.Error("whole-day permit should cover every subject")
	}
	oneSubject := permits.Permit{ClassGroupID: groupID, SubjectID: "subject-a"}
	if !oneSubject.AppliesTo(groupID, "subject-a") {
		t.Error("subject permit should cover its own subject")
	}
	if oneSubject.AppliesTo(groupID, "subject-b") {
		t.Error("subject permit should not cover another subject")
	}
	if oneSubject.AppliesTo("another-group", "subject-a") {
		t.Error("permit should not cover another group")
	}
}

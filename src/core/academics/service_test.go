package academics_test

import (
	"context"
	"testing"

	"grade/src/core/academics"
)

func TestRaiseAndReview(t *testing.T) {
	ctx := context.Background()
	svc := academics.NewService(academics.NewMemoryStore())

	a, err := svc.Raise(ctx, academics.Input{
		StudentID: "11111111-1111-4111-8111-111111111111",
		TeacherID: "teacher-1",
		Reason:    "Acumulación de faltas graves",
	})
	if err != nil {
		t.Fatalf("raise: %v", err)
	}
	if a.Status != academics.StatusOpen {
		t.Errorf("status = %q, want open", a.Status)
	}

	open, err := svc.Open(ctx, "")
	if err != nil || len(open) != 1 {
		t.Fatalf("open = %v, %v; want one", open, err)
	}

	reviewed, err := svc.Review(ctx, a.ID, "admin-1")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if reviewed.Status != academics.StatusReviewed || reviewed.ReviewedBy != "admin-1" {
		t.Errorf("reviewed = %+v, want reviewed by admin-1", reviewed)
	}
	if _, err := svc.Review(ctx, a.ID, "admin-1"); err != academics.ErrAlreadyReviewed {
		t.Errorf("second review err = %v, want ErrAlreadyReviewed", err)
	}
	if open, _ := svc.Open(ctx, ""); len(open) != 0 {
		t.Errorf("open = %v, want empty", open)
	}
}

func TestRaiseValidates(t *testing.T) {
	ctx := context.Background()
	svc := academics.NewService(academics.NewMemoryStore())

	for _, in := range []academics.Input{
		{StudentID: "not-a-uuid", TeacherID: "t", Reason: "x"},
		{StudentID: "11111111-1111-4111-8111-111111111111", TeacherID: "", Reason: "x"},
		{StudentID: "11111111-1111-4111-8111-111111111111", TeacherID: "t", Reason: "  "},
	} {
		if _, err := svc.Raise(ctx, in); err == nil {
			t.Errorf("raise %+v = nil error, want validation", in)
		}
	}
}

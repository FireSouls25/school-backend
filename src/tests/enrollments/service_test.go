package enrollments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/enrollments"
)

func newService() *enrollments.Service {
	return enrollments.NewService(enrollments.NewMemoryStore())
}

const (
	studentA = "11111111-1111-1111-1111-111111111111"
	studentB = "22222222-2222-2222-2222-222222222222"
	groupA   = "33333333-3333-3333-3333-333333333333"
	groupB   = "44444444-4444-4444-4444-444444444444"
)

func decidedAt() time.Time {
	return time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC)
}

func TestEnrollValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: "nope", ClassGroupID: groupA}); !errors.Is(err, enrollments.ErrInvalidStudent) {
		t.Errorf("bad student error = %v, want ErrInvalidStudent", err)
	}
	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentA, ClassGroupID: "nope"}); !errors.Is(err, enrollments.ErrInvalidClassGroup) {
		t.Errorf("bad group error = %v, want ErrInvalidClassGroup", err)
	}

	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentA, ClassGroupID: groupA}); err != nil {
		t.Fatalf("Enroll: %v", err)
	}
	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentA, ClassGroupID: groupA}); !errors.Is(err, enrollments.ErrDuplicateEnrollment) {
		t.Errorf("duplicate error = %v, want ErrDuplicateEnrollment", err)
	}
	// Same student in another group is a different enrollment.
	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentA, ClassGroupID: groupB}); err != nil {
		t.Fatalf("Enroll other group: %v", err)
	}
}

func TestRosterAndUnenroll(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentA, ClassGroupID: groupA})
	if err != nil {
		t.Fatalf("Enroll A: %v", err)
	}
	if a.ID == "" {
		t.Error("Enroll did not assign an id")
	}
	if _, err := svc.Enroll(ctx, enrollments.Enrollment{StudentID: studentB, ClassGroupID: groupA}); err != nil {
		t.Fatalf("Enroll B: %v", err)
	}

	roster, err := svc.EnrollmentsForGroup(ctx, groupA)
	if err != nil {
		t.Fatalf("EnrollmentsForGroup: %v", err)
	}
	if len(roster) != 2 || roster[0].StudentID != studentA || roster[1].StudentID != studentB {
		t.Errorf("roster = %+v, want enrollment order", roster)
	}

	mine, err := svc.EnrollmentsForStudent(ctx, studentA)
	if err != nil {
		t.Fatalf("EnrollmentsForStudent: %v", err)
	}
	if len(mine) != 1 || mine[0].ClassGroupID != groupA {
		t.Errorf("mine = %+v", mine)
	}

	if err := svc.Unenroll(ctx, a.ID); err != nil {
		t.Fatalf("Unenroll: %v", err)
	}
	roster, err = svc.EnrollmentsForGroup(ctx, groupA)
	if err != nil {
		t.Fatalf("EnrollmentsForGroup: %v", err)
	}
	if len(roster) != 1 {
		t.Errorf("len(roster) = %d, want 1", len(roster))
	}

	if err := svc.Unenroll(ctx, "not-a-uuid"); !errors.Is(err, enrollments.ErrInvalidID) {
		t.Errorf("bad id error = %v, want ErrInvalidID", err)
	}
}

func TestRecordPromotionValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	base := enrollments.Promotion{
		StudentID: studentA, FromGroupID: groupA, ToGroupID: groupB,
		Decision: enrollments.DecisionPromote, DecidedBy: "admin-1", DecidedAt: decidedAt(),
	}

	if _, err := svc.RecordPromotion(ctx, base); err != nil {
		t.Fatalf("RecordPromotion: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*enrollments.Promotion)
		want   error
	}{
		{"bad decision", func(p *enrollments.Promotion) { p.Decision = "skip" }, enrollments.ErrInvalidDecision},
		{"promote without destination", func(p *enrollments.Promotion) { p.ToGroupID = "" }, enrollments.ErrInvalidPromotion},
		{"graduate with destination", func(p *enrollments.Promotion) {
			p.Decision = enrollments.DecisionGraduate
		}, enrollments.ErrInvalidPromotion},
		{"zero date", func(p *enrollments.Promotion) { p.DecidedAt = time.Time{} }, enrollments.ErrInvalidDate},
		{"missing actor", func(p *enrollments.Promotion) { p.DecidedBy = "  " }, enrollments.ErrInvalidActor},
		{"bad student", func(p *enrollments.Promotion) { p.StudentID = "x" }, enrollments.ErrInvalidStudent},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := base
			tc.mutate(&p)
			if _, err := svc.RecordPromotion(ctx, p); !errors.Is(err, tc.want) {
				t.Errorf("RecordPromotion error = %v, want %v", err, tc.want)
			}
		})
	}

	// Graduation without destination is valid: end of the lifecycle.
	grad := base
	grad.ToGroupID, grad.Decision = "", enrollments.DecisionGraduate
	grad.DecidedAt = decidedAt().Add(365 * 24 * time.Hour)
	if _, err := svc.RecordPromotion(ctx, grad); err != nil {
		t.Fatalf("RecordPromotion graduate: %v", err)
	}

	hist, err := svc.PromotionsForStudent(ctx, studentA)
	if err != nil {
		t.Fatalf("PromotionsForStudent: %v", err)
	}
	if len(hist) != 2 || hist[0].Decision != enrollments.DecisionPromote {
		t.Errorf("history = %+v, want oldest first", hist)
	}
}

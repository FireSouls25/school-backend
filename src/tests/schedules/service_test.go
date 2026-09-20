package schedules_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/schedules"
)

// fakeChecker is a schedules.Checker backed by an explicit allow-list.
type fakeChecker struct {
	teaches map[string]bool // "teacherID/subjectID" -> teaches
	err     error
}

func (f fakeChecker) Teaches(_ context.Context, teacherID, subjectID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.teaches[teacherID+"/"+subjectID], nil
}

const (
	teacherA = "11111111-1111-1111-1111-111111111111"
	teacherB = "22222222-2222-2222-2222-222222222222"
	groupA   = "33333333-3333-3333-3333-333333333333"
	groupB   = "44444444-4444-4444-4444-444444444444"
	math     = "55555555-5555-5555-5555-555555555555"
	physics  = "66666666-6666-6666-6666-666666666666"
)

func newService() *schedules.Service {
	return schedules.NewService(schedules.NewMemoryStore(), fakeChecker{
		teaches: map[string]bool{teacherA + "/" + math: true},
	})
}

func slot(group, teacher, subject string, day time.Weekday, start, end string) schedules.Entry {
	return schedules.Entry{
		ClassGroupID: group, TeacherID: teacher, SubjectID: subject,
		Weekday: int(day), Start: schedules.MustClock(start), End: schedules.MustClock(end),
	}
}

func TestParseClock(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want schedules.Clock
		ok   bool
	}{
		{"07:30", 450, true},
		{"7:30", 450, true},
		{"23:59", 1439, true},
		{"00:00", 0, true},
		{"24:00", 0, false},
		{"07:60", 0, false},
		{"0730", 0, false},
		{"", 0, false},
	} {
		got, err := schedules.ParseClock(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Errorf("ParseClock(%q) = %v, %v; want %v", tc.in, got, err, tc.want)
		}
		if !tc.ok && err == nil {
			t.Errorf("ParseClock(%q) expected error", tc.in)
		}
	}
	if got := schedules.MustClock("07:30").String(); got != "07:30" {
		t.Errorf("Clock.String = %q, want 07:30", got)
	}
}

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name  string
		entry schedules.Entry
		want  error
	}{
		{"bad group", slot("nope", teacherA, math, time.Monday, "07:00", "08:00"), schedules.ErrInvalidClassGroup},
		{"bad teacher", slot(groupA, "nope", math, time.Monday, "07:00", "08:00"), schedules.ErrInvalidTeacher},
		{"bad subject", slot(groupA, teacherA, "nope", time.Monday, "07:00", "08:00"), schedules.ErrInvalidSubject},
		{"bad weekday", slot(groupA, teacherA, math, 7, "07:00", "08:00"), schedules.ErrInvalidWeekday},
		{"inverted range", slot(groupA, teacherA, math, time.Monday, "08:00", "07:00"), schedules.ErrInvalidTime},
		{"empty range", slot(groupA, teacherA, math, time.Monday, "08:00", "08:00"), schedules.ErrInvalidTime},
		{"not teaching", slot(groupA, teacherA, physics, time.Monday, "07:00", "08:00"), schedules.ErrNotTeachingSubject},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, tc.entry); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestConflicts(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	base, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if base.ID == "" {
		t.Error("Create did not assign an id")
	}

	// Same teacher, overlapping slot in another group.
	_, err = svc.Create(ctx, slot(groupB, teacherA, math, time.Monday, "07:30", "08:30"))
	if !errors.Is(err, schedules.ErrTeacherConflict) {
		t.Errorf("teacher overlap error = %v, want ErrTeacherConflict", err)
	}

	// Same group, overlapping slot with another teacher needs a checker
	// allowing teacherB/math.
	svc2 := schedules.NewService(schedules.NewMemoryStore(), fakeChecker{
		teaches: map[string]bool{
			teacherA + "/" + math: true,
			teacherB + "/" + math: true,
		},
	})
	if _, err := svc2.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00")); err != nil {
		t.Fatalf("Create svc2: %v", err)
	}
	_, err = svc2.Create(ctx, slot(groupA, teacherB, math, time.Monday, "07:30", "08:30"))
	if !errors.Is(err, schedules.ErrClassConflict) {
		t.Errorf("class overlap error = %v, want ErrClassConflict", err)
	}

	// Touching edges are fine; other days are fine.
	if _, err := svc.Create(ctx, slot(groupB, teacherA, math, time.Monday, "08:00", "09:00")); err != nil {
		t.Errorf("touching slot: %v", err)
	}
	if _, err := svc.Create(ctx, slot(groupB, teacherA, math, time.Tuesday, "07:30", "08:30")); err != nil {
		t.Errorf("other day: %v", err)
	}

	// Updating the entry itself is stable (self excluded from conflicts).
	base.End = schedules.MustClock("07:30")
	upd, err := svc.Update(ctx, base)
	if err != nil {
		t.Fatalf("Update self: %v", err)
	}
	if upd.End != schedules.MustClock("07:30") {
		t.Errorf("Update = %+v, want shortened slot", upd)
	}
}

func TestDayViewAndGroupSchedule(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Wednesday, "09:00", "10:00")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	monday, err := svc.EntriesForTeacherOnDay(ctx, teacherA, time.Monday)
	if err != nil {
		t.Fatalf("EntriesForTeacherOnDay: %v", err)
	}
	if len(monday) != 1 || monday[0].Start != schedules.MustClock("07:00") {
		t.Errorf("monday = %+v", monday)
	}
	tuesday, err := svc.EntriesForTeacherOnDay(ctx, teacherA, time.Tuesday)
	if err != nil {
		t.Fatalf("EntriesForTeacherOnDay: %v", err)
	}
	if len(tuesday) != 0 {
		t.Errorf("tuesday = %+v, want empty", tuesday)
	}

	weekly, err := svc.EntriesForGroup(ctx, groupA)
	if err != nil {
		t.Fatalf("EntriesForGroup: %v", err)
	}
	if len(weekly) != 2 || weekly[0].Weekday != int(time.Monday) {
		t.Errorf("weekly = %+v, want monday first", weekly)
	}

	if _, err := svc.EntriesForTeacherOnDay(ctx, teacherA, 9); !errors.Is(err, schedules.ErrInvalidWeekday) {
		t.Errorf("bad weekday error = %v, want ErrInvalidWeekday", err)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	e, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, e.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, e.ID); !errors.Is(err, schedules.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
}

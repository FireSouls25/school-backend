package statistics_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/statistics"
)

const (
	groupID  = "11111111-1111-1111-1111-111111111111"
	studentA = "22222222-2222-2222-2222-222222222222"
	studentB = "33333333-3333-3333-3333-333333333333"
)

func day(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// fakeSources is a statistics.SessionSource, WarningSource and FaultSource
// backed by fixed views.
type fakeSources struct {
	sessions []statistics.SessionView
	warnings map[string][]statistics.WarningView
	faults   map[string][]statistics.FaultView
}

func (f fakeSources) GroupSessions(_ context.Context, _ string) ([]statistics.SessionView, error) {
	return f.sessions, nil
}

func (f fakeSources) StudentSessions(_ context.Context, id string) ([]statistics.SessionView, error) {
	out := make([]statistics.SessionView, 0)
	for _, v := range f.sessions {
		for _, e := range v.Roster {
			if e.StudentID == id {
				out = append(out, v)
			}
		}
	}
	return out, nil
}

func (f fakeSources) StudentWarnings(_ context.Context, id string) ([]statistics.WarningView, error) {
	return f.warnings[id], nil
}

func (f fakeSources) StudentFaults(_ context.Context, id string) ([]statistics.FaultView, error) {
	return f.faults[id], nil
}

func fixture() fakeSources {
	roster := []statistics.RosterEntryView{
		{StudentID: studentA, Names: "Ana", Surnames: "Gómez"},
		{StudentID: studentB, Names: "Luis", Surnames: "Pardo"},
	}
	return fakeSources{
		sessions: []statistics.SessionView{
			{
				ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ClassGroupID: groupID,
				ClassLabel: "9-1", SchoolYear: 2026, Date: day(2026, 8, 18), Period: 2,
				Roster: roster, Marks: map[string]string{studentA: "absence"},
			},
			{
				ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ClassGroupID: groupID,
				ClassLabel: "9-1", SchoolYear: 2026, Date: day(2026, 8, 20), Period: 2,
				Roster: roster,
				Marks:  map[string]string{studentA: "late", studentB: "evasion"},
			},
		},
		warnings: map[string][]statistics.WarningView{
			studentA: {{Gravity: "moderate", Date: day(2026, 8, 20)}},
		},
		faults: map[string][]statistics.FaultView{
			studentA: {{Severity: "severe", Date: day(2026, 8, 21)}},
			studentB: {{Severity: "minor", Date: day(2026, 8, 21)}},
		},
	}
}

func TestClassReport(t *testing.T) {
	ctx := context.Background()
	svc := statistics.NewService(fixture(), fixture(), fixture())

	report, err := svc.ClassReport(ctx, groupID)
	if err != nil {
		t.Fatalf("ClassReport: %v", err)
	}
	if report.Sessions != 2 {
		t.Errorf("Sessions = %d, want 2", report.Sessions)
	}
	if len(report.Students) != 2 {
		t.Fatalf("len(Students) = %d, want 2", len(report.Students))
	}
	// Alphabetical by surnames: Gómez before Pardo.
	if report.Students[0].StudentID != studentA || report.Students[1].StudentID != studentB {
		t.Errorf("order = %+v, want Gómez then Pardo", report.Students)
	}
	a := report.Students[0]
	if a.Sessions != 2 || a.Presences != 0 || a.Absences != 1 || a.Lates != 1 {
		t.Errorf("studentA = %+v", a)
	}
	if got := a.AttendanceRate(); got != 0 {
		t.Errorf("studentA rate = %v, want 0", got)
	}
	b := report.Students[1]
	if b.Presences != 1 || b.Evasions != 1 {
		t.Errorf("studentB = %+v", b)
	}
	if got := b.AttendanceRate(); got != 0.5 {
		t.Errorf("studentB rate = %v, want 0.5", got)
	}
	if report.Totals.Students != 2 || report.Totals.Absences != 1 ||
		report.Totals.Evasions != 1 || report.Totals.Lates != 1 || report.Totals.Presences != 1 {
		t.Errorf("totals = %+v", report.Totals)
	}

	if _, err := svc.ClassReport(ctx, "nope"); !errors.Is(err, statistics.ErrInvalidClassGroup) {
		t.Errorf("bad group error = %v, want ErrInvalidClassGroup", err)
	}
}

func TestStudentReport(t *testing.T) {
	ctx := context.Background()
	fx := fixture()
	svc := statistics.NewService(fx, fx, fx)

	report, err := svc.StudentReport(ctx, studentA)
	if err != nil {
		t.Fatalf("StudentReport: %v", err)
	}
	if len(report.Sessions) != 2 {
		t.Fatalf("len(Sessions) = %d, want 2", len(report.Sessions))
	}
	if report.Sessions[0].Mark != "absence" || report.Sessions[1].Mark != "late" {
		t.Errorf("marks = %+v", report.Sessions)
	}
	if report.Sessions[0].ClassLabel != "9-1" || report.Sessions[0].SchoolYear != 2026 {
		t.Errorf("frozen context lost: %+v", report.Sessions[0])
	}
	if report.Absences != 1 || report.Lates != 1 || report.Presences != 0 {
		t.Errorf("tallies = %+v", report)
	}
	if report.WarningsModerate != 1 || report.WarningsMild+report.WarningsSevere != 0 {
		t.Errorf("warnings = %+v", report)
	}
	if report.FaultsSevere != 1 || report.FaultsMinor+report.FaultsOrdinary != 0 {
		t.Errorf("faults = %+v", report)
	}

	empty, err := svc.StudentReport(ctx, "44444444-4444-4444-4444-444444444444")
	if err != nil {
		t.Fatalf("StudentReport unknown: %v", err)
	}
	if len(empty.Sessions) != 0 || empty.Presences+empty.Absences != 0 {
		t.Errorf("unknown student report not empty: %+v", empty)
	}

	if _, err := svc.StudentReport(ctx, "nope"); !errors.Is(err, statistics.ErrInvalidStudent) {
		t.Errorf("bad student error = %v, want ErrInvalidStudent", err)
	}
}

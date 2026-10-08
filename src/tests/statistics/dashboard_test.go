package statistics_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"grade/src/core/statistics"
)

// fakeDash is a statistics.DashboardSource backed by fixed data.
type fakeDash struct {
	teachers int
	groups   []string
	enrolled map[string][]string
	recents  []statistics.DashboardWarning
}

func (f fakeDash) TeacherTotal(context.Context) (int, error) { return f.teachers, nil }
func (f fakeDash) GroupsOfYear(_ context.Context, _ int) ([]string, error) {
	return f.groups, nil
}
func (f fakeDash) EnrolledIDs(_ context.Context, gid string) ([]string, error) {
	return f.enrolled[gid], nil
}
func (f fakeDash) RecentWarnings(_ context.Context, _ time.Time) ([]statistics.DashboardWarning, error) {
	return f.recents, nil
}

// groupSessions is a statistics.SessionSource answering per group.
type groupSessions struct {
	views map[string][]statistics.SessionView
}

func (g groupSessions) GroupSessions(_ context.Context, gid string) ([]statistics.SessionView, error) {
	return g.views[gid], nil
}
func (g groupSessions) StudentSessions(context.Context, string) ([]statistics.SessionView, error) {
	return nil, nil
}

func dashService(sessions groupSessions, dash fakeDash) *statistics.DashboardService {
	return statistics.NewDashboardService(sessions, fakeSources{}, fakeSources{}, dash)
}

func TestDashboardSummary(t *testing.T) {
	now := time.Now()
	inWindow := now.AddDate(0, 0, -3)
	old := now.AddDate(0, 0, -10)
	roster := []statistics.RosterEntryView{
		{StudentID: studentA, Names: "Ana", Surnames: "Gómez"},
		{StudentID: studentB, Names: "Luis", Surnames: "Pardo"},
	}
	sessions := groupSessions{views: map[string][]statistics.SessionView{
		groupID: {
			{
				ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", ClassGroupID: groupID,
				ClassLabel: "9-1", SchoolYear: 2026, Date: inWindow, Period: 1,
				Roster: roster, Marks: map[string]string{studentA: "absence"},
			},
			{
				ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", ClassGroupID: groupID,
				ClassLabel: "9-1", SchoolYear: 2026, Date: old, Period: 1,
				Roster: roster, Marks: map[string]string{studentA: "absence"},
			},
		},
	}}
	dash := fakeDash{
		teachers: 4,
		groups:   []string{groupID},
		enrolled: map[string][]string{groupID: {studentA, studentB}},
		recents: []statistics.DashboardWarning{
			{ID: "w1", StudentID: studentA, Gravity: "severe", Title: "Grave", Date: inWindow},
			{ID: "w2", StudentID: studentB, Gravity: "mild", Title: "Leve", Date: inWindow},
		},
	}

	got, err := dashService(sessions, dash).Summary(context.Background(), 2026)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if got.Year != 2026 {
		t.Errorf("Year = %d, want 2026", got.Year)
	}
	if got.Totals != (statistics.DashboardTotals{Teachers: 4, Students: 2, Groups: 1}) {
		t.Errorf("Totals = %+v", got.Totals)
	}
	if got.GroupsTruncated {
		t.Error("GroupsTruncated = true, want false")
	}
	// Only the in-window call counts: one absence, one presence.
	if got.MarksLast7d.Absences != 1 || got.MarksLast7d.Presences != 1 {
		t.Errorf("MarksLast7d = %+v, want 1 absence and 1 presence", got.MarksLast7d)
	}
	if got.WarningsLast7d != (statistics.DashboardWarnings{Mild: 1, Severe: 1}) {
		t.Errorf("WarningsLast7d = %+v", got.WarningsLast7d)
	}
	// Top-risk folds every call in the year (both absences of Ana).
	if len(got.TopRisk) != 1 || got.TopRisk[0].StudentID != studentA || got.TopRisk[0].Absences != 2 {
		t.Errorf("TopRisk = %+v, want Ana with 2 absences", got.TopRisk)
	}
	if len(got.RecentWarnings) != 2 || got.RecentWarnings[0].ID != "w1" {
		t.Errorf("RecentWarnings = %+v, want newest first", got.RecentWarnings)
	}
}

func TestDashboardCapsGroupsAndRecent(t *testing.T) {
	groups := make([]string, 0, 30)
	views := make(map[string][]statistics.SessionView)
	recents := make([]statistics.DashboardWarning, 0, 10)
	for i := 0; i < 30; i++ {
		gid := fmt.Sprintf("00000000-0000-4000-8000-%012d", i)
		groups = append(groups, gid)
	}
	for i := 0; i < 10; i++ {
		recents = append(recents, statistics.DashboardWarning{
			ID: fmt.Sprintf("w-%d", i), Gravity: "mild",
			Date: time.Now().AddDate(0, 0, -1),
		})
	}
	dash := fakeDash{teachers: 1, groups: groups, enrolled: map[string][]string{}, recents: recents}

	got, err := dashService(groupSessions{views: views}, dash).Summary(context.Background(), 2026)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !got.GroupsTruncated {
		t.Error("GroupsTruncated = false, want true past 25 groups")
	}
	if got.Totals.Groups != 30 {
		t.Errorf("Totals.Groups = %d, want every group counted", got.Totals.Groups)
	}
	if len(got.RecentWarnings) != statistics.MaxDashboardRecent {
		t.Errorf("len(RecentWarnings) = %d, want %d", len(got.RecentWarnings), statistics.MaxDashboardRecent)
	}
	if got.TopRisk == nil || got.RecentWarnings == nil {
		t.Error("ranking and recents must be empty arrays, never nil")
	}
}

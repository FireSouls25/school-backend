package statistics

import (
	"context"
	"sort"
	"strings"
	"time"
)

// Dashboard window and caps. The dashboard folds one ClassReport per
// class-group of the requested year, so Groups caps the sweep: past the
// cap the summary still answers with the first groups (ListByYear order:
// grade, then group) and flags the cut instead of timing out.
const (
	// DashboardWindowDays is the trailing window (in days) summarized
	// for llamados and attendance marks.
	DashboardWindowDays = 7
	// MaxDashboardGroups caps how many class-groups attendance and
	// top-risk fold per year.
	MaxDashboardGroups = 25
	// MaxDashboardTopRisk caps the inasistencia risk ranking.
	MaxDashboardTopRisk = 8
	// MaxDashboardRecent caps the latest llamados list.
	MaxDashboardRecent = 6
)

// DashboardTotals counts the school for the requested year: every teacher
// on staff, every student enrolled in the year and every class-group.
// TeacherTotal is staff-wide (not year-scoped); see the API docs.
type DashboardTotals struct {
	Teachers int
	Students int
	Groups   int
}

// DashboardWarnings tallies window llamados by gravity.
type DashboardWarnings struct {
	Mild     int
	Moderate int
	Severe   int
}

// DashboardMarks tallies window attendance marks. Presences counts roster
// rows without a mark.
type DashboardMarks struct {
	Presences int
	Absences  int
	Evasions  int
	Lates     int
}

// DashboardWarning is one recent llamado reduced to its card fields.
type DashboardWarning struct {
	ID        string
	StudentID string
	ClassID   string
	TeacherID string
	Gravity   string
	Title     string
	Date      time.Time
}

// DashboardSummary is the admin landing aggregate for one school year:
// totals, window tallies, the inasistencia ranking and the latest
// llamados. Anything the current sources cannot answer cheaply stays
// out and documented instead of invented (see the API docs).
type DashboardSummary struct {
	Year   int
	Totals DashboardTotals
	// GroupsTruncated reports that the year holds more than
	// MaxDashboardGroups and attendance/top-risk folded only the first
	// ones.
	GroupsTruncated bool
	WarningsLast7d  DashboardWarnings
	MarksLast7d     DashboardMarks
	TopRisk         []StudentSummary
	RecentWarnings  []DashboardWarning
}

// DashboardSource reads the dashboard-wide inputs the per-group and
// per-student sources cannot answer: school-wide totals, the groups of a
// year, enrollments per group and the global recent llamados. Only the
// composition root implements it, over the existing capabilities.
type DashboardSource interface {
	// TeacherTotal counts every teacher on staff.
	TeacherTotal(ctx context.Context) (int, error)
	// GroupsOfYear returns every class-group id of the calendar year,
	// in ListByYear order. Unknown years fail with the schoolyears
	// not-found error.
	GroupsOfYear(ctx context.Context, year int) ([]string, error)
	// EnrolledIDs returns every student id enrolled in the group.
	EnrolledIDs(ctx context.Context, classGroupID string) ([]string, error)
	// RecentWarnings returns every llamado happened at or after since,
	// newest first.
	RecentWarnings(ctx context.Context, since time.Time) ([]DashboardWarning, error)
}

// DashboardService aggregates the admin landing summary from the same
// session/warning/fault views as the other reports plus one
// dashboard-wide source. It persists nothing.
type DashboardService struct {
	reports  *Service
	sessions SessionSource
	dash     DashboardSource
}

// NewDashboardService wires a DashboardService onto the report sources
// and the dashboard-wide source.
func NewDashboardService(sessions SessionSource, warnings WarningSource, faults FaultSource, dash DashboardSource) *DashboardService {
	return &DashboardService{
		reports:  NewService(sessions, warnings, faults),
		sessions: sessions,
		dash:     dash,
	}
}

// Summary builds the dashboard aggregate for the calendar year:
// school totals, window tallies over the year's groups, the top
// inasistencia ranking and the latest llamados.
func (d *DashboardService) Summary(ctx context.Context, year int) (DashboardSummary, error) {
	since := time.Now().AddDate(0, 0, -DashboardWindowDays)

	groupIDs, err := d.dash.GroupsOfYear(ctx, year)
	if err != nil {
		return DashboardSummary{}, err
	}
	summary := DashboardSummary{Year: year}
	summary.Totals.Groups = len(groupIDs)

	teachers, err := d.dash.TeacherTotal(ctx)
	if err != nil {
		return DashboardSummary{}, err
	}
	summary.Totals.Teachers = teachers

	enrolled := make(map[string]struct{})
	for _, gid := range groupIDs {
		ids, err := d.dash.EnrolledIDs(ctx, gid)
		if err != nil {
			return DashboardSummary{}, err
		}
		for _, id := range ids {
			enrolled[id] = struct{}{}
		}
	}
	summary.Totals.Students = len(enrolled)

	folded := groupIDs
	if len(folded) > MaxDashboardGroups {
		folded = folded[:MaxDashboardGroups]
		summary.GroupsTruncated = true
	}

	risk := make(map[string]*StudentSummary)
	for _, gid := range folded {
		// Top-risk reuses the per-group report, so the ranking can
		// never drift from what the class view shows.
		rep, err := d.reports.ClassReport(ctx, gid)
		if err != nil {
			return DashboardSummary{}, err
		}
		for _, st := range rep.Students {
			acc, ok := risk[st.StudentID]
			if !ok {
				cp := st
				risk[st.StudentID] = &cp
				continue
			}
			acc.Sessions += st.Sessions
			acc.Presences += st.Presences
			acc.Absences += st.Absences
			acc.Evasions += st.Evasions
			acc.Lates += st.Lates
		}
		// The window tally needs session dates, which the report
		// folds away, so it reads the same group views directly.
		views, err := d.sessions.GroupSessions(ctx, gid)
		if err != nil {
			return DashboardSummary{}, err
		}
		for _, v := range views {
			if v.Date.Before(since) {
				continue
			}
			for _, e := range v.Roster {
				switch v.Marks[e.StudentID] {
				case "absence":
					summary.MarksLast7d.Absences++
				case "evasion":
					summary.MarksLast7d.Evasions++
				case "late":
					summary.MarksLast7d.Lates++
				default:
					summary.MarksLast7d.Presences++
				}
			}
		}
	}
	for _, acc := range risk {
		if acc.Absences > 0 {
			summary.TopRisk = append(summary.TopRisk, *acc)
		}
	}
	sort.Slice(summary.TopRisk, func(i, j int) bool {
		if summary.TopRisk[i].Absences != summary.TopRisk[j].Absences {
			return summary.TopRisk[i].Absences > summary.TopRisk[j].Absences
		}
		a := strings.ToLower(summary.TopRisk[i].Surnames + " " + summary.TopRisk[i].Names)
		b := strings.ToLower(summary.TopRisk[j].Surnames + " " + summary.TopRisk[j].Names)
		if a == b {
			return summary.TopRisk[i].StudentID < summary.TopRisk[j].StudentID
		}
		return a < b
	})
	if len(summary.TopRisk) > MaxDashboardTopRisk {
		summary.TopRisk = summary.TopRisk[:MaxDashboardTopRisk]
	}
	if summary.TopRisk == nil {
		summary.TopRisk = []StudentSummary{}
	}

	recents, err := d.dash.RecentWarnings(ctx, since)
	if err != nil {
		return DashboardSummary{}, err
	}
	for _, w := range recents {
		switch w.Gravity {
		case "mild":
			summary.WarningsLast7d.Mild++
		case "moderate":
			summary.WarningsLast7d.Moderate++
		case "severe":
			summary.WarningsLast7d.Severe++
		}
	}
	if len(recents) > MaxDashboardRecent {
		recents = recents[:MaxDashboardRecent]
	}
	if recents == nil {
		recents = []DashboardWarning{}
	}
	summary.RecentWarnings = recents

	return summary, nil
}

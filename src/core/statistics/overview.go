package statistics

import (
	"context"
	"sort"
	"time"
)

// OverviewPeriod dates one academic period for the overview scope: the
// 1-based number plus its date-only bounds.
type OverviewPeriod struct {
	N     int
	Start time.Time
	End   time.Time
}

// OverviewMarks tallies attendance marks inside the overview scope.
// StudentDays counts every roster row folded (one student in one roll
// call); Presences counts rows without a mark. Rates must divide by
// StudentDays, never by session counts.
type OverviewMarks struct {
	Presences   int
	Absences    int
	Excused     int
	Evasions    int
	Lates       int
	StudentDays int
}

// OverviewWarnings tallies llamados inside the overview scope by
// gravity. Warnings carry no period, so they are scoped by date against
// the period ranges.
type OverviewWarnings struct {
	Mild     int
	Moderate int
	Severe   int
}

// OverviewSalon carries one salon's scoped tally plus its rate and the
// days it called roll.
type OverviewSalon struct {
	Label       string
	Days        int
	Presences   int
	Absences    int
	Excused     int
	Evasions    int
	Lates       int
	StudentDays int
	Rate        int
}

// Overview is the year (or year+period) aggregate behind the admin
// dashboard charts and the year export: every number derives from the
// roll calls actually taken, so charts can never drift from the class
// views. Days lists the distinct dates with at least one roll call in
// the scope, date-only ISO (YYYY-MM-DD), oldest first.
type Overview struct {
	Year     int
	Period   int
	Periods  []OverviewPeriod
	Days     []string
	Marks    OverviewMarks
	Warnings OverviewWarnings
	Salons   []OverviewSalon
}

// PeriodSpan is one dated period bound used to scope warnings. Sessions
// carry their own period number and need no ranges.
type PeriodSpan struct {
	Start time.Time
	End   time.Time
}

func foldMark(m *OverviewMarks, mark string) {
	m.StudentDays++
	switch mark {
	case "absence":
		m.Absences++
	case "absence-permission":
		m.Excused++
	case "evasion":
		m.Evasions++
	case "late":
		m.Lates++
	default:
		m.Presences++
	}
}

// Overview builds the year (period 0) or year+period aggregate:
// distinct roll-call days, mark tallies, llamados by gravity and the
// per-salon breakdown. Unknown years fail with the schoolyears
// not-found error from GroupsOfYear; out-of-range periods answer empty
// (no invented data).
func (d *DashboardService) Overview(ctx context.Context, year, period int, spans []PeriodSpan) (Overview, error) {
	groupIDs, err := d.dash.GroupsOfYear(ctx, year)
	if err != nil {
		return Overview{}, err
	}
	out := Overview{Year: year, Period: period}
	for i, sp := range spans {
		out.Periods = append(out.Periods, OverviewPeriod{N: i + 1, Start: sp.Start, End: sp.End})
	}
	if out.Periods == nil {
		out.Periods = []OverviewPeriod{}
	}

	// Sessions carry their own period number, so they filter even for
	// legacy years without dated ranges; warnings only scope by date
	// when the range exists.
	var span *PeriodSpan
	if period > 0 && period <= len(spans) {
		span = &spans[period-1]
	}

	days := make(map[string]struct{})
	salonDays := make(map[string]struct{})
	salons := make(map[string]*OverviewSalon)
	for _, gid := range groupIDs {
		views, err := d.sessions.GroupSessions(ctx, gid)
		if err != nil {
			return Overview{}, err
		}
		for _, v := range views {
			if v.SchoolYear != year {
				continue
			}
			if period > 0 && v.Period != period {
				continue
			}
			day := v.Date.UTC().Format("2006-01-02")
			days[day] = struct{}{}
			sal, ok := salons[v.ClassLabel]
			if !ok {
				sal = &OverviewSalon{Label: v.ClassLabel}
				salons[v.ClassLabel] = sal
			}
			if _, seen := salonDays[v.ClassLabel+"\x00"+day]; !seen {
				salonDays[v.ClassLabel+"\x00"+day] = struct{}{}
				sal.Days++
			}
			for _, e := range v.Roster {
				foldMark(&out.Marks, v.Marks[e.StudentID])
				sal.StudentDays++
				switch v.Marks[e.StudentID] {
				case "absence":
					sal.Absences++
				case "absence-permission":
					sal.Excused++
				case "evasion":
					sal.Evasions++
				case "late":
					sal.Lates++
				default:
					sal.Presences++
				}
			}
		}
	}
	out.Days = make([]string, 0, len(days))
	for day := range days {
		out.Days = append(out.Days, day)
	}
	sort.Strings(out.Days)
	for _, sal := range salons {
		if sal.StudentDays > 0 {
			sal.Rate = sal.Presences * 100 / sal.StudentDays
		}
		out.Salons = append(out.Salons, *sal)
	}
	sort.Slice(out.Salons, func(i, j int) bool { return out.Salons[i].Label < out.Salons[j].Label })
	if out.Salons == nil {
		out.Salons = []OverviewSalon{}
	}

	recents, err := d.dash.RecentWarnings(ctx, time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		return Overview{}, err
	}
	for _, w := range recents {
		if w.Date.Year() != year {
			continue
		}
		if span != nil && (w.Date.Before(span.Start) || w.Date.After(span.End)) {
			continue
		}
		switch w.Gravity {
		case "mild":
			out.Warnings.Mild++
		case "moderate":
			out.Warnings.Moderate++
		case "severe":
			out.Warnings.Severe++
		}
	}
	return out, nil
}

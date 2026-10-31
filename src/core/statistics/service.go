package statistics

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StudentSummary aggregates one student's attendance within a report.
type StudentSummary struct {
	StudentID string
	Names     string
	Surnames  string
	// Sessions counts calls whose roster included the student.
	Sessions int
	// Presences counts roster calls without a mark.
	Presences int
	// Absences counts plain absences (falta injustificada).
	Absences int
	// Excused counts absences covered by a school permission
	// (falta con permiso).
	Excused int
	Evasions int
	Lates    int
	// LateStreak is the longest run of consecutive late marks.
	LateStreak int
}

// AttendanceRate returns presences over sessions, or 0 without sessions.
func (s StudentSummary) AttendanceRate() float64 {
	if s.Sessions == 0 {
		return 0
	}
	return float64(s.Presences) / float64(s.Sessions)
}

// ClassTotals aggregates a whole class-group report.
type ClassTotals struct {
	Sessions  int
	Students  int
	Presences int
	// Absences counts plain absences (falta injustificada).
	Absences int
	// Excused counts absences covered by a school permission
	// (falta con permiso).
	Excused  int
	Evasions int
	Lates    int
}

// ClassReport is the per-class-group view admins filter by year and group:
// one summary per roster student plus class totals.
type ClassReport struct {
	ClassGroupID string
	Sessions     int
	Students     []StudentSummary
	Totals       ClassTotals
}

// SessionEntry is one call inside a student's history with its mark.
// Mark is "" for present, otherwise "absence", "absence-permission",
// "evasion" or "late".
type SessionEntry struct {
	SessionID  string
	Date       time.Time
	ClassLabel string
	SchoolYear int
	Period     int
	Mark       string
	// Note is the latest revision note of the session, if any: the
	// recorded reason of a permission lives here.
	Note string
	// UniformOK is false only when a uniform fault was recorded for
	// this session; missing records count as proper uniform.
	UniformOK bool
}

// StudentReport is the full per-student view across years: session
// history as it was recorded, mark totals, and warning/fault tallies.
type StudentReport struct {
	StudentID string
	Sessions  []SessionEntry
	// Defenses maps session id to the recorded defense text, if any.
	Defenses  map[string]string
	Presences int
	// Absences counts plain absences (falta injustificada).
	Absences int
	// Excused counts absences covered by a school permission
	// (falta con permiso).
	Excused  int
	Evasions int
	Lates    int
	// UniformFaults counts sessions with a recorded uniform fault.
	UniformFaults int
	// LateStreak is the longest run of consecutive late marks, oldest to
	// newest. Several in a row should raise an alert.
	LateStreak int
	// Warnings tallies llamados by gravity.
	WarningsMild     int
	WarningsModerate int
	WarningsSevere   int
	// Faults tallies faults by severity.
	FaultsMinor    int
	FaultsOrdinary int
	FaultsSevere   int
}

// Service aggregates reports from reader ports wired at the composition
// root. It persists nothing.
type Service struct {
	sessions SessionSource
	warnings WarningSource
	faults   FaultSource
}

// NewService wires a Service onto its sources.
func NewService(sessions SessionSource, warnings WarningSource, faults FaultSource) *Service {
	return &Service{sessions: sessions, warnings: warnings, faults: faults}
}

// ClassReport builds the per-class-group report: one summary per roster
// student, ordered alphabetically by surnames then names, plus totals.
func (s *Service) ClassReport(ctx context.Context, classGroupID string) (ClassReport, error) {
	if _, err := uuid.Parse(strings.TrimSpace(classGroupID)); err != nil {
		return ClassReport{}, ErrInvalidClassGroup
	}
	views, err := s.sessions.GroupSessions(ctx, strings.TrimSpace(classGroupID))
	if err != nil {
		return ClassReport{}, err
	}
	ordered := append([]SessionView(nil), views...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Date.Equal(ordered[j].Date) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Date.Before(ordered[j].Date)
	})
	streak := map[string]int{}
	best := map[string]int{}
	for _, v := range ordered {
		seen := map[string]bool{}
		for _, e := range v.Roster {
			seen[e.StudentID] = true
			if v.Marks[e.StudentID] == "late" {
				streak[e.StudentID]++
				if streak[e.StudentID] > best[e.StudentID] {
					best[e.StudentID] = streak[e.StudentID]
				}
			} else {
				streak[e.StudentID] = 0
			}
		}
		for id := range streak {
			if !seen[id] {
				streak[id] = 0
			}
		}
	}
	byID := make(map[string]*StudentSummary)
	for _, v := range views {
		for _, e := range v.Roster {
			sum, ok := byID[e.StudentID]
			if !ok {
				sum = &StudentSummary{StudentID: e.StudentID, Names: e.Names, Surnames: e.Surnames, LateStreak: best[e.StudentID]}
				byID[e.StudentID] = sum
			}
			sum.Sessions++
			switch v.Marks[e.StudentID] {
			case "absence":
				sum.Absences++
			case "absence-permission":
				sum.Excused++
			case "evasion":
				sum.Evasions++
			case "late":
				sum.Lates++
			default:
				sum.Presences++
			}
		}
	}
	report := ClassReport{ClassGroupID: strings.TrimSpace(classGroupID), Sessions: len(views)}
	for _, sum := range byID {
		report.Students = append(report.Students, *sum)
		report.Totals.Students++
		report.Totals.Presences += sum.Presences
		report.Totals.Absences += sum.Absences
		report.Totals.Excused += sum.Excused
		report.Totals.Evasions += sum.Evasions
		report.Totals.Lates += sum.Lates
	}
	report.Totals.Sessions = len(views)
	sort.Slice(report.Students, func(i, j int) bool {
		a := strings.ToLower(report.Students[i].Surnames + " " + report.Students[i].Names)
		b := strings.ToLower(report.Students[j].Surnames + " " + report.Students[j].Names)
		if a == b {
			return report.Students[i].StudentID < report.Students[j].StudentID
		}
		return a < b
	})
	return report, nil
}

// StudentReport builds the full per-student view across years.
func (s *Service) StudentReport(ctx context.Context, studentID string) (StudentReport, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return StudentReport{}, ErrInvalidStudent
	}
	studentID = strings.TrimSpace(studentID)

	views, err := s.sessions.StudentSessions(ctx, studentID)
	if err != nil {
		return StudentReport{}, err
	}
	notes, err := s.sessions.StudentSessionNotes(ctx, studentID)
	if err != nil {
		return StudentReport{}, err
	}
	defenses, err := s.sessions.StudentSessionDefenses(ctx, studentID)
	if err != nil {
		return StudentReport{}, err
	}
	report := StudentReport{StudentID: studentID, Defenses: defenses}
	ordered := append([]SessionView(nil), views...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Date.Equal(ordered[j].Date) {
			return ordered[i].ID < ordered[j].ID
		}
		return ordered[i].Date.Before(ordered[j].Date)
	})
	run := 0
	for _, v := range ordered {
		if v.Marks[studentID] == "late" {
			run++
			if run > report.LateStreak {
				report.LateStreak = run
			}
		} else {
			run = 0
		}
	}

	for _, v := range views {
		mark := v.Marks[studentID]
		uniformOK := true
		if bad, explicit := v.Uniform[studentID]; explicit && !bad {
			uniformOK = false
			report.UniformFaults++
		}
		report.Sessions = append(report.Sessions, SessionEntry{
			SessionID:  v.ID,
			Date:       v.Date,
			ClassLabel: v.ClassLabel,
			SchoolYear: v.SchoolYear,
			Period:     v.Period,
			Mark:       mark,
			Note:       notes[v.ID],
			UniformOK:  uniformOK,
		})
		switch mark {
		case "absence":
			report.Absences++
		case "absence-permission":
			report.Excused++
		case "evasion":
			report.Evasions++
		case "late":
			report.Lates++
		default:
			report.Presences++
		}
	}

	warns, err := s.warnings.StudentWarnings(ctx, studentID)
	if err != nil {
		return StudentReport{}, err
	}
	for _, w := range warns {
		switch w.Gravity {
		case "mild":
			report.WarningsMild++
		case "moderate":
			report.WarningsModerate++
		case "severe":
			report.WarningsSevere++
		}
	}

	faults, err := s.faults.StudentFaults(ctx, studentID)
	if err != nil {
		return StudentReport{}, err
	}
	for _, f := range faults {
		switch f.Severity {
		case "minor":
			report.FaultsMinor++
		case "ordinary":
			report.FaultsOrdinary++
		case "severe":
			report.FaultsSevere++
		}
	}
	return report, nil
}

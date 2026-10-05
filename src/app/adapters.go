// Adapters bridging capabilities at the composition root: the reader
// ports statistics and schedules consume, implemented over the sessions,
// subjects, warnings and incidents services. Only the composition root
// may depend on concrete services from several capabilities at once.
package app

import (
	"context"

	"grade/src/core/incidents"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/subjects"
	"grade/src/core/warnings"
)

// subjectChecker implements schedules.Checker on top of the subjects
// capability.
type subjectChecker struct {
	subjects *subjects.Service
}

// Teaches implements schedules.Checker.
func (c subjectChecker) Teaches(ctx context.Context, teacherID, subjectID string) (bool, error) {
	current, err := c.subjects.CurrentForTeacher(ctx, teacherID)
	if err != nil {
		return false, err
	}
	for _, a := range current {
		if a.SubjectID == subjectID {
			return true, nil
		}
	}
	return false, nil
}

// statisticsSessions implements statistics.SessionSource on top of the
// sessions capability.
type statisticsSessions struct {
	sessions *sessions.Service
}

// GroupSessions implements statistics.SessionSource.
func (a statisticsSessions) GroupSessions(ctx context.Context, classGroupID string) ([]statistics.SessionView, error) {
	list, err := a.sessions.SessionsForGroup(ctx, classGroupID)
	if err != nil {
		return nil, err
	}
	return a.withMarks(ctx, list)
}

// StudentSessions implements statistics.SessionSource.
func (a statisticsSessions) StudentSessions(ctx context.Context, studentID string) ([]statistics.SessionView, error) {
	list, err := a.sessions.SessionsForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	return a.withMarks(ctx, list)
}

func (a statisticsSessions) withMarks(ctx context.Context, list []sessions.Session) ([]statistics.SessionView, error) {
	out := make([]statistics.SessionView, 0, len(list))
	for _, sess := range list {
		detail, err := a.sessions.SessionDetail(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		view := statistics.SessionView{
			ID:           sess.ID,
			ClassGroupID: sess.ClassGroupID,
			ClassLabel:   sess.ClassLabel,
			SchoolYear:   sess.SchoolYear,
			Date:         sess.Date,
			Period:       sess.Period,
			Marks:        make(map[string]string, len(detail.Marks)),
		}
		for _, e := range sess.Roster {
			view.Roster = append(view.Roster, statistics.RosterEntryView{
				StudentID: e.StudentID, Names: e.Names, Surnames: e.Surnames,
			})
		}
		for id, mark := range detail.Marks {
			view.Marks[id] = mark.String()
		}
		out = append(out, view)
	}
	return out, nil
}

// statisticsWarnings implements statistics.WarningSource on top of the
// warnings capability.
type statisticsWarnings struct {
	warnings *warnings.Service
}

// StudentWarnings implements statistics.WarningSource.
func (a statisticsWarnings) StudentWarnings(ctx context.Context, studentID string) ([]statistics.WarningView, error) {
	list, err := a.warnings.ForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	out := make([]statistics.WarningView, 0, len(list))
	for _, w := range list {
		out = append(out, statistics.WarningView{Gravity: w.Gravity.String(), Date: w.HappenedAt})
	}
	return out, nil
}

// statisticsFaults implements statistics.FaultSource on top of the
// incidents capability.
type statisticsFaults struct {
	incidents *incidents.Service
}

// StudentFaults implements statistics.FaultSource.
func (a statisticsFaults) StudentFaults(ctx context.Context, studentID string) ([]statistics.FaultView, error) {
	list, err := a.incidents.ForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	out := make([]statistics.FaultView, 0, len(list))
	for _, f := range list {
		out = append(out, statistics.FaultView{Severity: f.Severity.String(), Date: f.Date})
	}
	return out, nil
}

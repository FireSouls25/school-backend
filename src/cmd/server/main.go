package main

import (
	"context"
	"log/slog"
	"os"

	"grade/src/core/attendance"
	"grade/src/core/classes"
	"grade/src/core/enrollments"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/schedules"
	"grade/src/core/schoolyears"
	"grade/src/core/sessions"
	"grade/src/core/statistics"
	"grade/src/core/students"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	"grade/src/core/warnings"
	"grade/src/platform/config"
	httpapi "grade/src/platform/http"
	"grade/src/platform/i18n"
	pg "grade/src/platform/postgres"
)

func main() {
	cfg := config.FromEnv()
	ctx := context.Background()

	i18nSvc, err := i18n.NewService()
	if err != nil {
		slog.Error("i18n initialization failed", "error", err)
		os.Exit(1)
	}

	var (
		studentsStore    students.Store    = students.NewMemoryStore()
		attendanceStore  attendance.Store  = attendance.NewMemoryStore()
		incidentsStore   incidents.Store   = incidents.NewMemoryStore()
		warningsStore    warnings.Store    = warnings.NewMemoryStore()
		teachersStore    teachers.Store    = teachers.NewMemoryStore()
		subjectsStore    subjects.Store    = subjects.NewMemoryStore()
		yearsStore       schoolyears.Store = schoolyears.NewMemoryStore()
		classesStore     classes.Store     = classes.NewMemoryStore()
		enrollmentsStore enrollments.Store = enrollments.NewMemoryStore()
		schedulesStore   schedules.Store   = schedules.NewMemoryStore()
		sessionsStore    sessions.Store    = sessions.NewMemoryStore()
	)
	if cfg.DatabaseURL != "" {
		db, err := pg.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			slog.Error("postgres connection failed", "error", err)
			os.Exit(1)
		}
		defer db.Close()
		studentsStore = pg.NewStudentsStore(db)
		attendanceStore = pg.NewAttendanceStore(db)
		incidentsStore = pg.NewIncidentsStore(db)
		warningsStore = pg.NewWarningsStore(db)
		teachersStore = pg.NewTeachersStore(db)
		subjectsStore = pg.NewSubjectsStore(db)
		yearsStore = pg.NewSchoolYearsStore(db)
		classesStore = pg.NewClassesStore(db)
		enrollmentsStore = pg.NewEnrollmentsStore(db)
		schedulesStore = pg.NewSchedulesStore(db)
		sessionsStore = pg.NewSessionsStore(db)
		slog.Info("using postgres persistence")
	} else {
		slog.Warn("DATABASE_URL not set; using in-memory stores (development only)")
	}

	_ = students.NewService(studentsStore)
	_ = attendance.NewService(attendanceStore)
	_ = teachers.NewService(teachersStore)
	_ = schoolyears.NewService(yearsStore)
	_ = classes.NewService(classesStore)
	_ = enrollments.NewService(enrollmentsStore)

	subjectsSvc := subjects.NewService(subjectsStore)
	_ = schedules.NewService(schedulesStore, subjectChecker{subjectsSvc})
	sessionsSvc := sessions.NewService(sessionsStore)
	warningsSvc := warnings.NewService(warningsStore)
	incidentsSvc := incidents.NewService(incidentsStore)
	_ = statistics.NewService(
		statisticsSessions{sessionsSvc},
		statisticsWarnings{warningsSvc},
		statisticsFaults{incidentsSvc},
	)

	roleStore := roles.NewMemoryStore()
	roles.NewService(roleStore)

	addr := ":" + cfg.Port
	slog.Info("starting server", "addr", addr)
	if err := httpapi.Run(addr, httpapi.Router(i18nSvc)); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// subjectChecker implements schedules.Checker on top of the subjects
// capability. It lives here because only the composition root may wire
// concrete types across capabilities.
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

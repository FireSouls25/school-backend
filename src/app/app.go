// Package app is the composition root: the single place allowed to pick
// concrete adapters and wire services together. Both binaries build on it
// (cmd/server, cmd/devseed) so they can never drift apart in how stores
// and services are built.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

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
	"grade/src/core/users"
	"grade/src/core/warnings"
	"grade/src/devseed"
	"grade/src/platform/config"
	pg "grade/src/platform/postgres"
)

// Stores holds the persistence ports, already bound to a real adapter:
// PostgreSQL when DATABASE_URL is set, in-memory otherwise (development).
type Stores struct {
	Students    students.Store
	Attendance  attendance.Store
	Incidents   incidents.Store
	Warnings    warnings.Store
	Teachers    teachers.Store
	Subjects    subjects.Store
	Years       schoolyears.Store
	Classes     classes.Store
	Enrollments enrollments.Store
	Schedules   schedules.Store
	Sessions    sessions.Store
	Users       users.Store
	// Roles is still in-memory: the Postgres adapter lands with the auth
	// phase, which also makes grants survive restarts.
	Roles roles.Store
}

// Services holds the application services built over those stores.
type Services struct {
	Students    *students.Service
	Attendance  *attendance.Service
	Incidents   *incidents.Service
	Warnings    *warnings.Service
	Teachers    *teachers.Service
	Subjects    *subjects.Service
	Years       *schoolyears.Service
	Classes     *classes.Service
	Enrollments *enrollments.Service
	Schedules   *schedules.Service
	Sessions    *sessions.Service
	Statistics  *statistics.Service
	Users       *users.Service
	Roles       *roles.Service
}

// App is the wired application.
type App struct {
	Config   config.Config
	Stores   Stores
	Services Services
	// db is nil when the deployment uses the in-memory stores.
	db *pg.DB
}

// New wires stores and services for cfg. The schema is applied when
// PostgreSQL is configured. The caller must Close the result.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	s := Stores{
		Students:    students.NewMemoryStore(),
		Attendance:  attendance.NewMemoryStore(),
		Incidents:   incidents.NewMemoryStore(),
		Warnings:    warnings.NewMemoryStore(),
		Teachers:    teachers.NewMemoryStore(),
		Subjects:    subjects.NewMemoryStore(),
		Years:       schoolyears.NewMemoryStore(),
		Classes:     classes.NewMemoryStore(),
		Enrollments: enrollments.NewMemoryStore(),
		Schedules:   schedules.NewMemoryStore(),
		Sessions:    sessions.NewMemoryStore(),
		Users:       users.NewMemoryStore(),
		Roles:       roles.NewMemoryStore(),
	}

	var db *pg.DB
	if cfg.DatabaseURL != "" {
		connected, err := pg.Connect(ctx, cfg.DatabaseURL)
		if err != nil {
			return nil, fmt.Errorf("app: postgres: %w", err)
		}
		db = connected
		s.Students = pg.NewStudentsStore(db)
		s.Attendance = pg.NewAttendanceStore(db)
		s.Incidents = pg.NewIncidentsStore(db)
		s.Warnings = pg.NewWarningsStore(db)
		s.Teachers = pg.NewTeachersStore(db)
		s.Subjects = pg.NewSubjectsStore(db)
		s.Years = pg.NewSchoolYearsStore(db)
		s.Classes = pg.NewClassesStore(db)
		s.Enrollments = pg.NewEnrollmentsStore(db)
		s.Schedules = pg.NewSchedulesStore(db)
		s.Sessions = pg.NewSessionsStore(db)
		s.Users = pg.NewUsersStore(db)
		slog.Info("using postgres persistence")
	} else {
		slog.Warn("DATABASE_URL not set; using in-memory stores (development only, data lost on restart)")
	}

	subjectsSvc := subjects.NewService(s.Subjects)
	sessionsSvc := sessions.NewService(s.Sessions)
	warningsSvc := warnings.NewService(s.Warnings)
	incidentsSvc := incidents.NewService(s.Incidents)

	return &App{
		Config: cfg,
		Stores: s,
		db:     db,
		Services: Services{
			Students:    students.NewService(s.Students),
			Attendance:  attendance.NewService(s.Attendance),
			Incidents:   incidentsSvc,
			Warnings:    warningsSvc,
			Teachers:    teachers.NewService(s.Teachers),
			Subjects:    subjectsSvc,
			Years:       schoolyears.NewService(s.Years),
			Classes:     classes.NewService(s.Classes),
			Enrollments: enrollments.NewService(s.Enrollments),
			Schedules:   schedules.NewService(s.Schedules, subjectChecker{subjectsSvc}),
			Sessions:    sessionsSvc,
			Statistics: statistics.NewService(
				statisticsSessions{sessionsSvc},
				statisticsWarnings{warningsSvc},
				statisticsFaults{incidentsSvc},
			),
			Users: users.NewService(s.Users),
			Roles: roles.NewService(s.Roles),
		},
	}, nil
}

// Close releases the database connection, if any.
func (a *App) Close() error {
	if a.db != nil {
		a.db.Close()
	}
	return nil
}

// Prepare makes an installation usable: the first admin account and, in
// development, the demo school. Both steps are idempotent, so calling it
// on every boot is the intended behaviour.
func (a *App) Prepare(ctx context.Context) error {
	if err := a.Bootstrap(ctx); err != nil {
		return err
	}
	if !a.Config.SeedDemo {
		return nil
	}
	sum, err := devseed.Run(ctx, devseed.Deps{
		Users:       a.Stores.Users,
		Roles:       a.Stores.Roles,
		Teachers:    a.Services.Teachers,
		Subjects:    a.Services.Subjects,
		Years:       a.Services.Years,
		Classes:     a.Services.Classes,
		Enrollments: a.Services.Enrollments,
		Students:    a.Services.Students,
		Sessions:    a.Services.Sessions,
		Warnings:    a.Services.Warnings,
		Incidents:   a.Services.Incidents,
	})
	if err != nil {
		return err
	}
	if sum.AlreadySeeded {
		slog.Info("demo data", "state", "already seeded")
		// Roles live in memory while the Postgres adapter lands with the
		// auth phase, so every restart must re-grant the demo placeholder
		// roles even when the data itself is already there. Assign is
		// idempotent: re-granting what is already held is a no-op.
		for _, g := range []struct {
			subject string
			role    roles.Role
		}{
			{devseed.AdminSubject, roles.RoleAdmin},
			{devseed.TeacherSubject, roles.RoleTeacher},
		} {
			if err := a.Services.Roles.Assign(ctx, g.subject, g.role); err != nil {
				return fmt.Errorf("app: re-grant demo role: %w", err)
			}
		}
		return nil
	}
	slog.Warn("demo data seeded (development only): do not use real student data",
		"accounts", sum.Accounts, "teachers", sum.Teachers, "students", sum.Students,
		"classes", sum.Classes, "logins", devseed.AdminUsername+", "+devseed.TeacherUsername)
	return nil
}

// Bootstrap creates the configured first admin and grants it the admin
// role. Without it an empty installation has nobody who can create users.
// Idempotent: a second boot reuses the existing username and only ensures
// the role, so restarts against a persisted database never fail on a
// duplicate username.
func (a *App) Bootstrap(ctx context.Context) error {
	b := a.Config.Bootstrap
	if !b.Enabled() {
		return nil
	}
	if existing, err := a.Stores.Users.ByUsername(ctx, b.Username); err == nil {
		if err := a.Services.Roles.Assign(ctx, existing.ID, roles.RoleAdmin); err != nil {
			return fmt.Errorf("app: bootstrap admin role: %w", err)
		}
		slog.Info("bootstrap admin already exists", "username", existing.Username)
		return nil
	}
	created, err := a.Services.Users.Create(ctx, b.Username, b.Email, b.Password)
	if err != nil {
		return fmt.Errorf("app: bootstrap admin: %w", err)
	}
	if err := a.Services.Roles.Assign(ctx, created.ID, roles.RoleAdmin); err != nil {
		return fmt.Errorf("app: bootstrap admin role: %w", err)
	}
	slog.Info("bootstrap admin created", "username", created.Username)
	return nil
}

// SeedDemo runs only the demo data step, ignoring the bootstrap account.
func (a *App) SeedDemo(ctx context.Context) (devseed.Summary, error) {
	return devseed.Run(ctx, devseed.Deps{
		Users:       a.Stores.Users,
		Roles:       a.Stores.Roles,
		Teachers:    a.Services.Teachers,
		Subjects:    a.Services.Subjects,
		Years:       a.Services.Years,
		Classes:     a.Services.Classes,
		Enrollments: a.Services.Enrollments,
		Students:    a.Services.Students,
		Sessions:    a.Services.Sessions,
		Warnings:    a.Services.Warnings,
		Incidents:   a.Services.Incidents,
	})
}

// seedRoles grants role to every subject id listed in a comma-separated
// env var. Legacy test hook: it exists for the placeholder identity
// (X-Subject-ID) used before real authentication. Prefer creating real
// accounts (POST /v1/users) or the bootstrap admin.
func (a *App) seedRoles(ctx context.Context, envVar string, role roles.Role) {
	raw := strings.TrimSpace(os.Getenv(envVar))
	if raw == "" {
		return
	}
	count := 0
	for _, id := range strings.Split(raw, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if err := a.Services.Roles.Assign(ctx, id, role); err != nil {
			slog.Warn("seed role skipped", "env", envVar, "role", role.String(), "error", err)
			continue
		}
		count++
	}
	slog.Info("seed roles granted", "env", envVar, "role", role.String(), "count", count)
}

// SeedPlaceholderRoles applies the legacy SEED_* identity hooks. It is a
// no-op unless those variables are set.
func (a *App) SeedPlaceholderRoles(ctx context.Context) {
	a.seedRoles(ctx, "SEED_ADMINS", roles.RoleAdmin)
	a.seedRoles(ctx, "SEED_TEACHERS", roles.RoleTeacher)
}

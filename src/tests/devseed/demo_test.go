package devseed_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"grade/src/core/classes"
	"grade/src/core/enrollments"
	"grade/src/core/incidents"
	"grade/src/core/roles"
	"grade/src/core/schoolyears"
	"grade/src/core/sessions"
	"grade/src/core/students"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	"grade/src/core/users"
	"grade/src/core/warnings"
	"grade/src/devseed"
)

// memoryDeps wires every port the seeder needs to in-memory stores, so
// the suite exercises the real services and their validation.
type memoryDeps struct {
	deps devseed.Deps

	userStore users.Store
	roleStore roles.Store

	teacherStore  teachers.Store
	subjectStore  subjects.Store
	yearStore     schoolyears.Store
	classStore    classes.Store
	enrollStore   enrollments.Store
	studentStore  students.Store
	sessionStore  sessions.Store
	warningStore  warnings.Store
	incidentStore incidents.Store
}

func newMemoryDeps() *memoryDeps {
	d := &memoryDeps{
		userStore:     users.NewMemoryStore(),
		roleStore:     roles.NewMemoryStore(),
		teacherStore:  teachers.NewMemoryStore(),
		subjectStore:  subjects.NewMemoryStore(),
		yearStore:     schoolyears.NewMemoryStore(),
		classStore:    classes.NewMemoryStore(),
		enrollStore:   enrollments.NewMemoryStore(),
		studentStore:  students.NewMemoryStore(),
		sessionStore:  sessions.NewMemoryStore(),
		warningStore:  warnings.NewMemoryStore(),
		incidentStore: incidents.NewMemoryStore(),
	}
	d.deps = devseed.Deps{
		Users:       d.userStore,
		Roles:       d.roleStore,
		Teachers:    teachers.NewService(d.teacherStore),
		Subjects:    subjects.NewService(d.subjectStore),
		Years:       schoolyears.NewService(d.yearStore),
		Classes:     classes.NewService(d.classStore),
		Enrollments: enrollments.NewService(d.enrollStore),
		Students:    students.NewService(d.studentStore),
		Sessions:    sessions.NewService(d.sessionStore),
		Warnings:    warnings.NewService(d.warningStore),
		Incidents:   incidents.NewService(d.incidentStore),
	}
	return d
}

func TestRunCreatesUsableSchool(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()

	sum, err := devseed.Run(ctx, d.deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if sum.AlreadySeeded {
		t.Fatal("AlreadySeeded on a fresh store")
	}
	if sum.Accounts != 2 || sum.Teachers == 0 || sum.Subjects == 0 || sum.Classes == 0 {
		t.Fatalf("summary = %+v, want a populated school", sum)
	}
	if sum.Students == 0 || sum.Enrollments != sum.Students {
		t.Errorf("students = %d, enrollments = %d, want one enrollment per student",
			sum.Students, sum.Enrollments)
	}
	if sum.Sessions == 0 || sum.Marks == 0 {
		t.Errorf("sessions = %d, marks = %d, want roll calls with marks",
			sum.Sessions, sum.Marks)
	}
	if sum.Warnings == 0 || sum.Faults == 0 {
		t.Errorf("warnings = %d, faults = %d, want both non-zero",
			sum.Warnings, sum.Faults)
	}
}

func TestDemoLoginsMatchTheSPAAccounts(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()
	if _, err := devseed.Run(ctx, d.deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The SPA dev login resolves these two identifiers to fixed ids, so
	// the seeded roles must sit exactly there.
	for _, tc := range []struct {
		username string
		password string
		subject  string
		role     roles.Role
	}{
		{devseed.AdminUsername, devseed.AdminPassword, devseed.AdminSubject, roles.RoleAdmin},
		{devseed.TeacherUsername, devseed.TeacherPassword, devseed.TeacherSubject, roles.RoleTeacher},
	} {
		account, err := d.userStore.ByUsername(ctx, tc.username)
		if err != nil {
			t.Errorf("ByUsername(%q): %v", tc.username, err)
			continue
		}
		if account.ID != tc.subject {
			t.Errorf("%s id = %q, want %q", tc.username, account.ID, tc.subject)
		}
		if _, err := d.userStore.ByID(ctx, tc.subject); err != nil {
			t.Errorf("ByID(%q): %v", tc.subject, err)
		}
		granted, err := d.roleStore.RolesFor(ctx, tc.subject)
		if err != nil {
			t.Errorf("RolesFor(%q): %v", tc.subject, err)
			continue
		}
		if len(granted) != 1 || granted[0] != tc.role {
			t.Errorf("%s roles = %v, want [%s]", tc.username, granted, tc.role)
		}
	}
}

func TestDemoPasswordsAreUsable(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()
	if _, err := devseed.Run(ctx, d.deps); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The seeded accounts authenticate: a demo that cannot log in is
	// useless. bcrypt.DefaultCost is slow, so only one round trip here.
	svc := users.NewService(d.userStore)
	for _, tc := range []struct{ username, password string }{
		{devseed.AdminUsername, devseed.AdminPassword},
		{devseed.TeacherUsername, devseed.TeacherPassword},
	} {
		if _, err := svc.Authenticate(ctx, tc.username, tc.password); err != nil {
			t.Errorf("Authenticate(%q): %v", tc.username, err)
		}
		if _, err := svc.Authenticate(ctx, tc.username, "wrong-pass1"); err == nil {
			t.Errorf("Authenticate(%q, wrong password) = nil error", tc.username)
		}
	}
}

func TestRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()

	first, err := devseed.Run(ctx, d.deps)
	if err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if first.AlreadySeeded {
		t.Fatal("first Run reported AlreadySeeded")
	}

	second, err := devseed.Run(ctx, d.deps)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !second.AlreadySeeded {
		t.Error("second Run did not detect existing demo data")
	}
	if second.Total() != 0 {
		t.Errorf("second Run created %d records, want 0", second.Total())
	}
	if got := first.String(); got == second.String() {
		t.Error("Summary.String() is identical for seeded and already-seeded runs")
	}
}

func TestRunReusesExistingBootstrapAccount(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()

	// A deployment that already bootstrapped an "admin" must still be
	// seedable: the seeder reuses the account instead of colliding.
	svc := users.NewServiceWithCost(d.userStore, 4)
	existing, err := svc.Create(ctx, devseed.AdminUsername, "boss@colegio.edu", "otra-clave1")
	if err != nil {
		t.Fatalf("Create bootstrap admin: %v", err)
	}

	if _, err := devseed.Run(ctx, d.deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	reused, err := d.userStore.ByUsername(ctx, devseed.AdminUsername)
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if reused.ID != existing.ID {
		t.Errorf("admin id = %q, want the pre-existing %q", reused.ID, existing.ID)
	}
	granted, err := d.roleStore.RolesFor(ctx, reused.ID)
	if err != nil || len(granted) != 1 || granted[0] != roles.RoleAdmin {
		t.Errorf("roles = %v (%v), want [admin] on the reused account", granted, err)
	}
}

func TestSeededDataIsSelfConsistent(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()
	sum, err := devseed.Run(ctx, d.deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Every student is enrolled, and every class group has students, so
	// rosters, statistics and reports have something to show.
	groups, err := d.classStore.ListByYear(ctx, mustYearID(ctx, d.yearStore))
	if err != nil {
		t.Fatalf("ListByYear: %v", err)
	}
	if len(groups) != sum.Classes {
		t.Errorf("class groups = %d, want %d", len(groups), sum.Classes)
	}
	for _, g := range groups {
		roster, err := d.enrollStore.EnrollmentsForGroup(ctx, g.ID)
		if err != nil {
			t.Fatalf("EnrollmentsForGroup(%s): %v", g.Label(), err)
		}
		if len(roster) == 0 {
			t.Errorf("class %s has no students", g.Label())
		}
		for _, e := range roster {
			if _, err := uuid.Parse(e.StudentID); err != nil {
				t.Errorf("enrollment %s references an invalid student id %q",
					g.Label(), e.StudentID)
			}
		}
	}

	// Sessions carry a frozen roster and real marks.
	for _, g := range groups {
		list, err := d.sessionStore.SessionsForGroup(ctx, g.ID)
		if err != nil {
			t.Fatalf("SessionsForGroup: %v", err)
		}
		for _, s := range list {
			if len(s.Roster) == 0 {
				t.Errorf("session %s has an empty roster", s.ID)
			}
			detail, err := sessions.NewService(d.sessionStore).SessionDetail(ctx, s.ID)
			if err != nil {
				t.Fatalf("SessionDetail: %v", err)
			}
			for studentID := range detail.Marks {
				found := false
				for _, e := range s.Roster {
					if e.StudentID == studentID {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("mark for %s is outside the frozen roster", studentID)
				}
			}
		}
	}
}

func TestWarningsReferenceSeededTeacher(t *testing.T) {
	ctx := context.Background()
	d := newMemoryDeps()
	if _, err := devseed.Run(ctx, d.deps); err != nil {
		t.Fatalf("Run: %v", err)
	}
	list, err := d.teacherStore.List(ctx)
	if err != nil {
		t.Fatalf("List teachers: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("no teachers seeded")
	}
	// The first seeded teacher is Carlos Mendoza Ruiz, the SPA mock's
	// "carlos.mendoza".
	if list[0].DocumentID == "" || list[0].Names == "" {
		t.Errorf("first teacher = %+v, want a complete profile", list[0])
	}
}

func mustYearID(ctx context.Context, store schoolyears.Store) string {
	years, err := store.List(ctx)
	if err != nil || len(years) == 0 {
		return ""
	}
	return years[0].ID
}

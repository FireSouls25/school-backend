// Package devseed creates the demo school used for development, demos and
// manual testing: a few teachers, subjects, classes, students, enrollments,
// roll calls with marks, warnings and faults — plus the two login accounts
// the SPA dev build expects.
//
// It lives outside core because it wires concrete services across
// capabilities (composition-level code), and outside the server binary
// because demo data must never be a production concern.
//
// Everything is derived from the current calendar year and keyed by a fixed
// marker account, so running it twice is a no-op: the second run detects
// the marker and reports AlreadySeeded. Demo records are ordinary domain
// rows, which is what makes screens, statistics and PDFs testable end to
// end; they are fake people, so no personal data is involved (Ley 1581).
package devseed

import (
	"context"
	"fmt"
	"time"

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
)

// Accounts mirrored by the SPA dev login (frontend/src/domains/auth/
// accounts.ts). Fixed ids are the point: the mock resolves "admin" and
// "carlos.mendoza" to these UUIDs, so seeded roles must live on them.
// Both are removed when real authentication lands.
const (
	// AdminSubject is the demo administrator's user id.
	AdminSubject = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	// TeacherSubject is the demo teacher's user id.
	TeacherSubject = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"

	// AdminUsername and TeacherUsername are the demo logins.
	AdminUsername   = "admin"
	TeacherUsername = "carlos.mendoza"

	// AdminPassword and TeacherPassword are the demo passwords. They
	// satisfy the users password policy and are rejected by
	// config.Validate when APP_ENV=production.
	AdminPassword   = "admin123*"
	TeacherPassword = "docente123*"

	// MarkerSubject and MarkerUsername are the account that proves the
	// demo data is already present. It is not a login anybody uses.
	MarkerSubject  = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	MarkerUsername = "demo.marcador"

	markerPassword = "marcador123"
)

// Deps are the ports and services the seeder writes through. Services are
// used where the domain assigns ids, so demo data exercises real
// validation; the users store is used directly because the two SPA
// accounts need explicit ids.
type Deps struct {
	Users       users.Store
	Roles       roles.Store
	Teachers    *teachers.Service
	Subjects    *subjects.Service
	Years       *schoolyears.Service
	Classes     *classes.Service
	Enrollments *enrollments.Service
	Students    *students.Service
	Sessions    *sessions.Service
	Warnings    *warnings.Service
	Incidents   *incidents.Service
}

// Summary reports what the run did, for startup logs and for the CLI.
type Summary struct {
	// AlreadySeeded is true when the marker account was found and the
	// run returned without touching any data.
	AlreadySeeded bool
	SchoolYear    int
	Accounts      int
	Roles         int
	Teachers      int
	Subjects      int
	Classes       int
	Students      int
	Enrollments   int
	Sessions      int
	Marks         int
	Warnings      int
	Faults        int
}

// Total returns the number of domain records created.
func (s Summary) Total() int {
	return s.Accounts + s.Roles + s.Teachers + s.Subjects + s.Classes +
		s.Students + s.Enrollments + s.Sessions + s.Marks + s.Warnings + s.Faults
}

// String renders a compact human-readable report (startup logs).
func (s Summary) String() string {
	if s.AlreadySeeded {
		return "demo data already present; nothing to do"
	}
	return fmt.Sprintf(
		"demo data seeded: year %d, %d accounts, %d teachers, %d subjects, "+
			"%d classes, %d students, %d enrollments, %d sessions, %d marks, "+
			"%d warnings, %d faults",
		s.SchoolYear, s.Accounts, s.Teachers, s.Subjects, s.Classes,
		s.Students, s.Enrollments, s.Sessions, s.Marks, s.Warnings, s.Faults)
}

// Run creates the demo data, or returns AlreadySeeded when it is already
// there. Callers must be in development: production is refused by
// config.Validate before this ever runs.
func Run(ctx context.Context, deps Deps) (Summary, error) {
	var sum Summary

	if _, err := deps.Users.ByUsername(ctx, MarkerUsername); err == nil {
		sum.AlreadySeeded = true
		return sum, nil
	}

	year := time.Now().Year()
	sum.SchoolYear = year

	// Accounts first: everything else references them (roles, sessions).
	// ensureAccount may reuse a colliding username owned by another id
	// (e.g. the bootstrap admin created before the demo seed runs), so the
	// roles below attach to the fixed placeholder subject ids the SPA
	// actually sends, never to whichever account row won the username.
	if _, err := deps.ensureAccount(ctx, AdminSubject, AdminUsername,
		"admin@observador.edu.co", AdminPassword); err != nil {
		return sum, err
	}
	if _, err := deps.ensureAccount(ctx, TeacherSubject, TeacherUsername,
		"carlos.mendoza@observador.edu.co", TeacherPassword); err != nil {
		return sum, err
	}
	sum.Accounts = 2
	sum.Roles += deps.grant(ctx, AdminSubject, roles.RoleAdmin)
	sum.Roles += deps.grant(ctx, TeacherSubject, roles.RoleTeacher)

	start, end := date(year, 2, 2), date(year, 11, 27)
	schoolYear, err := deps.Years.Create(ctx, schoolyears.SchoolYear{
		Year:      year,
		Periods:   3,
		StartDate: start,
		EndDate:   end,
		Holidays:  holidaysWithin(year, start, end),
	})
	if err != nil {
		return sum, fmt.Errorf("devseed: school year: %w", err)
	}

	groups := map[string]classes.ClassGroup{}
	for _, g := range []struct{ grade, group int }{
		{7, 1}, {7, 2}, {8, 1}, {9, 1}, {9, 2}, {10, 1}, {11, 1},
	} {
		created, err := deps.Classes.Create(ctx, classes.ClassGroup{
			SchoolYearID: schoolYear.ID, Grade: g.grade, GroupNo: g.group,
		})
		if err != nil {
			return sum, fmt.Errorf("devseed: class %d-%d: %w", g.grade, g.group, err)
		}
		groups[label(g.grade, g.group)] = created
		sum.Classes++
	}

	subjectCatalog := map[string]subjects.Subject{}
	for _, name := range []string{"Matemáticas", "Español", "Ciencias Naturales", "Sociales", "Inglés"} {
		created, err := deps.Subjects.CreateSubject(ctx, subjects.Subject{
			Name: name, Code: subjectCode(name), Active: true,
		})
		if err != nil {
			return sum, fmt.Errorf("devseed: subject %s: %w", name, err)
		}
		subjectCatalog[name] = created
		sum.Subjects++
	}

	staff := []struct {
		names, surnames, document, phone, email, homeroom string
	}{
		{"Carlos", "Mendoza Ruiz", "79888777", "3205556677", "carlos.mendoza@observador.edu.co", "9-1"},
		{"Ana", "Ríos Ballard", "51999111", "3112223344", "ana.rios@observador.edu.co", "9-2"},
		{"Julián", "Pardo Sanz", "80111222", "3123334455", "julian.pardo@observador.edu.co", "7-1"},
		{"Laura", "Torres Mejía", "10203040", "3134445566", "laura.torres@observador.edu.co", ""},
		{"Andrés", "Quintero Lotto", "90055667", "3145556677", "andres.quintero@observador.edu.co", ""},
	}
	var teacherProfiles []teachers.Teacher
	for _, p := range staff {
		t, err := deps.Teachers.Create(ctx, teachers.Teacher{
			Names: p.names, Surnames: p.surnames, DocumentID: p.document,
			Phone: p.phone, Email: p.email, HomeroomClassID: p.homeroom,
			MedicalConditions: "Ninguna",
		})
		if err != nil {
			return sum, fmt.Errorf("devseed: teacher %s: %w", p.names, err)
		}
		teacherProfiles = append(teacherProfiles, t)
		sum.Teachers++
	}

	// The demo teacher account and the demo teacher profile are two
	// records by design: the profile holds school data, the account
	// holds credentials. Sessions reference the profile; roles live on
	// the account id.
	for _, a := range []struct {
		teacher int
		subject string
	}{
		{0, "Matemáticas"}, {0, "Ciencias Naturales"}, {1, "Sociales"}, {2, "Inglés"},
	} {
		if _, err := deps.Subjects.Assign(ctx, teacherProfiles[a.teacher].ID,
			subjectCatalog[a.subject].ID, date(year, 2, 2)); err != nil {
			return sum, fmt.Errorf("devseed: assign %s: %w", a.subject, err)
		}
	}

	roster := []struct {
		group     string
		grade     int
		names     []string
		surnames  []string
		documents []string
	}{
		{"7-1", 7, []string{"Mateo", "Sara", "Julián", "Valeria"},
			[]string{"Ospina Gutiérrez", "Bedoya Ruiz", "Cuéllar Pérez", "Mantilla León"},
			[]string{"10204001", "10204002", "10204003", "10204004"}},
		{"7-2", 7, []string{"Tomás", "Isabella"},
			[]string{"Arango Ciro", "Zapata Nieto"},
			[]string{"10204005", "10204006"}},
		{"8-1", 8, []string{"Samuel", "Mariana"},
			[]string{"Hoyos Vidal", "Cardona Ruiz"},
			[]string{"10204007", "10204008"}},
		{"9-1", 9, []string{"Ana", "Pedro", "Luisa", "Daniel", "Sofía", "Camilo"},
			[]string{"Gómez Restrepo", "López Quintero", "Mejía Arenas", "Pineda Toro", "Ríos Salas", "Vélez Hoyos"},
			[]string{"10204009", "10204010", "10204011", "10204012", "10204013", "10204014"}},
		{"9-2", 9, []string{"Laura", "Andrés"},
			[]string{"Salazar Ortiz", "Uribe Nieto"},
			[]string{"10204015", "10204016"}},
		{"10-1", 10, []string{"Juan", "Carolina"},
			[]string{"Brandero Gil", "Castaño Uribe"},
			[]string{"10204017", "10204018"}},
		{"11-1", 11, []string{"Esteban", "Manuela"},
			[]string{"Duque Zapata", "Escobar Londoño"},
			[]string{"10204019", "10204020"}},
	}

	var byGroup = make(map[string][]students.Student, len(roster))
	for _, r := range roster {
		group := groups[r.group]
		for i := range r.names {
			student, err := deps.Students.Create(ctx, students.Student{
				Names: r.names[i], Surnames: r.surnames[i],
				ClassID: group.Label(), DocumentID: r.documents[i],
				Birthplace: "Medellín",
				Birthdate:  date(birthYear(year, r.grade), 3, 1+((i*3)%25)),
				Phone:      "31" + r.documents[i],
				Address:    "Calle 10 # 5-20",
				Email:      "",
				Caregiver: students.Guardian{
					Names:      r.surnames[i] + " (acudiente)",
					Phone:      "32" + r.documents[i],
					Occupation: "Docente",
					Address:    "Calle 10 # 5-20",
				},
				LivesWith: "Padres",
			})
			if err != nil {
				return sum, fmt.Errorf("devseed: student %s %s: %w",
					r.names[i], r.surnames[i], err)
			}
			if _, err := deps.Enrollments.Enroll(ctx, enrollments.Enrollment{
				StudentID: student.ID, ClassGroupID: group.ID,
			}); err != nil {
				return sum, fmt.Errorf("devseed: enroll %s: %w", student.FullName(), err)
			}
			byGroup[r.group] = append(byGroup[r.group], student)
			sum.Students++
			sum.Enrollments++
		}
	}

	// Roll calls for 9-1 with a few marks, so class statistics, the
	// student report and the attendance screens have real data.
	nine := groups["9-1"]
	// Marks are keyed by position in the frozen roster above, so they stay
	// stable across runs.
	rolls := []struct {
		day   time.Time
		marks map[int]sessions.Mark
	}{
		{date(year, 3, 10), map[int]sessions.Mark{}},
		{date(year, 3, 17), map[int]sessions.Mark{1: sessions.MarkLate}},
		{date(year, 4, 7), map[int]sessions.Mark{
			0: sessions.MarkAbsence, 2: sessions.MarkEvasion, 3: sessions.MarkLate,
		}},
	}
	for _, roll := range rolls {
		rosterEntries := make([]sessions.RosterEntry, 0, len(byGroup["9-1"]))
		for _, st := range byGroup["9-1"] {
			rosterEntries = append(rosterEntries, sessions.RosterEntry{
				StudentID: st.ID, Names: st.Names,
				Surnames: st.Surnames, DocumentID: st.DocumentID,
			})
		}
		sess, err := deps.Sessions.OpenSession(ctx, sessions.Session{
			ClassGroupID: nine.ID, ClassLabel: nine.Label(), SchoolYear: year,
			TeacherID: teacherProfiles[0].ID, SubjectID: subjectCatalog["Matemáticas"].ID,
			Date: roll.day, Period: 1, Roster: rosterEntries,
		})
		if err != nil {
			return sum, fmt.Errorf("devseed: session %s: %w", roll.day.Format("2006-01-02"), err)
		}
		sum.Sessions++
		for idx, mark := range roll.marks {
			if _, err := deps.Sessions.RecordMark(ctx, sess.ID,
				byGroup["9-1"][idx].ID, mark, teacherProfiles[0].ID, ""); err != nil {
				return sum, fmt.Errorf("devseed: mark: %w", err)
			}
			sum.Marks++
		}
	}

	// One warning and one fault keep the admin dashboards non-empty.
	target := byGroup["9-1"][0]
	if _, err := deps.Warnings.Issue(ctx, warnings.Input{
		StudentID: target.ID, ClassID: nine.Label(), TeacherID: teacherProfiles[0].ID,
		HappenedAt:  date(year, 3, 17).Add(10 * time.Hour),
		Gravity:     warnings.GravityMild,
		Title:       "Interrupción de la clase",
		Description: "Interrumpió la clase en dos ocasiones durante el primer periodo.",
		Snapshot:    snapshot(target),
	}); err != nil {
		return sum, fmt.Errorf("devseed: warning: %w", err)
	}
	sum.Warnings++

	if _, err := deps.Incidents.Report(ctx, target.ID, nine.Label(), date(year, 3, 24),
		incidents.SeverityMinor, "Uso del celular en clase."); err != nil {
		return sum, fmt.Errorf("devseed: fault: %w", err)
	}
	sum.Faults++

	// The marker makes re-runs a no-op. It carries a fixed id so the
	// store can check it by primary key, like the two SPA accounts.
	if _, err := deps.ensureAccount(ctx, MarkerSubject, MarkerUsername,
		"demo.marcador@observador.edu.co", markerPassword); err != nil {
		return sum, err
	}
	return sum, nil
}

// ensureAccount creates a login account with an explicit id, or reuses the
// existing one when it is already there (by id or by username, e.g. the
// bootstrap admin reusing the "admin" name). Existing passwords are never
// overwritten, so re-running never resets a changed password.
func (d Deps) ensureAccount(ctx context.Context, id, username, email, password string) (users.User, error) {
	if id != "" {
		if existing, err := d.Users.ByID(ctx, id); err == nil {
			return existing, nil
		}
	}
	if existing, err := d.Users.ByUsername(ctx, username); err == nil {
		return existing, nil
	}
	account, err := users.NewAccount(id, username, email, password)
	if err != nil {
		return users.User{}, fmt.Errorf("devseed: account %s: %w", username, err)
	}
	created, err := d.Users.Create(ctx, account)
	if err != nil {
		return users.User{}, fmt.Errorf("devseed: create account %s: %w", username, err)
	}
	return created, nil
}

// grant assigns a role and reports how many roles were actually written.
func (d Deps) grant(ctx context.Context, subjectID string, role roles.Role) int {
	if subjectID == "" {
		return 0
	}
	existing, err := d.Roles.RolesFor(ctx, subjectID)
	if err == nil {
		for _, r := range existing {
			if r == role {
				return 0
			}
		}
	}
	if err := d.Roles.AssignRole(ctx, subjectID, role); err != nil {
		return 0
	}
	return 1
}

func snapshot(st students.Student) warnings.StudentSnapshot {
	age := -1
	if a, ok := st.AgeAt(time.Now()); ok {
		age = a
	}
	return warnings.StudentSnapshot{
		Names: st.Names, Surnames: st.Surnames, DocumentID: st.DocumentID,
		ClassID: st.ClassID, Birthdate: st.Birthdate, Age: age,
		CaregiverName: st.Caregiver.Names, CaregiverPhone: st.Caregiver.Phone,
	}
}

func date(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// birthYear maps a grade to a plausible birth year: Colombian students
// enter grade 1 around age 6, so grade g sits at age 6+g.
func birthYear(year, grade int) int {
	return year - 6 - grade
}

// holidaysWithin keeps only the nationwide holidays inside the school
// calendar. ColombianHolidays returns the whole year (including 1 January
// and the December feasts), which a February-to-November school year
// cannot hold: the domain requires every holiday to be within the range.
func holidaysWithin(year int, start, end time.Time) []time.Time {
	all := schoolyears.ColombianHolidays(year)
	out := make([]time.Time, 0, len(all))
	for _, h := range all {
		if h.Before(start) || h.After(end) {
			continue
		}
		out = append(out, h)
	}
	return out
}

func label(grade, group int) string {
	return fmt.Sprintf("%d-%d", grade, group)
}

// subjectCode returns a short catalog code, or "" when none applies.
func subjectCode(name string) string {
	switch name {
	case "Matemáticas":
		return "MAT"
	case "Español":
		return "ESP"
	case "Ciencias Naturales":
		return "CNAT"
	case "Sociales":
		return "SOC"
	case "Inglés":
		return "ING"
	default:
		return ""
	}
}

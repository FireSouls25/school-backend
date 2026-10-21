package postgres_test

import (
	"context"
	"errors"
	"testing"

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
	pg "grade/src/platform/postgres"
)

// TestDevseedRoundTrip runs the demo seeder against the real schema and
// then re-runs it, covering the parts memory stores cannot check: foreign
// keys between the seeded records (school year → classes → enrollments,
// sessions → rosters) and the partial unique indexes on users.
//
// It exercises the whole stack the way a fresh developer install does, so
// a broken seed fails here instead of on someone's first boot.
func TestDevseedRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	deps := devseed.Deps{
		Users:       pg.NewUsersStore(db),
		Roles:       roles.NewMemoryStore(),
		Teachers:    teachers.NewService(pg.NewTeachersStore(db)),
		Subjects:    subjects.NewService(pg.NewSubjectsStore(db)),
		Years:       schoolyears.NewService(pg.NewSchoolYearsStore(db)),
		Classes:     classes.NewService(pg.NewClassesStore(db)),
		Enrollments: enrollments.NewService(pg.NewEnrollmentsStore(db)),
		Students:    students.NewService(pg.NewStudentsStore(db)),
		Sessions:    sessions.NewService(pg.NewSessionsStore(db)),
		Warnings:    warnings.NewService(pg.NewWarningsStore(db)),
		Incidents:   incidents.NewService(pg.NewIncidentsStore(db)),
	}

	first, err := devseed.Run(ctx, deps)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if first.AlreadySeeded {
		t.Fatal("Run reported AlreadySeeded on a truncated database")
	}
	if first.Students == 0 || first.Sessions == 0 || first.Classes == 0 {
		t.Fatalf("summary = %+v, want a populated school", first)
	}

	// The SPA dev logins must exist with the ids the mock sends.
	for _, id := range []string{devseed.AdminSubject, devseed.TeacherSubject, devseed.MarkerSubject} {
		if _, err := deps.Users.ByID(ctx, id); err != nil {
			t.Errorf("ByID(%s): %v", id, err)
		}
	}

	// Second run: the marker short-circuits it, and the unique indexes on
	// users are never reached.
	second, err := devseed.Run(ctx, deps)
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if !second.AlreadySeeded {
		t.Error("second Run did not detect existing demo data")
	}

	// Seeding twice must not have doubled anything.
	years, err := deps.Years.List(ctx)
	if err != nil {
		t.Fatalf("List years: %v", err)
	}
	if len(years) != 1 {
		t.Errorf("school years = %d, want 1", len(years))
	}
}

// TestDevseedUniqueIndexOnUsernames proves the PostgreSQL adapter
// translates the unique index into the coded duplicate error, so a second
// account with the same username never leaks a raw driver error to the
// API.
func TestDevseedUniqueIndexOnUsernames(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := pg.NewUsersStore(db)

	account, err := users.NewAccount(devseed.AdminSubject, "geminiano", "", "clave1234")
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if _, err := store.Create(ctx, account); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Same username, different id: only the index can catch it.
	clash, err := users.NewAccount(devseed.TeacherSubject, "GEMINIANO", "", "clave1234")
	if err != nil {
		t.Fatalf("NewAccount: %v", err)
	}
	if _, err := store.Create(ctx, clash); !errors.Is(err, users.ErrDuplicateUsername) {
		t.Errorf("duplicate username error = %v, want ErrDuplicateUsername", err)
	}
}

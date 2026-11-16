package postgres_test

import (
	"context"
	"testing"
	"time"

	"grade/src/core/classes"
	"grade/src/core/permits"
	"grade/src/core/schoolyears"
	"grade/src/core/warnings"
	pg "grade/src/platform/postgres"
)

// Permisos previos y descargo de llamados contra PostgreSQL real.
func TestPermitsAndWarningDefense(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	st, err := pg.NewStudentsStore(db).Create(ctx, fullProfile("12121212-1212-4212-8212-121212121212"))
	if err != nil {
		t.Fatalf("Create student: %v", err)
	}
	year, err := pg.NewSchoolYearsStore(db).Create(ctx, schoolyears.SchoolYear{
		ID: "13131313-1313-4313-8313-131313131313", Year: 2028, Periods: 1,
		StartDate: time.Date(2028, 2, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2028, 11, 30, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Create year: %v", err)
	}
	g, err := pg.NewClassesStore(db).Create(ctx, classes.ClassGroup{
		ID: "14141414-1414-4414-8414-141414141414", SchoolYearID: year.ID, Grade: 9, GroupNo: 1,
	})
	if err != nil {
		t.Fatalf("Create class: %v", err)
	}

	day := time.Date(2028, 3, 6, 0, 0, 0, 0, time.UTC)
	ps := pg.NewPermitsStore(db)
	p, err := ps.Create(ctx, permits.Permit{
		ID: "15151515-1515-4515-8515-151515151515", StudentID: st.ID, ClassGroupID: g.ID,
		Date: day, Reason: "Cita médica", CreatedBy: "sec-1", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Create permit: %v", err)
	}
	if list, err := ps.ForGroupOnDate(ctx, g.ID, day); err != nil || len(list) != 1 || list[0].Reason != "Cita médica" {
		t.Fatalf("ForGroupOnDate = %v, %v", list, err)
	}
	if list, err := ps.ForGroupOnDate(ctx, g.ID, day.AddDate(0, 0, 1)); err != nil || len(list) != 0 {
		t.Fatalf("ForGroupOnDate otro día = %v, %v", list, err)
	}
	if list, err := ps.ForStudent(ctx, st.ID); err != nil || len(list) != 1 {
		t.Fatalf("ForStudent = %v, %v", list, err)
	}
	if err := ps.Delete(ctx, p.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := ps.Delete(ctx, p.ID); err == nil {
		t.Error("second Delete = nil, want ErrNotFound")
	}

	ws := pg.NewWarningsStore(db)
	w, err := ws.Add(ctx, warnings.Warning{
		ID: "16161616-1616-4616-8616-161616161616", StudentID: st.ID, ClassID: g.ID, TeacherID: "t-1",
		HappenedAt: day.Add(9 * time.Hour), Gravity: warnings.GravityModerate,
		Title:    "Evadirse de clase o de la institución",
		Snapshot: warnings.StudentSnapshot{Names: "Ana", Surnames: "Gómez", DocumentID: "1", ClassID: "9-1"},
	})
	if err != nil {
		t.Fatalf("Add warning sin descripción: %v", err)
	}
	at := time.Date(2028, 3, 7, 15, 0, 0, 0, time.UTC)
	got, err := ws.SetDefense(ctx, w.ID, "Estaba en enfermería", "sec-1", at)
	if err != nil || got.Defense != "Estaba en enfermería" || got.DefenseBy != "sec-1" || !got.DefenseAt.Equal(at) {
		t.Fatalf("SetDefense = %+v, %v", got, err)
	}
	if again, _ := ws.Fetch(ctx, w.ID); again.Defense != "Estaba en enfermería" {
		t.Errorf("Fetch defense = %q", again.Defense)
	}
	cleared, err := ws.SetDefense(ctx, w.ID, "", "sec-1", at)
	if err != nil || cleared.Defense != "" || cleared.DefenseBy != "" || !cleared.DefenseAt.IsZero() {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	if _, err := ws.SetDefense(ctx, "17171717-1717-4717-8717-171717171717", "x", "y", at); err == nil {
		t.Error("SetDefense on missing warning = nil, want ErrNotFound")
	}
}

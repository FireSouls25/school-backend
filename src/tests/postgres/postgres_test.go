package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"grade/src/core/attendance"
	"grade/src/core/incidents"
	"grade/src/core/students"
	pg "grade/src/platform/postgres"
)

// testDB skips the suite unless TEST_DATABASE_URL points at an isolated
// PostgreSQL database (schema is created and torn down per run).
func testDB(t *testing.T) *pg.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	db, err := pg.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestStudentsStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := pg.NewStudentsStore(db)

	st, err := store.Create(ctx, students.Student{
		ID: "33333333-3333-3333-3333-333333333333", Names: "Ana",
		Surnames: "Gómez", ClassID: "9-1",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if st.ID != "33333333-3333-3333-3333-333333333333" {
		t.Errorf("Create returned id %q", st.ID)
	}

	got, err := store.ByID(ctx, st.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if got != st {
		t.Errorf("ByID = %+v, want %+v", got, st)
	}

	if _, err := store.ByID(ctx, "44444444-4444-4444-4444-444444444444"); !errors.Is(err, students.ErrNotFound) {
		t.Errorf("ByID unknown error = %v, want ErrNotFound", err)
	}

	list, err := store.ListByClass(ctx, "9-1")
	if err != nil {
		t.Fatalf("ListByClass: %v", err)
	}
	if len(list) != 1 || list[0].FullName() != "Gómez Ana" {
		t.Errorf("ListByClass = %+v", list)
	}
}

func TestPhotoRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := pg.NewStudentsStore(db)

	id := "55555555-5555-5555-5555-555555555555"
	if _, err := store.Create(ctx, students.Student{ID: id, Names: "Beto", Surnames: "Ruiz", ClassID: "9-2"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	ph := students.Photo{Data: []byte{0x89, 'P', 'N', 'G', 0x0A}}
	if err := store.PutPhoto(ctx, id, ph); err != nil {
		t.Fatalf("PutPhoto: %v", err)
	}
	got, err := store.Photo(ctx, id)
	if err != nil {
		t.Fatalf("Photo: %v", err)
	}
	if string(got.Data) != string(ph.Data) {
		t.Errorf("photo data mismatch: got %x want %x", got.Data, ph.Data)
	}

	err = store.PutPhoto(ctx, "66666666-6666-6666-6666-666666666666", ph)
	if !errors.Is(err, students.ErrNotFound) {
		t.Errorf("PutPhoto unknown error = %v, want ErrNotFound", err)
	}
}

func TestAttendanceAndIncidentsCascadeOnStudentDelete(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	sid := "77777777-7777-7777-7777-777777777777"

	studentsStore := pg.NewStudentsStore(db)
	if _, err := studentsStore.Create(ctx, students.Student{
		ID: sid, Names: "Carla", Surnames: "Zapata", ClassID: "10-1",
	}); err != nil {
		t.Fatalf("Create student: %v", err)
	}

	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	attStore := pg.NewAttendanceStore(db)
	if _, err := attStore.Add(ctx, attendance.Record{
		ID: "88888888-8888-8888-8888-888888888888", StudentID: sid,
		ClassID: "10-1", Date: day, Reason: attendance.ReasonEvasion,
	}); err != nil {
		t.Fatalf("Add attendance: %v", err)
	}

	incStore := pg.NewIncidentsStore(db)
	fault, err := incStore.Add(ctx, incidents.Fault{
		ID: "99999999-9999-9999-9999-999999999999", StudentID: sid,
		ClassID: "10-1", Date: day, Severity: incidents.SeveritySevere,
		Description: "falta grave en clase",
	})
	if err != nil {
		t.Fatalf("Add fault: %v", err)
	}

	recs, err := attStore.ForStudent(ctx, sid)
	if err != nil || len(recs) != 1 {
		t.Fatalf("ForStudent attendance = %v, %d records", err, len(recs))
	}
	if recs[0].Reason != attendance.ReasonEvasion {
		t.Errorf("reason = %q, want evasion", recs[0].Reason)
	}
	faults, err := incStore.ForStudent(ctx, sid)
	if err != nil || len(faults) != 1 {
		t.Fatalf("ForStudent faults = %v, %d records", err, len(faults))
	}
	if faults[0] != fault {
		t.Errorf("fault = %+v, want %+v", faults[0], fault)
	}

	if err := studentsStore.Delete(ctx, sid); err != nil {
		t.Fatalf("Delete student: %v", err)
	}
	recs, _ = attStore.ForStudent(ctx, sid)
	if len(recs) != 0 {
		t.Errorf("attendance not cascaded: %d records remain", len(recs))
	}
	faults, _ = incStore.ForStudent(ctx, sid)
	if len(faults) != 0 {
		t.Errorf("faults not cascaded: %d records remain", len(faults))
	}
}

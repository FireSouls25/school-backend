package postgres_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"grade/src/core/attendance"
	"grade/src/core/incidents"
	"grade/src/core/students"
	"grade/src/core/subjects"
	"grade/src/core/teachers"
	"grade/src/core/warnings"
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

// fullProfile returns an extended student profile exercising every column.
func fullProfile(id string) students.Student {
	return students.Student{
		ID: id, Names: "Ana", Surnames: "Gómez", ClassID: "9-1",
		DocumentID: "1234567890", Phone: "3001234567", Address: "Calle 10 # 5-20",
		Birthplace: "Bogotá", Birthdate: time.Date(2012, 4, 15, 0, 0, 0, 0, time.UTC),
		Email:     "ana.gomez@example.com",
		Vision:    students.Condition{Has: true, Detail: "Miopía"},
		Hearing:   students.Condition{Has: false},
		BloodType: "O+", IsNew: true,
		PreviousSchool: "Colegio Norte", TransferReason: "Cambio de ciudad",
		RepeatCount: 1,
		Mother: students.Guardian{
			Names: "María Gómez", DocumentID: "51999888",
			Phone: "3101112233", Occupation: "Docente", Address: "Calle 10 # 5-20",
		},
		Father: students.Guardian{
			Names: "José Ruiz", DocumentID: "79999777",
			Phone: "3114445566", Occupation: "Comerciante", Address: "Calle 10 # 5-20",
		},
		Caregiver: students.Guardian{
			Names: "María Gómez", DocumentID: "51999888",
			Phone: "3101112233", Occupation: "Docente", Address: "Calle 10 # 5-20",
		},
		LivesWith: "Padres",
		Siblings: []students.Sibling{
			{Name: "Luis Gómez", ClassID: "7-2"},
			{Name: "Sara Gómez", ClassID: "5-1"},
		},
		MedicalReport:      "Asma leve",
		DiversityCondition: "",
		SpecialistReport:   students.Condition{Has: true, Detail: "Neumología"},
	}
}

func TestStudentsStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := pg.NewStudentsStore(db)

	want := fullProfile("33333333-3333-3333-3333-333333333333")
	st, err := store.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if st.ID != want.ID {
		t.Errorf("Create returned id %q", st.ID)
	}

	got, err := store.ByID(ctx, st.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ByID = %+v, want %+v", got, want)
	}

	// Update exercises the full-column UPDATE path.
	got.ClassID = "9-2"
	got.RepeatCount = 2
	got.Siblings = []students.Sibling{{Name: "Luis Gómez", ClassID: "8-2"}}
	upd, err := store.Update(ctx, got)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !reflect.DeepEqual(upd, got) {
		t.Errorf("Update = %+v, want %+v", upd, got)
	}

	if _, err := store.ByID(ctx, "44444444-4444-4444-4444-444444444444"); !errors.Is(err, students.ErrNotFound) {
		t.Errorf("ByID unknown error = %v, want ErrNotFound", err)
	}

	list, err := store.ListByClass(ctx, "9-2")
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
	prof := fullProfile(id)
	prof.Names, prof.Surnames, prof.DocumentID = "Beto", "Ruiz", "2222222222"
	if _, err := store.Create(ctx, prof); err != nil {
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

func TestHistoriesCascadeOnStudentDelete(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	sid := "77777777-7777-7777-7777-777777777777"

	studentsStore := pg.NewStudentsStore(db)
	prof := fullProfile(sid)
	prof.Names, prof.Surnames, prof.DocumentID, prof.ClassID = "Carla", "Zapata", "3333333333", "10-1"
	if _, err := studentsStore.Create(ctx, prof); err != nil {
		t.Fatalf("Create student: %v", err)
	}

	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	attStore := pg.NewAttendanceStore(db)
	if _, err := attStore.Add(ctx, attendance.Record{
		ID: "88888888-8888-8888-8888-888888888888", StudentID: sid,
		ClassID: "10-1", Date: day, Reason: attendance.ReasonLate,
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

	warnStore := pg.NewWarningsStore(db)
	warning, err := warnStore.Add(ctx, warnings.Warning{
		ID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", StudentID: sid,
		ClassID: "10-1", TeacherID: "teacher-1",
		HappenedAt:  time.Date(2026, 8, 24, 10, 30, 0, 0, time.UTC),
		Gravity:     warnings.GravityModerate,
		Title:       "Interrupción reiterada",
		Description: "Interrumpió la clase en tres ocasiones.",
		Snapshot: warnings.StudentSnapshot{
			Names: "Carla", Surnames: "Zapata", DocumentID: "3333333333",
			ClassID: "10-1", Age: 14,
			CaregiverName: "María Gómez", CaregiverPhone: "3101112233",
		},
	})
	if err != nil {
		t.Fatalf("Add warning: %v", err)
	}

	recs, err := attStore.ForStudent(ctx, sid)
	if err != nil || len(recs) != 1 {
		t.Fatalf("ForStudent attendance = %v, %d records", err, len(recs))
	}
	if recs[0].Reason != attendance.ReasonLate {
		t.Errorf("reason = %q, want late", recs[0].Reason)
	}
	faults, err := incStore.ForStudent(ctx, sid)
	if err != nil || len(faults) != 1 {
		t.Fatalf("ForStudent faults = %v, %d records", err, len(faults))
	}
	if !reflect.DeepEqual(faults[0], fault) {
		t.Errorf("fault = %+v, want %+v", faults[0], fault)
	}
	warns, err := warnStore.ForStudent(ctx, sid)
	if err != nil || len(warns) != 1 {
		t.Fatalf("ForStudent warnings = %v, %d records", err, len(warns))
	}
	if !reflect.DeepEqual(warns[0], warning) {
		t.Errorf("warning = %+v, want %+v", warns[0], warning)
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
	warns, _ = warnStore.ForStudent(ctx, sid)
	if len(warns) != 0 {
		t.Errorf("warnings not cascaded: %d records remain", len(warns))
	}
}

func TestTeachersStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)
	store := pg.NewTeachersStore(db)

	want := teachers.Teacher{
		ID: "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa", Names: "Carlos", Surnames: "Mendoza Ruiz",
		DocumentID: "79888777", Phone: "3205556677", Address: "Carrera 15 # 30-10",
		Birthplace: "Medellín", Birthdate: time.Date(1985, 6, 20, 0, 0, 0, 0, time.UTC),
		Email: "carlos.mendoza@example.com", MedicalConditions: "Hipertensión controlada",
		HomeroomClassID: "9-1",
	}
	got, err := store.Create(ctx, want)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Create = %+v, want %+v", got, want)
	}
	byID, err := store.ByID(ctx, want.ID)
	if err != nil {
		t.Fatalf("ByID: %v", err)
	}
	if !reflect.DeepEqual(byID, want) {
		t.Errorf("ByID = %+v, want %+v", byID, want)
	}

	// Update exercises the full-column UPDATE path.
	byID.HomeroomClassID = ""
	upd, err := store.Update(ctx, byID)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if upd.IsHomeroomDirector() {
		t.Errorf("Update = %+v, want no homeroom", upd)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List = %+v, want 1 teacher", list)
	}

	if _, err := store.ByID(ctx, "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb"); !errors.Is(err, teachers.ErrNotFound) {
		t.Errorf("ByID unknown error = %v, want ErrNotFound", err)
	}
}

func TestSubjectsTimelineRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	teachersStore := pg.NewTeachersStore(db)
	tid := "cccccccc-3333-4333-8333-cccccccccccc"
	if _, err := teachersStore.Create(ctx, teachers.Teacher{
		ID: tid, Names: "Ana", Surnames: "Ríos", DocumentID: "51999111",
	}); err != nil {
		t.Fatalf("Create teacher: %v", err)
	}

	store := pg.NewSubjectsStore(db)
	math, err := store.CreateSubject(ctx, subjects.Subject{
		ID: "dddddddd-4444-4444-8444-dddddddddddd", Name: "Matemáticas", Code: "MAT", Active: true,
	})
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}
	physics, err := store.CreateSubject(ctx, subjects.Subject{
		ID: "eeeeeeee-5555-4555-8555-eeeeeeeeeeee", Name: "Física", Active: true,
	})
	if err != nil {
		t.Fatalf("CreateSubject: %v", err)
	}

	a, err := store.AddAssignment(ctx, subjects.Assignment{
		ID: "ffffffff-6666-4666-8666-ffffffffffff", TeacherID: tid, SubjectID: math.ID,
		StartedAt: time.Date(2021, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("AddAssignment: %v", err)
	}
	if !a.IsOpen() {
		t.Error("new assignment should be open")
	}

	ended, err := store.EndAssignment(ctx, a.ID, time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("EndAssignment: %v", err)
	}
	if ended.IsOpen() {
		t.Error("ended assignment should be closed")
	}

	if _, err := store.AddAssignment(ctx, subjects.Assignment{
		ID: "99999999-7777-4777-8777-999999999999", TeacherID: tid, SubjectID: physics.ID,
		StartedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("AddAssignment physics: %v", err)
	}

	timeline, err := store.AssignmentsForTeacher(ctx, tid)
	if err != nil {
		t.Fatalf("AssignmentsForTeacher: %v", err)
	}
	if len(timeline) != 2 || timeline[0].SubjectID != math.ID || timeline[1].SubjectID != physics.ID {
		t.Errorf("timeline = %+v, want math then physics", timeline)
	}
	forSubj, err := store.AssignmentsForSubject(ctx, math.ID)
	if err != nil || len(forSubj) != 1 {
		t.Fatalf("AssignmentsForSubject = %v, %d records", err, len(forSubj))
	}

	// Deleting the teacher cascades its assignments.
	if err := teachersStore.Delete(ctx, tid); err != nil {
		t.Fatalf("Delete teacher: %v", err)
	}
	timeline, _ = store.AssignmentsForTeacher(ctx, tid)
	if len(timeline) != 0 {
		t.Errorf("assignments not cascaded: %d records remain", len(timeline))
	}
	// Subjects survive: only the timeline is gone.
	if _, err := store.SubjectByID(ctx, math.ID); err != nil {
		t.Errorf("SubjectByID after teacher delete: %v", err)
	}
}

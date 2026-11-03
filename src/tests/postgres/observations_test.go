package postgres_test

import (
	"context"
	"testing"
	"time"

	pg "grade/src/platform/postgres"
	"grade/src/core/observations"
)

func TestObservationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	studentsStore := pg.NewStudentsStore(db)
	st, err := studentsStore.Create(ctx, fullProfile("99999999-9999-4999-8999-999999999999"))
	if err != nil {
		t.Fatalf("Create student: %v", err)
	}
	store := pg.NewObservationsStore(db)
	created, err := store.Add(ctx, observations.Observation{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", StudentID: st.ID,
		StudentName: "Ana Gómez", TeacherID: "teacher-1",
		ClassGroupID: "group-1", ClassLabel: "9-1",
		SituationID: "s3-02", Kind: observations.KindThree, Note: "nota",
		Date: time.Date(2026, 5, 4, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if created.Kind != observations.KindThree || created.StudentName != "Ana Gómez" || created.ClassLabel != "9-1" {
		t.Errorf("roundtrip = %+v, want kind 3 with snapshots", created)
	}
	mine, err := store.ForTeacher(ctx, "teacher-1")
	if err != nil {
		t.Fatalf("ForTeacher: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != created.ID {
		t.Errorf("ForTeacher = %+v, want the row", mine)
	}
}

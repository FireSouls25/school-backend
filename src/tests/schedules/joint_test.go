package schedules_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/schedules"
)

func TestCreateJointWorkshop(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	// El ancla existe primero como bloque normal.
	anchor, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00"))
	if err != nil {
		t.Fatalf("Create anchor: %v", err)
	}
	if anchor.JointID != "" {
		t.Errorf("plain entry JointID = %q, want empty", anchor.JointID)
	}

	// Crear con JointID directo está prohibido: solo el flujo conjunto.
	if _, err := svc.Create(ctx, slot(groupB, teacherA, math, time.Monday, "07:00", "08:00")); !errors.Is(err, schedules.ErrTeacherConflict) {
		t.Fatalf("overlap without joint error = %v, want ErrTeacherConflict", err)
	}

	// Enlazar con el ancla crea el taller: misma llave en ambos.
	joined, err := svc.CreateJoint(ctx,
		slot(groupB, teacherA, math, time.Monday, "07:00", "08:00"), []string{anchor.ID})
	if err != nil {
		t.Fatalf("CreateJoint: %v", err)
	}
	if joined.JointID == "" {
		t.Fatal("CreateJoint JointID empty, want shared key")
	}
	updated, err := svc.ByID(ctx, anchor.ID)
	if err != nil {
		t.Fatalf("ByID anchor: %v", err)
	}
	if updated.JointID != joined.JointID {
		t.Errorf("anchor JointID = %q, want %q", updated.JointID, joined.JointID)
	}

	// El par conjunto ya no choca entre sí (update pasa).
	if _, err := svc.Update(ctx, updated); err != nil {
		t.Errorf("Update joint sibling = %v, want nil", err)
	}

	// Tercer salón se une a la misma llave.
	third, err := svc.CreateJoint(ctx,
		slot("77777777-7777-7777-7777-777777777777", teacherA, math, time.Monday, "07:00", "08:00"), []string{anchor.ID, joined.ID})
	if err != nil {
		t.Fatalf("CreateJoint third: %v", err)
	}
	if third.JointID != joined.JointID {
		t.Errorf("third JointID = %q, want %q", third.JointID, joined.JointID)
	}

	// Sin anclas no hay taller.
	if _, err := svc.CreateJoint(ctx, slot(groupB, teacherA, math, time.Tuesday, "07:00", "08:00"), nil); !errors.Is(err, schedules.ErrInvalidJoint) {
		t.Errorf("empty join error = %v, want ErrInvalidJoint", err)
	}
	// Otro docente no entra.
	both := schedules.NewService(schedules.NewMemoryStore(), fakeChecker{teaches: map[string]bool{
		teacherA + "/" + math: true, teacherB + "/" + math: true,
	}})
	anchorB, err := both.Create(ctx, slot(groupB, teacherB, math, time.Monday, "07:00", "08:00"))
	if err != nil {
		t.Fatalf("Create anchorB: %v", err)
	}
	if _, err := both.CreateJoint(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00"), []string{anchorB.ID}); !errors.Is(err, schedules.ErrInvalidJoint) {
		t.Errorf("foreign teacher error = %v, want ErrInvalidJoint", err)
	}
	// Sin traslape no hay taller.
	if _, err := svc.CreateJoint(ctx, slot(groupB, teacherA, math, time.Monday, "08:00", "09:00"), []string{anchor.ID}); !errors.Is(err, schedules.ErrInvalidJoint) {
		t.Errorf("no-overlap error = %v, want ErrInvalidJoint", err)
	}
	// ID inexistente.
	if _, err := svc.CreateJoint(ctx, slot(groupB, teacherA, math, time.Monday, "07:00", "08:00"), []string{"00000000-0000-0000-0000-000000000000"}); !errors.Is(err, schedules.ErrNotFound) {
		t.Errorf("unknown id error = %v, want ErrNotFound", err)
	}
}

func TestConflictsProbe(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Create(ctx, slot(groupA, teacherA, math, time.Monday, "07:00", "08:00")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	found, err := svc.Conflicts(ctx, slot(groupB, teacherA, math, time.Monday, "07:30", "08:30"))
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(found) != 1 || found[0].ClassGroupID != groupA {
		t.Errorf("Conflicts = %+v, want the overlapping entry", found)
	}
	free, err := svc.Conflicts(ctx, slot(groupB, teacherA, math, time.Monday, "08:00", "09:00"))
	if err != nil {
		t.Fatalf("Conflicts: %v", err)
	}
	if len(free) != 0 {
		t.Errorf("Conflicts = %+v, want empty", free)
	}
}

package observations_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/observations"
)

func TestCatalogShape(t *testing.T) {
	cat := observations.Catalog()
	if len(cat) != 65 {
		t.Fatalf("Catalog len = %d, want 65", len(cat))
	}
	counts := map[observations.Kind]int{}
	seen := map[string]bool{}
	for _, s := range cat {
		counts[s.Kind]++
		if seen[s.ID] {
			t.Errorf("duplicate id %q", s.ID)
		}
		seen[s.ID] = true
	}
	if counts[observations.KindOne] != 14 || counts[observations.KindTwo] != 43 || counts[observations.KindThree] != 8 {
		t.Errorf("kind counts = %v, want 14/43/8", counts)
	}
	if k, ok := observations.KindOf("s2-01"); !ok || k != observations.KindTwo {
		t.Errorf("KindOf(s2-01) = %v, %v, want kind 2 true", k, ok)
	}
	if _, ok := observations.KindOf("s9-99"); ok {
		t.Error("KindOf(s9-99) ok, want false")
	}
}

func TestRecordValidation(t *testing.T) {
	ctx := context.Background()
	svc := observations.NewService(observations.NewMemoryStore())

	got, err := svc.Record(ctx, observations.Record{
		StudentID: "st-1", StudentName: "Ana Gómez", TeacherID: "t-1",
		ClassGroupID: "g-1", ClassLabel: "9-1", SituationID: "s1-03", Note: "hablaba",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if got.Kind != observations.KindOne {
		t.Errorf("Kind = %v, want 1 (server-decided)", got.Kind)
	}
	if got.StudentName != "Ana Gómez" || got.ClassLabel != "9-1" {
		t.Errorf("snapshots = %q/%q, want frozen names", got.StudentName, got.ClassLabel)
	}

	// Unknown situation: rejected, kind never rides along.
	if _, err := svc.Record(ctx, observations.Record{
		StudentID: "st-1", TeacherID: "t-1", SituationID: "s9-99",
	}); !errors.Is(err, observations.ErrInvalidSituation) {
		t.Errorf("bad situation error = %v, want ErrInvalidSituation", err)
	}
	// Future day: rejected.
	if _, err := svc.Record(ctx, observations.Record{
		StudentID: "st-1", TeacherID: "t-1", SituationID: "s2-01",
		Date: time.Now().UTC().AddDate(0, 0, 2),
	}); !errors.Is(err, observations.ErrInvalidDate) {
		t.Errorf("future error = %v, want ErrInvalidDate", err)
	}
	// Empty student: rejected.
	if _, err := svc.Record(ctx, observations.Record{
		TeacherID: "t-1", SituationID: "s3-01",
	}); !errors.Is(err, observations.ErrInvalidStudent) {
		t.Errorf("empty student error = %v, want ErrInvalidStudent", err)
	}

	mine, err := svc.Mine(ctx, "t-1")
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	if len(mine) != 1 || mine[0].ID != got.ID {
		t.Errorf("Mine = %d rows, want the recorded one", len(mine))
	}
}

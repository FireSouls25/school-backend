package postgres_test

import (
	"context"
	"testing"

	pg "grade/src/platform/postgres"
	"grade/src/core/notify"
)

func TestNotifyOutboxRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := testDB(t)

	store := pg.NewNotifyStore(db)
	ok, err := store.Enqueue(ctx, notify.Notification{
		ID: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", To: "+573001234567",
		Kind: notify.KindAbsence, Template: "falta_hoy", Language: "es",
		Params: []string{"Ana", "03/11/2026"}, DedupKey: "st|2026-11-03|absence",
	})
	if err != nil || !ok {
		t.Fatalf("Enqueue = %v, %v; want true, nil", ok, err)
	}
	dup, err := store.Enqueue(ctx, notify.Notification{
		ID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", To: "+573001234567",
		Kind: notify.KindAbsence, Template: "falta_hoy", DedupKey: "st|2026-11-03|absence",
	})
	if err != nil || dup {
		t.Fatalf("dup Enqueue = %v, %v; want false, nil", dup, err)
	}
	rows, err := store.Claim(ctx, 25, 3)
	if err != nil || len(rows) != 1 {
		t.Fatalf("Claim = %d rows, %v; want 1, nil", len(rows), err)
	}
	if rows[0].Params[0] != "Ana" || rows[0].Language != "es" {
		t.Errorf("row = %+v, want params round-tripped", rows[0])
	}
	if err := store.MarkSent(ctx, rows[0].ID); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}
	rows, err = store.Claim(ctx, 25, 3)
	if err != nil || len(rows) != 0 {
		t.Fatalf("Claim after send = %d rows, %v; want 0, nil", len(rows), err)
	}
	if err := store.MarkFailed(ctx, "00000000-0000-4000-8000-000000000000", 3); err == nil {
		t.Error("MarkFailed unknown ok, want error")
	}
}

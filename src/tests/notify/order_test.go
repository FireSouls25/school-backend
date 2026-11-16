package notify_test

import (
	"context"
	"testing"

	"grade/src/core/notify"
)

// TestPendingKeepsQueueOrder proves that notices queued within the same
// coarse clock tick still come out in the order they were enqueued. On
// Windows the wall clock can collapse several writes into one instant,
// and a pure time sort would list them in random order, which once made
// the roll-call notify test see the wrong "last" template.
func TestPendingKeepsQueueOrder(t *testing.T) {
	store := notify.NewMemoryStore()
	svc := notify.NewService(store)
	ctx := context.Background()
	want := []string{"falta_hoy", "atraso_hoy", "evasion_hoy"}
	for i, tpl := range want {
		inserted, err := svc.Notify(ctx, notify.Draft{
			To:       "3001234567",
			Kind:     notify.KindAbsence,
			Template: tpl,
			DedupKey: "st|2026-11-03|" + tpl,
		})
		if err != nil || !inserted {
			t.Fatalf("notify %d (%s) = %v, %v; want true, nil", i, tpl, inserted, err)
		}
	}

	rows := store.Pending()
	if len(rows) != len(want) {
		t.Fatalf("pending = %d rows, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		if row.Template != want[i] {
			t.Errorf("row %d template = %q, want %q", i, row.Template, want[i])
		}
	}
}

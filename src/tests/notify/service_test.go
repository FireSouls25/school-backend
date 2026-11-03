package notify_test

import (
	"context"
	"errors"
	"testing"

	"grade/src/core/notify"
)

func TestNormalizePhone(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"3001234567", "+573001234567", true},
		{"+57 300 123 4567", "+573001234567", true},
		{"573001234567", "+573001234567", true},
		{"6012345678", "", false}, // landline: no sirve para WhatsApp
		{"123", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := notify.NormalizePhone(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("NormalizePhone(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestEnqueueDedups(t *testing.T) {
	ctx := context.Background()
	svc := notify.NewService(notify.NewMemoryStore())

	first, err := svc.Notify(ctx, notify.Draft{To: "3001234567", Kind: notify.KindAbsence, Template: "falta_hoy", Params: []string{"Ana"}, DedupKey: "st|2026-11-03|absence"})
	if err != nil || !first {
		t.Fatalf("first Notify = %v, %v; want true, nil", first, err)
	}
	second, err := svc.Notify(ctx, notify.Draft{To: "3001234567", Kind: notify.KindAbsence, Template: "falta_hoy", Params: []string{"Ana"}, DedupKey: "st|2026-11-03|absence"})
	if err != nil || second {
		t.Fatalf("double Notify = %v, %v; want false (dedup), nil", second, err)
	}
	if _, err := svc.Notify(ctx, notify.Draft{To: "nope", Kind: notify.KindAbsence, Template: "falta_hoy"}); !errors.Is(err, notify.ErrInvalidRecipient) {
		t.Errorf("bad phone error = %v, want ErrInvalidRecipient", err)
	}
	if _, err := svc.Notify(ctx, notify.Draft{To: "3001234567", Kind: notify.KindAbsence}); !errors.Is(err, notify.ErrInvalidTemplate) {
		t.Errorf("empty template error = %v, want ErrInvalidTemplate", err)
	}
}

type scriptedSender struct {
	fail error
	sent *[]notify.Notification
}

func (s scriptedSender) Send(_ context.Context, n notify.Notification) error {
	if s.fail != nil {
		return s.fail
	}
	*s.sent = append(*s.sent, n)
	return nil
}

func TestFlushRetriesThenFails(t *testing.T) {
	ctx := context.Background()
	svc := notify.NewService(notify.NewMemoryStore())
	if _, err := svc.Notify(ctx, notify.Draft{To: "3001234567", Kind: notify.KindLate, Template: "atraso_hoy", DedupKey: "k1"}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	var sent []notify.Notification
	if _, _, err := svc.Flush(ctx, scriptedSender{fail: errors.New("down")}, 25, 2); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	// Still pending after the first failure: one attempt recorded.
	if _, _, err := svc.Flush(ctx, scriptedSender{sent: &sent}, 25, 2); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("sent = %d, want the retried row", len(sent))
	}
	if _, _, err := svc.Flush(ctx, scriptedSender{sent: &sent}, 25, 2); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if len(sent) != 1 {
		t.Errorf("sent = %d, want no double delivery", len(sent))
	}
}

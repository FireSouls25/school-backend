package throttle_test

import (
	"context"
	"testing"
	"time"

	"grade/src/core/throttle"
)

func TestAllowCapsWindow(t *testing.T) {
	svc := throttle.NewService(throttle.NewMemoryStore())
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 10; i++ {
		if !svc.AllowSimple(ctx, "admin|1.2.3.4", 10, time.Minute, now) {
			t.Fatalf("attempt %d denied, want allow", i)
		}
	}
	if svc.AllowSimple(ctx, "admin|1.2.3.4", 10, time.Minute, now) {
		t.Fatal("11th attempt allowed, want deny")
	}
	// Denied attempts do not extend the window: after the window the
	// budget is whole again.
	if !svc.AllowSimple(ctx, "admin|1.2.3.4", 10, time.Minute, now.Add(2*time.Minute)) {
		t.Fatal("post-window attempt denied, want allow")
	}
}

func TestCheckDoesNotRecord(t *testing.T) {
	svc := throttle.NewService(throttle.NewMemoryStore())
	ctx := context.Background()
	now := time.Now()

	for i := 0; i < 10; i++ {
		if !svc.Check(ctx, "k", 10, time.Minute, now) {
			t.Fatalf("check %d denied, want allow", i)
		}
	}
	// Checks alone never fill the budget.
	if !svc.Check(ctx, "k", 10, time.Minute, now) {
		t.Fatal("check-only budget exhausted, want allow")
	}
}

func TestClientIPTrustsOnlyProxies(t *testing.T) {
	trusted, err := throttle.ParseTrusted("10.0.0.1, 192.168.0.0/16")
	if err != nil {
		t.Fatalf("ParseTrusted: %v", err)
	}
	if got := throttle.ClientIP("10.0.0.1:1234", "203.0.113.9", trusted); got != "203.0.113.9" {
		t.Errorf("trusted proxy ip = %q, want forwarded 203.0.113.9", got)
	}
	if got := throttle.ClientIP("203.0.113.5:1234", "10.9.9.9", trusted); got != "203.0.113.5" {
		t.Errorf("untrusted ip = %q, want RemoteAddr host", got)
	}
	if got := throttle.ClientIP("1.2.3.4:5678", "", nil); got != "1.2.3.4" {
		t.Errorf("nil trusted ip = %q, want 1.2.3.4", got)
	}
	if _, err := throttle.ParseTrusted("no-es-ip"); err == nil {
		t.Error("invalid proxy accepted, want error")
	}
}

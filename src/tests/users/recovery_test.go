package users_test

import (
	"context"
	"testing"
	"time"

	"grade/src/core/users"
)

func newRecovery(t *testing.T) (*users.Service, *users.RecoveryService) {
	t.Helper()
	svc := newService()
	rec := users.NewRecoveryService(svc, users.NewMemoryRecoveryStore(), time.Hour)
	return svc, rec
}

func seedActive(t *testing.T, svc *users.Service) {
	t.Helper()
	ctx := context.Background()
	if _, err := svc.Create(ctx, "admin", "admin@colegio.edu", "admin123*"); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestRecoveryRequestHidesUnknown(t *testing.T) {
	_, rec := newRecovery(t)
	ctx := context.Background()

	if tok, err := rec.Request(ctx, "nadie"); err != nil || tok != "" {
		t.Fatalf("unknown request = %q, %v; want empty, nil", tok, err)
	}
}

func TestRecoveryRoundTrip(t *testing.T) {
	svc, rec := newRecovery(t)
	ctx := context.Background()
	seedActive(t, svc)

	token, err := rec.Request(ctx, "admin")
	if err != nil || token == "" {
		t.Fatalf("request = %q, %v; want token", token, err)
	}
	admin, _ := svc.ByUsername(ctx, "admin")
	before := admin.TokenVersion
	if _, err := rec.Reset(ctx, token, "nuevaClave99"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	admin, _ = svc.ByUsername(ctx, "admin")
	if admin.TokenVersion == before {
		t.Error("version unchanged after reset, want bump (bearers die)")
	}
	if _, err := svc.Authenticate(ctx, "admin", "nuevaClave99"); err != nil {
		t.Fatalf("new password rejected: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "admin", "admin123*"); err == nil {
		t.Fatal("old password still authenticates")
	}
}

func TestRecoveryReuseAndWeakRejected(t *testing.T) {
	svc, rec := newRecovery(t)
	ctx := context.Background()
	seedActive(t, svc)

	token, err := rec.Request(ctx, "admin@colegio.edu")
	if err != nil || token == "" {
		t.Fatalf("request = %q, %v", token, err)
	}
	if _, err := rec.Reset(ctx, token, "corta1"); err != users.ErrWeakPassword {
		t.Fatalf("weak reset error = %v, want ErrWeakPassword", err)
	}
	if _, err := rec.Reset(ctx, token, "nuevaClave99"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := rec.Reset(ctx, token, "otraClave99"); err != users.ErrInvalidRecoveryToken {
		t.Fatalf("reuse error = %v, want ErrInvalidRecoveryToken", err)
	}
	if _, err := rec.Reset(ctx, "00"+token, "otraClave99"); err != users.ErrInvalidRecoveryToken {
		t.Fatalf("forged error = %v, want ErrInvalidRecoveryToken", err)
	}
}

func TestRecoveryExpiredRejected(t *testing.T) {
	svc, _ := newRecovery(t)
	ctx := context.Background()
	seedActive(t, svc)

	fast := users.NewRecoveryService(svc, users.NewMemoryRecoveryStore(), -time.Second)
	_ = fast
	// Expiry is simulated with a nanosecond-lived service.
	short := users.NewRecoveryService(svc, users.NewMemoryRecoveryStore(), time.Nanosecond)
	tok2, err := short.Request(ctx, "admin")
	if err != nil || tok2 == "" {
		t.Fatalf("request = %q, %v", tok2, err)
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := short.Reset(ctx, tok2, "nuevaClave99"); err != users.ErrInvalidRecoveryToken {
		t.Fatalf("expired error = %v, want ErrInvalidRecoveryToken", err)
	}
}

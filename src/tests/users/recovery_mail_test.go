package users_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"grade/src/core/users"
)

// fakeMailer records reset links without touching the network.
type fakeMailer struct {
	to   []string
	link []string
}

func (f *fakeMailer) SendResetLink(_ context.Context, to, link string) error {
	f.to = append(f.to, to)
	f.link = append(f.link, link)
	return nil
}

func TestRecoveryDeliversLinkWhenMailerWired(t *testing.T) {
	svc, _ := newRecovery(t)
	ctx := context.Background()
	seedActive(t, svc)

	m := &fakeMailer{}
	rec := users.NewRecoveryService(svc, users.NewMemoryRecoveryStore(), time.Hour, m, "https://fractalschool.app")
	tok, err := rec.Request(ctx, "admin")
	if err != nil || tok == "" {
		t.Fatalf("request = %q, %v", tok, err)
	}
	if len(m.link) != 1 {
		t.Fatalf("mails = %d, want 1", len(m.link))
	}
	if m.to[0] != "admin@colegio.edu" {
		t.Errorf("to = %q, want account email", m.to[0])
	}
	want := "https://fractalschool.app/recuperar/" + tok
	if m.link[0] != want {
		t.Errorf("link = %q, want %q", m.link[0], want)
	}
	if strings.Contains(m.link[0], " ") {
		t.Errorf("link contains spaces: %q", m.link[0])
	}
}

func TestRecoverySilentWithoutMailer(t *testing.T) {
	svc, rec := newRecovery(t)
	ctx := context.Background()
	seedActive(t, svc)

	if tok, err := rec.Request(ctx, "admin"); err != nil || tok == "" {
		t.Fatalf("request = %q, %v", tok, err)
	}
	// Unknown accounts still answer empty without touching any mailer.
	if tok, err := rec.Request(ctx, "nadie"); err != nil || tok != "" {
		t.Fatalf("unknown request = %q, %v", tok, err)
	}
}

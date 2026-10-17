package auth_test

import (
	"errors"
	"testing"
	"time"

	"grade/src/core/auth"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	svc, err := auth.NewService("test-secret-for-suite-only", time.Hour)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestMintVerifyRoundTrip(t *testing.T) {
	svc := newService(t)
	now := time.Now()

	token, expiresAt, err := svc.Mint("subject-1", []string{"admin", "teacher"}, now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if token == "" || !expiresAt.After(now) {
		t.Fatalf("mint = %q, %v; want token and future expiry", token, expiresAt)
	}

	claims, err := svc.Verify(token, now)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Subject != "subject-1" || len(claims.Roles) != 2 {
		t.Errorf("claims = %+v, want subject and roles", claims)
	}
}

func TestVerifyRejects(t *testing.T) {
	svc := newService(t)
	now := time.Now()

	token, _, err := svc.Mint("subject-1", nil, now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	t.Run("tampered payload", func(t *testing.T) {
		if _, err := svc.Verify(token[:len(token)-4]+"AAAA", now); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("error = %v, want ErrInvalidToken", err)
		}
	})
	t.Run("wrong secret", func(t *testing.T) {
		other, _ := auth.NewService("another-secret", time.Hour)
		if _, err := other.Verify(token, now); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("error = %v, want ErrInvalidToken", err)
		}
	})
	t.Run("expired", func(t *testing.T) {
		if _, err := svc.Verify(token, now.Add(2*time.Hour)); !errors.Is(err, auth.ErrExpiredToken) {
			t.Errorf("error = %v, want ErrExpiredToken", err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		for _, bad := range []string{"", "a.b", "a.b.c.d"} {
			if _, err := svc.Verify(bad, now); !errors.Is(err, auth.ErrInvalidToken) {
				t.Errorf("Verify(%q) error = %v, want ErrInvalidToken", bad, err)
			}
		}
	})
}

func TestNewServiceRejectsWeakConfig(t *testing.T) {
	if _, err := auth.NewService("", time.Hour); !errors.Is(err, auth.ErrInvalidSecret) {
		t.Errorf("empty secret error = %v, want ErrInvalidSecret", err)
	}
	if _, err := auth.NewService("s", 0); !errors.Is(err, auth.ErrInvalidTTL) {
		t.Errorf("zero ttl error = %v, want ErrInvalidTTL", err)
	}
	if _, _, err := newService(t).Mint("  ", nil, time.Now()); !errors.Is(err, auth.ErrInvalidSubject) {
		t.Errorf("blank subject error = %v, want ErrInvalidSubject", err)
	}
}

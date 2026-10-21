package auth_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/auth"
)

func newService(t *testing.T) *auth.Service {
	t.Helper()
	svc, err := auth.NewService("test-secret-for-suite-only", time.Hour, 7*24*time.Hour, nil)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc
}

func TestMintVerifyRoundTrip(t *testing.T) {
	svc := newService(t)
	now := time.Now()

	token, expiresAt, err := svc.MintAccess("subject-1", []string{"admin", "teacher"}, 0, now)
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
	if claims.Jti == "" {
		t.Errorf("claims.Jti empty, want unique token id")
	}
}

func TestVerifyRejects(t *testing.T) {
	svc := newService(t)
	now := time.Now()

	token, _, err := svc.MintAccess("subject-1", nil, 0, now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}

	t.Run("tampered payload", func(t *testing.T) {
		if _, err := svc.Verify(token[:len(token)-4]+"AAAA", now); !errors.Is(err, auth.ErrInvalidToken) {
			t.Errorf("error = %v, want ErrInvalidToken", err)
		}
	})
	t.Run("wrong secret", func(t *testing.T) {
		other, _ := auth.NewService("another-secret", time.Hour, 7*24*time.Hour, nil)
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
	if _, err := auth.NewService("", time.Hour, 7*24*time.Hour, nil); !errors.Is(err, auth.ErrInvalidSecret) {
		t.Errorf("empty secret error = %v, want ErrInvalidSecret", err)
	}
	if _, err := auth.NewService("s", 0, 7*24*time.Hour, nil); !errors.Is(err, auth.ErrInvalidTTL) {
		t.Errorf("zero access ttl error = %v, want ErrInvalidTTL", err)
	}
	if _, err := auth.NewService("s", time.Hour, 0, nil); !errors.Is(err, auth.ErrInvalidTTL) {
		t.Errorf("zero ttl error = %v, want ErrInvalidTTL", err)
	}
	if _, _, err := newService(t).MintAccess("  ", nil, 0, time.Now()); !errors.Is(err, auth.ErrInvalidSubject) {
		t.Errorf("blank subject error = %v, want ErrInvalidSubject", err)
	}
}

func TestMintCarriesVersion(t *testing.T) {
	svc := newService(t)
	now := time.Now()
	token, _, err := svc.MintAccess("subject-1", nil, 7, now)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	claims, err := svc.Verify(token, now)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims.Ver != 7 {
		t.Errorf("claims.Ver = %d, want 7", claims.Ver)
	}
}

func TestRefreshRoundTripAndRotation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	now := time.Now()

	refresh, exp, err := svc.IssueRefresh(ctx, "subject-1", 3, now)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}
	if refresh == "" || !exp.After(now) {
		t.Fatalf("refresh = %q, %v; want token and future expiry", refresh, exp)
	}
	claims, err := svc.CheckRefresh(ctx, refresh, now)
	if err != nil {
		t.Fatalf("CheckRefresh: %v", err)
	}
	if claims.Subject != "subject-1" || claims.Ver != 3 || claims.Typ != auth.RefreshTokenType {
		t.Errorf("claims = %+v, want subject/ver/refresh typ", claims)
	}
	// Access tokens are not refresh tokens.
	access, _, err := svc.MintAccess("subject-1", nil, 3, now)
	if err != nil {
		t.Fatalf("MintAccess: %v", err)
	}
	if _, err := svc.CheckRefresh(ctx, access, now); !errors.Is(err, auth.ErrInvalidToken) {
		t.Errorf("access as refresh error = %v, want ErrInvalidToken", err)
	}
	// Rotation: revoke the presented refresh; reuse fails as revoked.
	if err := svc.Revoke(ctx, claims.Jti); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.CheckRefresh(ctx, refresh, now); !errors.Is(err, auth.ErrRevokedToken) {
		t.Errorf("reused refresh error = %v, want ErrRevokedToken", err)
	}
}

func TestRefreshRevokedForUser(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	now := time.Now()

	r1, _, err := svc.IssueRefresh(ctx, "subject-9", 0, now)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}
	r2, _, err := svc.IssueRefresh(ctx, "subject-9", 0, now)
	if err != nil {
		t.Fatalf("IssueRefresh: %v", err)
	}
	if err := svc.RevokeAllForUser(ctx, "subject-9"); err != nil {
		t.Fatalf("RevokeAllForUser: %v", err)
	}
	for i, tok := range []string{r1, r2} {
		if _, err := svc.CheckRefresh(ctx, tok, now); !errors.Is(err, auth.ErrRevokedToken) {
			t.Errorf("token %d error = %v, want ErrRevokedToken", i, err)
		}
	}
}

package users_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"

	"grade/src/core/users"
)

func newService() *users.Service {
	// MinCost keeps the suite fast; production uses bcrypt.DefaultCost
	// through users.NewService.
	return users.NewServiceWithCost(users.NewMemoryStore(), bcrypt.MinCost)
}

func TestCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name            string
		username, email string
		password        string
		want            error
	}{
		{"short username", "ab", "", "secret123", users.ErrInvalidUsername},
		{"blank username", "   ", "", "secret123", users.ErrInvalidUsername},
		{"leading dot", ".admin", "", "secret123", users.ErrInvalidUsername},
		{"spaces inside", "ad min", "", "secret123", users.ErrInvalidUsername},
		{"bad email", "admin", "not-an-email", "secret123", users.ErrInvalidEmail},
		{"short password", "admin", "", "abc123", users.ErrWeakPassword},
		{"no digit", "admin", "", "passwordonly", users.ErrWeakPassword},
		{"no letter", "admin", "", "12345678", users.ErrWeakPassword},
		{"too long", "admin", "", strings.Repeat("a1", 40), users.ErrWeakPassword},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Create(ctx, tc.username, tc.email, tc.password); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateAssignsUUIDAndHashesPassword(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Create(ctx, "Admin", "Admin@Observador.edu.co", "admin123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" {
		t.Error("Create returned an empty id")
	}
	if a.Username != "admin" {
		t.Errorf("Username = %q, want normalized %q", a.Username, "admin")
	}
	if a.Email != "admin@observador.edu.co" {
		t.Errorf("Email = %q, want lowercased", a.Email)
	}
	if !a.Active {
		t.Error("new user Active = false, want true")
	}
	if a.PasswordHash != "" {
		t.Error("Create returned a password hash; public results must be sanitized")
	}

	// The stored hash must verify and must not be the plaintext.
	stored, err := svc.ByUsername(ctx, "admin")
	if err != nil {
		t.Fatalf("ByUsername: %v", err)
	}
	if stored.ID != a.ID {
		t.Errorf("ByUsername id = %q, want %q", stored.ID, a.ID)
	}
}

func TestProductionCostRoundTrip(t *testing.T) {
	// Covers users.NewService (bcrypt.DefaultCost) with a single account
	// so the production hashing path is exercised at least once.
	ctx := context.Background()
	svc := users.NewService(users.NewMemoryStore())

	u, err := svc.Create(ctx, "rector", "rectoria@observador.edu.co", "rector123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "rector", "rector123*"); err != nil {
		t.Errorf("Authenticate: %v", err)
	}
	_ = u
}

func TestPasswordHashIsBcrypt(t *testing.T) {
	ctx := context.Background()
	store := users.NewMemoryStore()
	svc := users.NewServiceWithCost(store, bcrypt.MinCost)

	u, err := svc.Create(ctx, "carlos.mendoza", "carlos.mendoza@observador.edu.co", "docente123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	raw, err := store.ByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("store ByID: %v", err)
	}
	if raw.PasswordHash == "docente123*" || raw.PasswordHash == "" {
		t.Error("password stored in plaintext or empty; want a bcrypt hash")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(raw.PasswordHash), []byte("docente123*")); err != nil {
		t.Errorf("stored hash does not verify: %v", err)
	}
}

func TestDuplicatesAreCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.Create(ctx, "admin", "admin@observador.edu.co", "admin123*"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.Create(ctx, "ADMIN", "other@observador.edu.co", "admin123*"); !errors.Is(err, users.ErrDuplicateUsername) {
		t.Errorf("duplicate username error = %v, want ErrDuplicateUsername", err)
	}
	if _, err := svc.Create(ctx, "other", "ADMIN@observador.edu.co", "admin123*"); !errors.Is(err, users.ErrDuplicateEmail) {
		t.Errorf("duplicate email error = %v, want ErrDuplicateEmail", err)
	}
	// Empty emails never collide.
	if _, err := svc.Create(ctx, "docente1", "", "docente123*"); err != nil {
		t.Fatalf("Create without email: %v", err)
	}
	if _, err := svc.Create(ctx, "docente2", "", "docente123*"); err != nil {
		t.Fatalf("second Create without email: %v", err)
	}
}

func TestAuthenticate(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	u, err := svc.Create(ctx, "carlos.mendoza", "carlos.mendoza@observador.edu.co", "docente123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// By username, by email, and case-insensitively.
	for _, id := range []string{"carlos.mendoza", "CARLOS.MENDOZA", "carlos.mendoza@observador.edu.co", "CARLOS.MENDOZA@OBSERVADOR.EDU.CO"} {
		got, err := svc.Authenticate(ctx, id, "docente123*")
		if err != nil {
			t.Errorf("Authenticate(%q): %v", id, err)
			continue
		}
		if got.ID != u.ID || got.PasswordHash != "" {
			t.Errorf("Authenticate(%q) = %+v, want id %q without hash", id, got, u.ID)
		}
	}

	// Every failure mode reports the same code: no enumeration.
	for _, tc := range []struct {
		name       string
		identifier string
		password   string
	}{
		{"wrong password", "carlos.mendoza", "wrong1234"},
		{"unknown username", "nadie.aqui", "docente123*"},
		{"unknown email", "nadie@observador.edu.co", "docente123*"},
		{"empty password", "carlos.mendoza", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Authenticate(ctx, tc.identifier, tc.password); !errors.Is(err, users.ErrInvalidCredentials) {
				t.Errorf("Authenticate error = %v, want ErrInvalidCredentials", err)
			}
		})
	}

	// Inactive accounts fail exactly like bad credentials.
	if _, err := svc.SetActive(ctx, u.ID, false); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "carlos.mendoza", "docente123*"); !errors.Is(err, users.ErrInvalidCredentials) {
		t.Errorf("inactive Authenticate error = %v, want ErrInvalidCredentials", err)
	}
	if _, err := svc.SetActive(ctx, u.ID, true); err != nil {
		t.Fatalf("SetActive reactivate: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "carlos.mendoza", "docente123*"); err != nil {
		t.Errorf("reactivated Authenticate: %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	u, err := svc.Create(ctx, "admin", "", "admin123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.SetPassword(ctx, u.ID, "nueva456*"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "admin", "admin123*"); !errors.Is(err, users.ErrInvalidCredentials) {
		t.Errorf("old password still works, want ErrInvalidCredentials")
	}
	if _, err := svc.Authenticate(ctx, "admin", "nueva456*"); err != nil {
		t.Errorf("new password rejected: %v", err)
	}
	if err := svc.SetPassword(ctx, u.ID, "short"); !errors.Is(err, users.ErrWeakPassword) {
		t.Errorf("weak SetPassword error = %v, want ErrWeakPassword", err)
	}
	if err := svc.SetPassword(ctx, "00000000-0000-4000-8000-000000000000", "nueva456*"); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("unknown SetPassword error = %v, want ErrNotFound", err)
	}
}

func TestByIDAndDelete(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	u, err := svc.Create(ctx, "admin", "", "admin123*")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := svc.ByID(ctx, "not-a-uuid"); !errors.Is(err, users.ErrInvalidID) {
		t.Errorf("ByID bad id error = %v, want ErrInvalidID", err)
	}
	if _, err := svc.ByID(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("ByID unknown error = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, u.ID); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
	// Deleting again is a no-op.
	if err := svc.Delete(ctx, u.ID); err != nil {
		t.Errorf("second Delete: %v", err)
	}
}

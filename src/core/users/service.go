package users

import (
	"context"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Password policy. Bcrypt caps passwords at 72 bytes; the minimum and the
// letter-plus-digit rule follow the frontend dev accounts (e.g.
// "admin123*") so existing school passwords keep working.
const (
	// MinPasswordLength is the minimum password length in bytes.
	MinPasswordLength = 8
	// MaxPasswordLength is the maximum password length in bytes (bcrypt limit).
	MaxPasswordLength = 72
)

// Service applies account policy and persists users through a Store.
// Passwords only cross this boundary as plaintext on Create/Authenticate/
// SetPassword; they are stored and compared as bcrypt hashes.
type Service struct {
	store Store
	cost  int
}

// NewService wires a Service onto the provided Store, hashing passwords
// with bcrypt.DefaultCost.
func NewService(store Store) *Service {
	return NewServiceWithCost(store, bcrypt.DefaultCost)
}

// NewServiceWithCost wires a Service hashing passwords with the given
// bcrypt cost (clamped to bcrypt.MinCost..bcrypt.MaxCost). It exists so
// tests run fast with bcrypt.MinCost; production always uses NewService.
func NewServiceWithCost(store Store, cost int) *Service {
	if cost < bcrypt.MinCost {
		cost = bcrypt.MinCost
	}
	if cost > bcrypt.MaxCost {
		cost = bcrypt.MaxCost
	}
	return &Service{store: store, cost: cost}
}

// Create validates the input, hashes the password with bcrypt, assigns a
// fresh UUID and persists the account. New users are always active. The
// returned user carries no password hash.
func (s *Service) Create(ctx context.Context, username, email, password string) (User, error) {
	username = normalizeUsername(username)
	email = normalizeEmail(email)
	if err := validateUsername(username); err != nil {
		return User{}, err
	}
	if err := validateEmail(email); err != nil {
		return User{}, err
	}
	if err := validatePassword(password); err != nil {
		return User{}, err
	}
	if _, err := s.store.ByUsername(ctx, username); err == nil {
		return User{}, ErrDuplicateUsername
	}
	if email != "" {
		if _, err := s.store.ByEmail(ctx, email); err == nil {
			return User{}, ErrDuplicateEmail
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	u, err := s.store.Create(ctx, User{
		ID:           uuid.NewString(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// Authenticate verifies an identifier (username, or email when it contains
// "@") plus password. Every failure — unknown identifier, wrong password
// or inactive account — returns ErrInvalidCredentials so callers cannot
// enumerate which field failed. The returned user carries no password hash.
func (s *Service) Authenticate(ctx context.Context, identifier, password string) (User, error) {
	identifier = strings.TrimSpace(identifier)
	var (
		u   User
		err error
	)
	if strings.Contains(identifier, "@") {
		u, err = s.store.ByEmail(ctx, normalizeEmail(identifier))
	} else {
		u, err = s.store.ByUsername(ctx, normalizeUsername(identifier))
	}
	if err != nil {
		return User{}, ErrInvalidCredentials
	}
	if !u.Active {
		return User{}, ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}
	return u.Sanitized(), nil
}

// ByID returns the user with the given id without its password hash.
func (s *Service) ByID(ctx context.Context, id string) (User, error) {
	if err := validateID(id); err != nil {
		return User{}, err
	}
	u, err := s.store.ByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// ByUsername returns the user with the given username (case-insensitive)
// without its password hash. It exists for admin tooling; login uses
// Authenticate.
func (s *Service) ByUsername(ctx context.Context, username string) (User, error) {
	if err := validateUsername(normalizeUsername(username)); err != nil {
		return User{}, err
	}
	u, err := s.store.ByUsername(ctx, normalizeUsername(username))
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// SetPassword validates the new password, hashes it and replaces the
// stored hash. The old password stops working immediately.
func (s *Service) SetPassword(ctx context.Context, id, newPassword string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	if _, err := s.store.ByID(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cost)
	if err != nil {
		return err
	}
	return s.store.SetPasswordHash(ctx, strings.TrimSpace(id), string(hash))
}

// SetActive enables or disables the login without deleting the account.
// Disabled users fail authentication with ErrInvalidCredentials.
func (s *Service) SetActive(ctx context.Context, id string, active bool) (User, error) {
	if err := validateID(id); err != nil {
		return User{}, err
	}
	u, err := s.store.ByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return User{}, err
	}
	u.Active = active
	u.UpdatedAt = time.Now()
	u, err = s.store.Update(ctx, u)
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// Delete removes the account. Removing an unknown user is a no-op.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, strings.TrimSpace(id))
}

// normalizeUsername trims and lowercases the username so validation and
// storage see a canonical form.
func normalizeUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// normalizeEmail trims and lowercases the email so validation and storage
// see a canonical form.
func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func validateUsername(username string) error {
	if len(username) < 3 || len(username) > 32 {
		return ErrInvalidUsername
	}
	for i, r := range username {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return ErrInvalidUsername
		}
		if i == 0 && !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			return ErrInvalidUsername
		}
	}
	return nil
}

func validateEmail(email string) error {
	if email == "" {
		return nil
	}
	if len(email) > 254 {
		return ErrInvalidEmail
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return ErrInvalidEmail
	}
	return nil
}

// validatePassword enforces the password policy: 8-72 bytes with at
// least one letter and one number.
func validatePassword(password string) error {
	if len(password) < MinPasswordLength || len(password) > MaxPasswordLength {
		return ErrWeakPassword
	}
	var hasLetter, hasNumber bool
	for _, r := range password {
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsNumber(r):
			hasNumber = true
		}
	}
	if !hasLetter || !hasNumber {
		return ErrWeakPassword
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}

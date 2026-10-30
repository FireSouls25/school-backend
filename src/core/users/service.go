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

// DefaultRetentionDays is how long a soft-deleted account survives
// before PurgeDeleted removes it for good. A week gives the school
// time to undo a mistaken deletion.
const DefaultRetentionDays = 7

// Service applies account policy and persists users through a Store.
// Passwords only cross this boundary as plaintext on Create/Authenticate/
// SetPassword; they are stored and compared as bcrypt hashes.
type Service struct {
	store         Store
	cost          int
	retentionDays int
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
	return &Service{store: store, cost: cost, retentionDays: DefaultRetentionDays}
}

// WithRetentionDays sets how many days a soft-deleted account survives
// before PurgeDeleted removes it. Non-positive values keep the default.
// It returns the service so wiring stays fluent.
func (s *Service) WithRetentionDays(days int) *Service {
	if days > 0 {
		s.retentionDays = days
	}
	return s
}

// RetentionDays reports the configured soft-delete retention in days.
func (s *Service) RetentionDays() int {
	if s.retentionDays <= 0 {
		return DefaultRetentionDays
	}
	return s.retentionDays
}

// RetentionCutoff returns the instant before which soft-deleted accounts
// count as expired: anything deleted at or before it is purgable.
func (s *Service) RetentionCutoff(now time.Time) time.Time {
	return now.AddDate(0, 0, -s.RetentionDays())
}

// Create validates the input, hashes the password with bcrypt, assigns a
// fresh UUID and persists the account. New users are always active. The
// returned user carries no password hash.
func (s *Service) Create(ctx context.Context, username, email, password string) (User, error) {
	u, err := buildAccount(uuid.NewString(), username, email, password, s.cost)
	if err != nil {
		return User{}, err
	}
	if err := s.assertFree(ctx, u); err != nil {
		return User{}, err
	}
	stored, err := s.store.Create(ctx, u)
	if err != nil {
		return User{}, err
	}
	return stored.Sanitized(), nil
}

// NewAccount builds an active account with an explicit id, applying the
// same policy and hashing as Create. It does not persist anything.
//
// It exists for seeding and tests, where reproducible ids matter: the
// SPA mock resolves its dev logins to fixed UUIDs, so demo data has to
// keep working those logins. User-facing flows (HTTP, bootstrap) always
// go through Service.Create, which assigns a fresh id.
func NewAccount(id, username, email, password string) (User, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return User{}, ErrInvalidID
	}
	return buildAccount(strings.TrimSpace(id), username, email, password, bcrypt.DefaultCost)
}

// assertFree fails when the username or email is already taken.
func (s *Service) assertFree(ctx context.Context, u User) error {
	if _, err := s.store.ByUsername(ctx, u.Username); err == nil {
		return ErrDuplicateUsername
	}
	if u.Email != "" {
		if _, err := s.store.ByEmail(ctx, u.Email); err == nil {
			return ErrDuplicateEmail
		}
	}
	return nil
}

// buildAccount validates the input and hashes the password.
func buildAccount(id, username, email, password string, cost int) (User, error) {
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
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return User{}, err
	}
	now := time.Now()
	return User{
		ID:           id,
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Active:       true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
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
	if !u.Active || u.DeletedAt != nil {
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
	if err := s.store.SetPasswordHash(ctx, strings.TrimSpace(id), string(hash)); err != nil {
		return err
	}
	if _, err := s.store.BumpTokenVersion(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	return nil
}

// ChangePassword verifies the current password before replacing the
// stored hash, so a stolen bearer cannot silently take over the account.
// A wrong current password answers ErrInvalidCredentials, exactly what
// login answers, and unknown ids fail with ErrNotFound.
func (s *Service) ChangePassword(ctx context.Context, id, currentPassword, newPassword string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validatePassword(newPassword); err != nil {
		return err
	}
	u, err := s.store.ByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), s.cost)
	if err != nil {
		return err
	}
	if err := s.store.SetPasswordHash(ctx, strings.TrimSpace(id), string(hash)); err != nil {
		return err
	}
	if _, err := s.store.BumpTokenVersion(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	return nil
}

// Photo returns the stored account picture: an empty Data when the
// account has none yet, ErrNotFound when the account does not exist.
func (s *Service) Photo(ctx context.Context, id string) (Photo, error) {
	if err := validateID(id); err != nil {
		return Photo{}, err
	}
	if _, err := s.store.ByID(ctx, strings.TrimSpace(id)); err != nil {
		return Photo{}, err
	}
	return s.store.Photo(ctx, strings.TrimSpace(id))
}

// SetPhoto validates the 2 MiB limit, then stores (or clears, when the
// data is empty) the picture of the account with the given id.
func (s *Service) SetPhoto(ctx context.Context, id string, photo Photo) error {
	if err := validateID(id); err != nil {
		return err
	}
	if len(photo.Data) > MaxPhotoSize {
		return ErrPhotoTooLarge
	}
	if _, err := s.store.ByID(ctx, strings.TrimSpace(id)); err != nil {
		return err
	}
	return s.store.SetPhoto(ctx, strings.TrimSpace(id), photo)
}

// SetActive enables or disables the login without deleting the account.// Disabled users fail authentication with ErrInvalidCredentials.
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
	ver, err := s.store.BumpTokenVersion(ctx, strings.TrimSpace(id))
	if err != nil {
		return User{}, err
	}
	u.TokenVersion = ver
	return u.Sanitized(), nil
}

// Delete soft-deletes the account: it stamps DeletedAt and disables
// the login instead of removing the row, so the account stops working
// immediately while its data survives the retention window for a
// possible Restore. Removing an unknown user is a no-op.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if err := s.store.SoftDelete(ctx, id, time.Now()); err != nil {
		return err
	}
	// Unknown users are a no-op: only bump when the row still exists.
	if _, err := s.store.ByID(ctx, id); err != nil {
		return nil
	}
	if _, err := s.store.BumpTokenVersion(ctx, id); err != nil {
		return err
	}
	return nil
}

// Restore brings a soft-deleted account back: it clears DeletedAt and
// re-enables the login. It fails with ErrNotFound for unknown ids,
// ErrNotDeleted when the account was never deleted, and
// ErrRetentionExpired when the retention window already lapsed (only a
// fresh account can replace it then).
func (s *Service) Restore(ctx context.Context, id string) (User, error) {
	if err := validateID(id); err != nil {
		return User{}, err
	}
	id = strings.TrimSpace(id)
	u, err := s.store.ByID(ctx, id)
	if err != nil {
		return User{}, err
	}
	if u.DeletedAt == nil {
		return User{}, ErrNotDeleted
	}
	if !u.DeletedAt.After(s.RetentionCutoff(time.Now())) {
		return User{}, ErrRetentionExpired
	}
	u.DeletedAt = nil
	u.Active = true
	u.UpdatedAt = time.Now()
	u, err = s.store.Update(ctx, u)
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// PurgeDeleted physically removes every account soft-deleted at or
// before the retention cutoff and returns their ids so callers can
// clean related role grants. It is the only path that truly deletes.
func (s *Service) PurgeDeleted(ctx context.Context, now time.Time) ([]string, error) {
	return s.store.PurgeBefore(ctx, s.RetentionCutoff(now))
}

// List returns accounts in store order, without password hashes.
// Deleted accounts are included only when includeDeleted is true.
func (s *Service) List(ctx context.Context, includeDeleted bool) ([]User, error) {
	all, err := s.store.List(ctx, includeDeleted)
	if err != nil {
		return nil, err
	}
	out := make([]User, 0, len(all))
	for _, u := range all {
		out = append(out, u.Sanitized())
	}
	return out, nil
}

// LinkTeacher binds a login to a teacher profile, so schedule and name
// lookups resolve the caller to their teaching self. Re-linking replaces
// the previous link; the teachers capability owns the profile itself.
func (s *Service) LinkTeacher(ctx context.Context, userID, teacherID string) (User, error) {
	if err := validateID(userID); err != nil {
		return User{}, err
	}
	if _, err := uuid.Parse(strings.TrimSpace(teacherID)); err != nil {
		return User{}, ErrInvalidTeacher
	}
	u, err := s.store.ByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return User{}, err
	}
	u.TeacherID = strings.TrimSpace(teacherID)
	u.UpdatedAt = time.Now()
	u, err = s.store.Update(ctx, u)
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// UnlinkTeacher removes the teacher binding, back to a pure account.
func (s *Service) UnlinkTeacher(ctx context.Context, userID string) (User, error) {
	if err := validateID(userID); err != nil {
		return User{}, err
	}
	u, err := s.store.ByID(ctx, strings.TrimSpace(userID))
	if err != nil {
		return User{}, err
	}
	u.TeacherID = ""
	u.UpdatedAt = time.Now()
	u, err = s.store.Update(ctx, u)
	if err != nil {
		return User{}, err
	}
	return u.Sanitized(), nil
}

// ForTeacher returns every login linked to the given teacher profile.
func (s *Service) ForTeacher(ctx context.Context, teacherID string) ([]User, error) {
	if err := validateID(teacherID); err != nil {
		return nil, err
	}
	return s.store.ForTeacher(ctx, strings.TrimSpace(teacherID))
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

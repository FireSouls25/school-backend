package postgres

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"grade/src/core/users"
)

// UsersStore implements users.Store on top of PostgreSQL.
type UsersStore struct {
	db *DB
}

// NewUsersStore returns a users.Store backed by db.
func NewUsersStore(db *DB) *UsersStore { return &UsersStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ users.Store = (*UsersStore)(nil)

// userColumns is the full column list used by every user SELECT.
const userColumns = `id, username, email, password_hash, active, teacher_id, token_version, created_at, updated_at, deleted_at`

// scanUser maps the current row (in userColumns order) onto a User.
func scanUser(scan func(dest ...any) error) (users.User, error) {
	var u users.User
	var teacherID *string
	var created, updated time.Time
	var deletedAt *time.Time
	err := scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Active, &teacherID, &u.TokenVersion, &created, &updated, &deletedAt)
	if err != nil {
		return users.User{}, err
	}
	if teacherID != nil {
		u.TeacherID = *teacherID
	}
	u.CreatedAt = created
	u.UpdatedAt = updated
	u.DeletedAt = deletedAt
	return u, nil
}

// teacherIDArg maps the empty link to a NULL UUID. pgtype is required:
// plain nil encodes as empty string, rejected by UUID columns.
func teacherIDArg(u users.User) any {
	if u.TeacherID == "" {
		return pgtype.UUID{Valid: false}
	}
	return u.TeacherID
}

// userArgs flattens u into the INSERT argument order.
func userArgs(u users.User) []any {
	return []any{u.ID, u.Username, u.Email, u.PasswordHash, u.Active, teacherIDArg(u), u.TokenVersion, u.CreatedAt, u.UpdatedAt, u.DeletedAt}
}

// translateUserWrite maps unique violations on the username/email indexes
// to the coded duplicate errors so services stay decoupled.
func translateUserWrite(err error, what string) error {
	if isUniqueViolationOn(err, "users_username") {
		return users.ErrDuplicateUsername
	}
	if isUniqueViolationOn(err, "users_email") {
		return users.ErrDuplicateEmail
	}
	return fmt.Errorf("postgres: %s user: %w", what, err)
}

// Create implements users.Store.
func (s *UsersStore) Create(ctx context.Context, u users.User) (users.User, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO users (id, username, email, password_hash, active, teacher_id, token_version, created_at, updated_at, deleted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING `+userColumns,
		userArgs(u)...)
	out, err := scanUser(row.Scan)
	if err != nil {
		return users.User{}, translateUserWrite(err, "create")
	}
	return out, nil
}

// ByID implements users.Store.
func (s *UsersStore) ByID(ctx context.Context, id string) (users.User, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	out, err := scanUser(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return users.User{}, users.ErrNotFound
	}
	if err != nil {
		return users.User{}, fmt.Errorf("postgres: get user: %w", err)
	}
	return out, nil
}

// ByUsername implements users.Store (case-insensitive).
func (s *UsersStore) ByUsername(ctx context.Context, username string) (users.User, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(username) = lower($1)`, username)
	out, err := scanUser(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return users.User{}, users.ErrNotFound
	}
	if err != nil {
		return users.User{}, fmt.Errorf("postgres: get user by username: %w", err)
	}
	return out, nil
}

// ByEmail implements users.Store (case-insensitive).
func (s *UsersStore) ByEmail(ctx context.Context, email string) (users.User, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE lower(email) = lower($1)`, email)
	out, err := scanUser(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return users.User{}, users.ErrNotFound
	}
	if err != nil {
		return users.User{}, fmt.Errorf("postgres: get user by email: %w", err)
	}
	return out, nil
}

// Update implements users.Store. The password hash is never written here;
// use SetPasswordHash for that.
func (s *UsersStore) Update(ctx context.Context, u users.User) (users.User, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE users
		SET username = $2, email = $3, active = $4, teacher_id = $5, updated_at = now()
		WHERE id = $1
		RETURNING `+userColumns,
		u.ID, u.Username, u.Email, u.Active, teacherIDArg(u))
	out, err := scanUser(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return users.User{}, users.ErrNotFound
	}
	if err != nil {
		return users.User{}, translateUserWrite(err, "update")
	}
	return out, nil
}

// SetPasswordHash implements users.Store.
func (s *UsersStore) SetPasswordHash(ctx context.Context, id, hash string) error {
	tag, err := s.db.pool.Exec(ctx,
		`UPDATE users SET password_hash = $2, updated_at = now() WHERE id = $1`, id, hash)
	if err != nil {
		return fmt.Errorf("postgres: set user password: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return users.ErrNotFound
	}
	return nil
}

// BumpTokenVersion implements users.Store.
func (s *UsersStore) BumpTokenVersion(ctx context.Context, id string) (int64, error) {
	row := s.db.pool.QueryRow(ctx,
		`UPDATE users SET token_version = token_version + 1, updated_at = now() WHERE id = $1 RETURNING token_version`, id)
	var ver int64
	if err := row.Scan(&ver); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, users.ErrNotFound
		}
		return 0, fmt.Errorf("postgres: bump user token version: %w", err)
	}
	return ver, nil
}

// SoftDelete implements users.Store: it stamps DeletedAt and disables
// the login without removing the row. Unknown users are a no-op;
// already deleted ones report users.ErrAlreadyDeleted.
func (s *UsersStore) SoftDelete(ctx context.Context, id string, at time.Time) error {
	tag, err := s.db.pool.Exec(ctx, `
		UPDATE users SET deleted_at = $2, active = FALSE, updated_at = now()
		WHERE id = $1 AND deleted_at IS NULL`, id, at)
	if err != nil {
		return fmt.Errorf("postgres: soft-delete user: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	if err := s.db.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: soft-delete user lookup: %w", err)
	}
	if !exists {
		return nil
	}
	return users.ErrAlreadyDeleted
}

// Restore implements users.Store: it clears the soft-delete mark and
// re-enables the login.
func (s *UsersStore) Restore(ctx context.Context, id string) error {
	tag, err := s.db.pool.Exec(ctx, `
		UPDATE users SET deleted_at = NULL, active = TRUE, updated_at = now()
		WHERE id = $1 AND deleted_at IS NOT NULL`, id)
	if err != nil {
		return fmt.Errorf("postgres: restore user: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	if err := s.db.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("postgres: restore user lookup: %w", err)
	}
	if !exists {
		return users.ErrNotFound
	}
	return users.ErrNotDeleted
}

// PurgeBefore implements users.Store: it physically removes users
// soft-deleted before cutoff and returns their ids so callers can clean
// related role grants.
func (s *UsersStore) PurgeBefore(ctx context.Context, cutoff time.Time) ([]string, error) {
	rows, err := s.db.pool.Query(ctx, `
		DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < $1 RETURNING id`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("postgres: purge users: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: scan purged user: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: purge users: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

// List implements users.Store in username order. Deleted accounts are
// included only when includeDeleted is true.
func (s *UsersStore) List(ctx context.Context, includeDeleted bool) ([]users.User, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+userColumns+` FROM users
		WHERE deleted_at IS NULL OR $1
		ORDER BY username, id`, includeDeleted)
	if err != nil {
		return nil, fmt.Errorf("postgres: list users: %w", err)
	}
	defer rows.Close()
	out := make([]users.User, 0)
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan listed user: %w", err)
		}
		out = append(out, u.Sanitized())
	}
	return out, rows.Err()
}

// ForTeacher implements users.Store.
func (s *UsersStore) ForTeacher(ctx context.Context, teacherID string) ([]users.User, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE teacher_id = $1
		ORDER BY username`,
		teacherID)
	if err != nil {
		return nil, fmt.Errorf("postgres: users for teacher: %w", err)
	}
	defer rows.Close()

	out := make([]users.User, 0)
	for rows.Next() {
		u, err := scanUser(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan user for teacher: %w", err)
		}
		out = append(out, u.Sanitized())
	}
	return out, rows.Err()
}

// Photo implements users.Store. A NULL column means no picture yet and
// scans as an empty Data.
func (s *UsersStore) Photo(ctx context.Context, id string) (users.Photo, error) {
	row := s.db.pool.QueryRow(ctx, `SELECT photo FROM users WHERE id = $1`, id)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return users.Photo{}, users.ErrNotFound
		}
		return users.Photo{}, fmt.Errorf("postgres: get user photo: %w", err)
	}
	return users.Photo{Data: data}, nil
}

// SetPhoto implements users.Store. An empty photo stores NULL, so a
// cleared picture reads back as "no picture yet" instead of zero bytes.
func (s *UsersStore) SetPhoto(ctx context.Context, id string, photo users.Photo) error {
	var data []byte
	if len(photo.Data) > 0 {
		data = photo.Data
	}
	tag, err := s.db.pool.Exec(ctx,
		`UPDATE users SET photo = $2, updated_at = now() WHERE id = $1`,
		id, data)
	if err != nil {
		return fmt.Errorf("postgres: put user photo: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return users.ErrNotFound
	}
	return nil
}

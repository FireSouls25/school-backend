package postgres

import (
	"context"
	"errors"
	"fmt"
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
const userColumns = `id, username, email, password_hash, active, teacher_id, created_at, updated_at`

// scanUser maps the current row (in userColumns order) onto a User.
func scanUser(scan func(dest ...any) error) (users.User, error) {
	var u users.User
	var teacherID *string
	var created, updated time.Time
	err := scan(&u.ID, &u.Username, &u.Email, &u.PasswordHash, &u.Active, &teacherID, &created, &updated)
	if err != nil {
		return users.User{}, err
	}
	if teacherID != nil {
		u.TeacherID = *teacherID
	}
	u.CreatedAt = created
	u.UpdatedAt = updated
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
	return []any{u.ID, u.Username, u.Email, u.PasswordHash, u.Active, teacherIDArg(u), u.CreatedAt, u.UpdatedAt}
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
		INSERT INTO users (id, username, email, password_hash, active, teacher_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

// Delete implements users.Store.
func (s *UsersStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete user: %w", err)
	}
	return nil
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

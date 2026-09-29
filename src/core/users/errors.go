package users

// Error is a domain error carrying a stable message key. The i18n layer
// translates Code into a user-facing message; Error is internal English text.
type Error struct {
	code    string
	message string
}

// Error implements the error interface with internal (English) text.
func (e *Error) Error() string { return e.message }

// Code is the stable i18n catalog key for this error.
func (e *Error) Code() string { return e.code }

func newError(code, message string) *Error {
	return &Error{code: code, message: message}
}

var (
	// ErrNotFound is returned when no user matches the given id.
	ErrNotFound = newError("users.err_not_found", "users: not found")
	// ErrInvalidID is returned when a user id is missing or malformed.
	ErrInvalidID = newError("users.err_invalid_id", "users: invalid id")
	// ErrInvalidUsername is returned when the username breaks the policy.
	ErrInvalidUsername = newError("users.err_invalid_username", "users: invalid username")
	// ErrInvalidEmail is returned when the email address is malformed.
	ErrInvalidEmail = newError("users.err_invalid_email", "users: invalid email")
	// ErrWeakPassword is returned when the password breaks the policy.
	ErrWeakPassword = newError("users.err_weak_password", "users: weak password")
	// ErrDuplicateUsername is returned when the username is already taken.
	ErrDuplicateUsername = newError("users.err_duplicate_username", "users: duplicate username")
	// ErrDuplicateEmail is returned when the email is already taken.
	ErrDuplicateEmail = newError("users.err_duplicate_email", "users: duplicate email")
	// ErrInvalidCredentials is returned on every authentication failure
	// (unknown identifier, wrong password or inactive account) so callers
	// cannot enumerate which field failed.
	ErrInvalidCredentials = newError("users.err_invalid_credentials", "users: invalid credentials")
)

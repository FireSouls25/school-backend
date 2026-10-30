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
	// ErrInvalidTeacher is returned when a linked teacher id is missing or
	// malformed. Profile existence is not checked here: the teachers
	// capability owns profiles, and a dangling link resolves to no
	// schedule rather than failing.
	ErrInvalidTeacher = newError("users.err_invalid_teacher", "users: invalid teacher id")
	// ErrPhotoTooLarge is returned when a photo exceeds MaxPhotoSize.
	ErrPhotoTooLarge = newError("users.err_photo_too_large", "users: photo too large")
	// ErrInvalidRecoveryToken is returned on every reset failure
	// (unknown, expired or reused token) so callers cannot distinguish
	// which half failed.
	ErrInvalidRecoveryToken = newError("users.err_invalid_recovery_token", "users: invalid recovery token")
	// ErrLastOwner is returned when removing the owner role from, or
	// deleting, the last remaining owner. The system never leaves
	// itself without an owner.
	ErrLastOwner = newError("users.err_last_owner", "users: last owner")
	// ErrAlreadyDeleted is returned when soft-deleting an account that
	// is already in the retention bin.
	ErrAlreadyDeleted = newError("users.err_already_deleted", "users: already deleted")
	// ErrNotDeleted is returned when restoring an account that is not
	// soft-deleted.
	ErrNotDeleted = newError("users.err_not_deleted", "users: not deleted")
	// ErrRetentionExpired is returned when restoring an account whose
	// retention window already lapsed. Only a fresh account (or a
	// database restore) can bring it back.
	ErrRetentionExpired = newError("users.err_retention_expired", "users: retention expired")
	// ErrSelfDelete is returned when an owner tries to delete their own
	// login. Destructive self-service is a footgun: another owner must
	// do it.
	ErrSelfDelete = newError("users.err_self_delete", "users: cannot delete own account")
)

package roles

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
	// ErrUnknownRole is returned when parsing or assigning an unsupported role.
	ErrUnknownRole = newError("roles.err_unknown_role", "roles: unknown role")
	// ErrInvalidSubjectID is returned when a subject identifier is missing.
	ErrInvalidSubjectID = newError("roles.err_invalid_subject", "roles: invalid subject id")
)

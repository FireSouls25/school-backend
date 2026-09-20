package teachers

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
	// ErrNotFound is returned when no teacher matches the given id.
	ErrNotFound = newError("teachers.err_not_found", "teachers: not found")
	// ErrInvalidID is returned when a teacher id is missing or malformed.
	ErrInvalidID = newError("teachers.err_invalid_id", "teachers: invalid id")
	// ErrInvalidName is returned when names or surnames are missing.
	ErrInvalidName = newError("teachers.err_invalid_name", "teachers: invalid name")
	// ErrInvalidDocument is returned when the identity document number is missing.
	ErrInvalidDocument = newError("teachers.err_invalid_document", "teachers: invalid document")
	// ErrInvalidBirth is returned when the birthdate is in the future.
	ErrInvalidBirth = newError("teachers.err_invalid_birth", "teachers: invalid birth data")
	// ErrInvalidEmail is returned when the email address is malformed.
	ErrInvalidEmail = newError("teachers.err_invalid_email", "teachers: invalid email")
)

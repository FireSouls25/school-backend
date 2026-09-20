package statistics

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
	// ErrInvalidClassGroup is returned when the class-group reference is malformed.
	ErrInvalidClassGroup = newError("statistics.err_invalid_class_group", "statistics: invalid class group")
	// ErrInvalidStudent is returned when the student reference is malformed.
	ErrInvalidStudent = newError("statistics.err_invalid_student", "statistics: invalid student id")
)

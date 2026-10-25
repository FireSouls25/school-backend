package academics

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
	// ErrInvalidStudent is returned when the alert names no valid student.
	ErrInvalidStudent = newError("academics.err_invalid_student", "academics: invalid student id")
	// ErrInvalidTeacher is returned when the alert names no valid author.
	ErrInvalidTeacher = newError("academics.err_invalid_teacher", "academics: invalid teacher id")
	// ErrEmptyReason is returned when the alert carries no reason.
	ErrEmptyReason = newError("academics.err_empty_reason", "academics: empty reason")
	// ErrNotFound is returned when the alert id does not exist.
	ErrNotFound = newError("academics.err_not_found", "academics: not found")
	// ErrAlreadyReviewed is returned when reviewing a reviewed alert.
	ErrAlreadyReviewed = newError("academics.err_already_reviewed", "academics: already reviewed")
)

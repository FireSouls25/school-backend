package attendance

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
	// ErrUnknownReason is returned when parsing or recording an unsupported reason.
	ErrUnknownReason = newError("attendance.err_unknown_reason", "attendance: unknown reason")
	// ErrInvalidStudent is returned when the student reference is missing or malformed.
	ErrInvalidStudent = newError("attendance.err_invalid_student", "attendance: invalid student id")
	// ErrInvalidClass is returned when the class reference is missing.
	ErrInvalidClass = newError("attendance.err_invalid_class", "attendance: invalid class")
	// ErrInvalidDate is returned when the record date is missing.
	ErrInvalidDate = newError("attendance.err_invalid_date", "attendance: invalid date")
	// ErrNotFound is returned when the requested record does not exist.
	ErrNotFound = newError("attendance.err_not_found", "attendance: not found")
)

package incidents

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
	// ErrUnknownSeverity is returned when parsing or recording an unsupported severity.
	ErrUnknownSeverity = newError("incidents.err_unknown_severity", "incidents: unknown severity")
	// ErrInvalidStudent is returned when the student reference is missing or malformed.
	ErrInvalidStudent = newError("incidents.err_invalid_student", "incidents: invalid student id")
	// ErrInvalidClass is returned when the class reference is missing.
	ErrInvalidClass = newError("incidents.err_invalid_class", "incidents: invalid class")
	// ErrInvalidDate is returned when the fault date is missing.
	ErrInvalidDate = newError("incidents.err_invalid_date", "incidents: invalid date")
	// ErrEmptyDescription is returned when the fault description is blank.
	ErrEmptyDescription = newError("incidents.err_empty_description", "incidents: empty description")
	// ErrNotFound is returned when the requested fault does not exist.
	ErrNotFound = newError("incidents.err_not_found", "incidents: not found")
)

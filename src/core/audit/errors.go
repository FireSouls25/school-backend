package audit

// Error is a domain error carrying a stable message key. The i18n layer
// translates Code into a user-facing message; Error is internal (English) text.
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
	// ErrNotFound is returned when no entry matches the given id.
	ErrNotFound = newError("audit.err_not_found", "audit: not found")
	// ErrInvalidID is returned when an entry id is missing or malformed.
	ErrInvalidID = newError("audit.err_invalid_id", "audit: invalid id")
	// ErrInvalidSubject is returned when the acting subject is missing or
	// malformed: an entry without an author would defeat the purpose.
	ErrInvalidSubject = newError("audit.err_invalid_subject", "audit: invalid subject")
	// ErrInvalidAction is returned when the action is not part of the catalog.
	ErrInvalidAction = newError("audit.err_invalid_action", "audit: invalid action")
	// ErrInvalidResource is returned when the resource is not part of the catalog.
	ErrInvalidResource = newError("audit.err_invalid_resource", "audit: invalid resource")
	// ErrInvalidStudent is returned when a student-scoped lookup gets a
	// malformed student id.
	ErrInvalidStudent = newError("audit.err_invalid_student", "audit: invalid student")
	// ErrInvalidLimit is returned when the requested page size is not positive.
	ErrInvalidLimit = newError("audit.err_invalid_limit", "audit: invalid limit")
)

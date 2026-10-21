package audit

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
	// ErrInvalidSubject is returned when the acting subject is missing.
	ErrInvalidSubject = newError("audit.err_invalid_subject", "audit: invalid subject")
	// ErrInvalidAction is returned when the action is outside the set.
	ErrInvalidAction = newError("audit.err_invalid_action", "audit: invalid action")
	// ErrInvalidResource is returned when the resource is outside the set.
	ErrInvalidResource = newError("audit.err_invalid_resource", "audit: invalid resource")
	// ErrInvalidStatus is returned when the outcome status is not HTTP.
	ErrInvalidStatus = newError("audit.err_invalid_status", "audit: invalid status")
)

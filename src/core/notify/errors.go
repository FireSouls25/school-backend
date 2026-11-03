package notify

// Error is a domain error carrying a stable message key. The i18n layer
// translates Code into a user-facing message; Error is internal text.
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
	// ErrInvalidRecipient is returned when the phone cannot be dialed.
	ErrInvalidRecipient = newError("notify.err_invalid_recipient", "notify: invalid recipient")
	// ErrInvalidTemplate is returned when the template name is blank.
	ErrInvalidTemplate = newError("notify.err_invalid_template", "notify: invalid template")
	// ErrNotFound is returned when no outbox row matches.
	ErrNotFound = newError("notify.err_not_found", "notify: not found")
)

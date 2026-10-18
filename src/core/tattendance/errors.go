package tattendance

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
	// ErrUnknownStatus is returned when parsing or recording an unsupported status.
	ErrUnknownStatus = newError("tattendance.err_unknown_status", "tattendance: unknown status")
	// ErrInvalidTeacher is returned when the teacher reference is missing.
	ErrInvalidTeacher = newError("tattendance.err_invalid_teacher", "tattendance: invalid teacher id")
	// ErrInvalidDay is returned when the day is missing or in the future.
	ErrInvalidDay = newError("tattendance.err_invalid_day", "tattendance: invalid day")
	// ErrEmptyDay is returned when recording a day roll without marks.
	ErrEmptyDay = newError("tattendance.err_empty_day", "tattendance: empty day")
	// ErrInvalidRange is returned when the history range is backwards or too wide.
	ErrInvalidRange = newError("tattendance.err_invalid_range", "tattendance: invalid range")
)

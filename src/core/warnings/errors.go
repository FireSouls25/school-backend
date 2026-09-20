package warnings

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
	// ErrUnknownGravity is returned when parsing or recording an unsupported gravity.
	ErrUnknownGravity = newError("warnings.err_unknown_gravity", "warnings: unknown gravity")
	// ErrInvalidStudent is returned when the student reference is missing or malformed.
	ErrInvalidStudent = newError("warnings.err_invalid_student", "warnings: invalid student id")
	// ErrInvalidClass is returned when the class reference is missing.
	ErrInvalidClass = newError("warnings.err_invalid_class", "warnings: invalid class")
	// ErrInvalidTeacher is returned when the issuing teacher reference is missing.
	ErrInvalidTeacher = newError("warnings.err_invalid_teacher", "warnings: invalid teacher id")
	// ErrInvalidDate is returned when the event date and hour are missing.
	ErrInvalidDate = newError("warnings.err_invalid_date", "warnings: invalid date")
	// ErrEmptyTitle is returned when the warning title is blank.
	ErrEmptyTitle = newError("warnings.err_empty_title", "warnings: empty title")
	// ErrEmptyDescription is returned when the warning description is blank.
	ErrEmptyDescription = newError("warnings.err_empty_description", "warnings: empty description")
	// ErrInvalidSnapshot is returned when the frozen student identity is incomplete.
	ErrInvalidSnapshot = newError("warnings.err_invalid_snapshot", "warnings: invalid snapshot")
	// ErrInvalidGroup is returned when a batch correlation id is malformed.
	ErrInvalidGroup = newError("warnings.err_invalid_group", "warnings: invalid group id")
	// ErrEmptyBatch is returned when issuing a batch without students.
	ErrEmptyBatch = newError("warnings.err_empty_batch", "warnings: empty batch")
	// ErrNotFound is returned when the requested warning does not exist.
	ErrNotFound = newError("warnings.err_not_found", "warnings: not found")
)

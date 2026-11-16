package permits

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
	// ErrNotFound is returned when no permit matches the given id.
	ErrNotFound = newError("permits.err_not_found", "permits: not found")
	// ErrInvalidID is returned when a permit id is missing or malformed.
	ErrInvalidID = newError("permits.err_invalid_id", "permits: invalid id")
	// ErrInvalidStudent is returned when the student reference is
	// missing or malformed.
	ErrInvalidStudent = newError("permits.err_invalid_student", "permits: invalid student")
	// ErrInvalidClassGroup is returned when the class-group reference is
	// missing or malformed.
	ErrInvalidClassGroup = newError("permits.err_invalid_class_group", "permits: invalid class group")
	// ErrInvalidDate is returned when the covered day cannot be parsed.
	ErrInvalidDate = newError("permits.err_invalid_date", "permits: invalid date")
	// ErrEmptyReason is returned when the permit has no motive: an
	// unexplained permission is indistinguishable from an unexcused
	// absence.
	ErrEmptyReason = newError("permits.err_empty_reason", "permits: empty reason")
	// ErrInvalidActor is returned when the granting office is unknown.
	ErrInvalidActor = newError("permits.err_invalid_actor", "permits: invalid actor")
	// ErrNotEnrolled is returned when the student is not enrolled in the
	// class-group the permit is meant to cover.
	ErrNotEnrolled = newError("permits.err_not_enrolled", "permits: student not enrolled")
	// ErrLocked is returned when a roll-call mark tries to override a
	// prior permission. The permission always wins.
	ErrLocked = newError("permits.err_locked", "permits: prior permission covers the class")
)

package classes

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
	// ErrNotFound is returned when no class-group matches the given id.
	ErrNotFound = newError("classes.err_not_found", "classes: not found")
	// ErrInvalidID is returned when a class-group id is missing or malformed.
	ErrInvalidID = newError("classes.err_invalid_id", "classes: invalid id")
	// ErrInvalidSchoolYear is returned when the school year reference is
	// missing, malformed or unknown.
	ErrInvalidSchoolYear = newError("classes.err_invalid_school_year", "classes: invalid school year")
	// ErrInvalidGrade is returned when the grade is outside 1-11.
	ErrInvalidGrade = newError("classes.err_invalid_grade", "classes: invalid grade")
	// ErrInvalidGroup is returned when the group number is below 1.
	ErrInvalidGroup = newError("classes.err_invalid_group", "classes: invalid group")
	// ErrDuplicateClass is returned when the grade+group already exists
	// in the school year.
	ErrDuplicateClass = newError("classes.err_duplicate_class", "classes: duplicate class")
	// ErrHasEnrollments is returned when deleting a group that keeps enrollments.
	ErrHasEnrollments = newError("classes.err_has_enrollments", "classes: class has enrollments")
	// ErrHasSessions is returned when deleting a group that keeps sessions.
	ErrHasSessions = newError("classes.err_has_sessions", "classes: class has sessions")
)

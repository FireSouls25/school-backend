package sessions

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
	// ErrNotFound is returned when no session matches the given id.
	ErrNotFound = newError("sessions.err_not_found", "sessions: not found")
	// ErrInvalidID is returned when a session or revision id is missing or malformed.
	ErrInvalidID = newError("sessions.err_invalid_id", "sessions: invalid id")
	// ErrInvalidClassGroup is returned when the class-group reference is
	// missing, malformed or unknown.
	ErrInvalidClassGroup = newError("sessions.err_invalid_class_group", "sessions: invalid class group")
	// ErrInvalidTeacher is returned when the teacher reference is missing or malformed.
	ErrInvalidTeacher = newError("sessions.err_invalid_teacher", "sessions: invalid teacher id")
	// ErrInvalidSubject is returned when the subject reference is malformed or unknown.
	ErrInvalidSubject = newError("sessions.err_invalid_subject", "sessions: invalid subject id")
	// ErrInvalidDate is returned when the session date is missing.
	ErrInvalidDate = newError("sessions.err_invalid_date", "sessions: invalid date")
	// ErrInvalidPeriod is returned when the period number is below 1.
	ErrInvalidPeriod = newError("sessions.err_invalid_period", "sessions: invalid period")
	// ErrInvalidClassLabel is returned when the frozen class label is missing.
	ErrInvalidClassLabel = newError("sessions.err_invalid_class_label", "sessions: invalid class label")
	// ErrInvalidSchoolYear is returned when the frozen school year is out of range.
	ErrInvalidSchoolYear = newError("sessions.err_invalid_school_year", "sessions: invalid school year")
	// ErrEmptyRoster is returned when opening a session without students.
	ErrEmptyRoster = newError("sessions.err_empty_roster", "sessions: empty roster")
	// ErrInvalidRoster is returned when a roster entry misses identity data.
	ErrInvalidRoster = newError("sessions.err_invalid_roster", "sessions: invalid roster")
	// ErrDuplicateRoster is returned when a student appears twice in the roster.
	ErrDuplicateRoster = newError("sessions.err_duplicate_roster", "sessions: duplicate roster")
	// ErrUnknownMark is returned when parsing or recording an unknown mark.
	ErrUnknownMark = newError("sessions.err_unknown_mark", "sessions: unknown mark")
	// ErrInvalidStudent is returned when the marked student reference is malformed.
	ErrInvalidStudent = newError("sessions.err_invalid_student", "sessions: invalid student id")
	// ErrNotEnrolled is returned when marking a student outside the roster.
	ErrNotEnrolled = newError("sessions.err_not_enrolled", "sessions: student not in roster")
	// ErrNoChange is returned when the new mark equals the current one.
	ErrNoChange = newError("sessions.err_no_change", "sessions: no change")
	// ErrInvalidActor is returned when the change author is missing.
	ErrInvalidActor = newError("sessions.err_invalid_actor", "sessions: invalid actor")
)

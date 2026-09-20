package schedules

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
	// ErrNotFound is returned when no schedule entry matches the given id.
	ErrNotFound = newError("schedules.err_not_found", "schedules: not found")
	// ErrInvalidID is returned when an entry id is missing or malformed.
	ErrInvalidID = newError("schedules.err_invalid_id", "schedules: invalid id")
	// ErrInvalidClassGroup is returned when the class-group reference is
	// missing, malformed or unknown.
	ErrInvalidClassGroup = newError("schedules.err_invalid_class_group", "schedules: invalid class group")
	// ErrInvalidTeacher is returned when the teacher reference is missing,
	// malformed or unknown.
	ErrInvalidTeacher = newError("schedules.err_invalid_teacher", "schedules: invalid teacher id")
	// ErrInvalidSubject is returned when the subject reference is missing,
	// malformed or unknown.
	ErrInvalidSubject = newError("schedules.err_invalid_subject", "schedules: invalid subject id")
	// ErrInvalidWeekday is returned when the weekday is outside 0–6.
	ErrInvalidWeekday = newError("schedules.err_invalid_weekday", "schedules: invalid weekday")
	// ErrInvalidTime is returned when a clock value is malformed or the
	// range is empty or inverted.
	ErrInvalidTime = newError("schedules.err_invalid_time", "schedules: invalid time")
	// ErrTeacherConflict is returned when the teacher already has a class
	// overlapping the slot.
	ErrTeacherConflict = newError("schedules.err_teacher_conflict", "schedules: teacher conflict")
	// ErrClassConflict is returned when the class-group already has a class
	// overlapping the slot.
	ErrClassConflict = newError("schedules.err_class_conflict", "schedules: class conflict")
	// ErrNotTeachingSubject is returned when the teacher does not currently
	// teach the subject.
	ErrNotTeachingSubject = newError("schedules.err_not_teaching_subject", "schedules: teacher does not teach subject")
)

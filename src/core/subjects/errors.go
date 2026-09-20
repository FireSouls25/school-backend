package subjects

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
	// ErrNotFound is returned when no subject matches the given id.
	ErrNotFound = newError("subjects.err_not_found", "subjects: not found")
	// ErrAssignmentNotFound is returned when no assignment matches the given id.
	ErrAssignmentNotFound = newError("subjects.err_assignment_not_found", "subjects: assignment not found")
	// ErrInvalidID is returned when a subject or assignment id is missing or malformed.
	ErrInvalidID = newError("subjects.err_invalid_id", "subjects: invalid id")
	// ErrInvalidName is returned when the subject name is missing.
	ErrInvalidName = newError("subjects.err_invalid_name", "subjects: invalid name")
	// ErrInvalidTeacher is returned when the teacher reference is missing or malformed.
	ErrInvalidTeacher = newError("subjects.err_invalid_teacher", "subjects: invalid teacher id")
	// ErrInvalidSubject is returned when the subject reference is missing or malformed.
	ErrInvalidSubject = newError("subjects.err_invalid_subject", "subjects: invalid subject id")
	// ErrInvalidDate is returned when the assignment start date is missing.
	ErrInvalidDate = newError("subjects.err_invalid_date", "subjects: invalid date")
	// ErrInvalidEnd is returned when the end date precedes the start date.
	ErrInvalidEnd = newError("subjects.err_invalid_end", "subjects: invalid end date")
	// ErrDuplicateSubject is returned when another subject already uses the name.
	ErrDuplicateSubject = newError("subjects.err_duplicate_subject", "subjects: duplicate subject")
	// ErrAlreadyAssigned is returned when the teacher already teaches the
	// subject in an open assignment.
	ErrAlreadyAssigned = newError("subjects.err_already_assigned", "subjects: already assigned")
	// ErrAlreadyEnded is returned when closing an assignment that is already closed.
	ErrAlreadyEnded = newError("subjects.err_already_ended", "subjects: already ended")
	// ErrHasAssignments is returned when deleting a subject that keeps history.
	ErrHasAssignments = newError("subjects.err_has_assignments", "subjects: subject has assignments")
)

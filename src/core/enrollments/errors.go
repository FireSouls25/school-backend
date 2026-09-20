package enrollments

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
	// ErrInvalidID is returned when an enrollment or promotion id is malformed.
	ErrInvalidID = newError("enrollments.err_invalid_id", "enrollments: invalid id")
	// ErrInvalidStudent is returned when the student reference is missing or malformed.
	ErrInvalidStudent = newError("enrollments.err_invalid_student", "enrollments: invalid student id")
	// ErrInvalidClassGroup is returned when the class-group reference is
	// missing, malformed or unknown.
	ErrInvalidClassGroup = newError("enrollments.err_invalid_class_group", "enrollments: invalid class group")
	// ErrDuplicateEnrollment is returned when the student is already
	// enrolled in the class-group.
	ErrDuplicateEnrollment = newError("enrollments.err_duplicate_enrollment", "enrollments: duplicate enrollment")
	// ErrInvalidDecision is returned when the promotion decision is unknown.
	ErrInvalidDecision = newError("enrollments.err_invalid_decision", "enrollments: invalid decision")
	// ErrInvalidPromotion is returned when the destination group does not
	// match the decision (set for promote/repeat, empty for graduate).
	ErrInvalidPromotion = newError("enrollments.err_invalid_promotion", "enrollments: invalid promotion")
	// ErrInvalidDate is returned when the decision date is missing.
	ErrInvalidDate = newError("enrollments.err_invalid_date", "enrollments: invalid date")
	// ErrInvalidActor is returned when the decision author is missing.
	ErrInvalidActor = newError("enrollments.err_invalid_actor", "enrollments: invalid actor")
)

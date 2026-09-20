package students

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
	// ErrNotFound is returned when no student matches the given id.
	ErrNotFound = newError("students.err_not_found", "students: not found")
	// ErrInvalidID is returned when a student id is missing or malformed.
	ErrInvalidID = newError("students.err_invalid_id", "students: invalid id")
	// ErrInvalidName is returned when names or surnames are missing.
	ErrInvalidName = newError("students.err_invalid_name", "students: invalid name")
	// ErrInvalidClass is returned when the class reference is missing.
	ErrInvalidClass = newError("students.err_invalid_class", "students: invalid class")
	// ErrPhotoTooLarge is returned when a photo exceeds MaxPhotoSize.
	ErrPhotoTooLarge = newError("students.err_photo_too_large", "students: photo too large")
	// ErrInvalidDocument is returned when the identity document number is missing.
	ErrInvalidDocument = newError("students.err_invalid_document", "students: invalid document")
	// ErrInvalidBirth is returned when the birthdate is in the future.
	ErrInvalidBirth = newError("students.err_invalid_birth", "students: invalid birth data")
	// ErrInvalidBloodType is returned when the blood type is not a known label.
	ErrInvalidBloodType = newError("students.err_invalid_blood_type", "students: invalid blood type")
	// ErrInvalidEmail is returned when the email address is malformed.
	ErrInvalidEmail = newError("students.err_invalid_email", "students: invalid email")
	// ErrInvalidGuardian is returned when guardian data is incomplete.
	ErrInvalidGuardian = newError("students.err_invalid_guardian", "students: invalid guardian")
	// ErrInvalidSibling is returned when a sibling entry misses name or class.
	ErrInvalidSibling = newError("students.err_invalid_sibling", "students: invalid sibling")
	// ErrInvalidHealth is returned when a health condition is affirmative
	// but its detail is missing.
	ErrInvalidHealth = newError("students.err_invalid_health", "students: invalid health data")
	// ErrInvalidRepeatCount is returned when the repeat count is negative.
	ErrInvalidRepeatCount = newError("students.err_invalid_repeat_count", "students: invalid repeat count")
	// ErrAlreadyGraduated is returned when graduating an already graduated student.
	ErrAlreadyGraduated = newError("students.err_already_graduated", "students: already graduated")
)

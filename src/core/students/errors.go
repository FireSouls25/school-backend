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
)

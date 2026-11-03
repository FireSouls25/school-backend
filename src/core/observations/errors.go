package observations

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
	// ErrNotFound is returned when no observation matches the given id.
	ErrNotFound = newError("observations.err_not_found", "observations: not found")
	// ErrInvalidID is returned when an observation id is missing or malformed.
	ErrInvalidID = newError("observations.err_invalid_id", "observations: invalid id")
	// ErrInvalidStudent is returned when the student reference is
	// missing or malformed.
	ErrInvalidStudent = newError("observations.err_invalid_student", "observations: invalid student")
	// ErrInvalidSituation is returned when the situation id is not in
	// the catalog. Teachers pick situations; the server owns kinds.
	ErrInvalidSituation = newError("observations.err_invalid_situation", "observations: invalid situation")
	// ErrInvalidDate is returned when the day is malformed or in the
	// future. Facts are recorded on or after they happen, never before.
	ErrInvalidDate = newError("observations.err_invalid_date", "observations: invalid date")
)

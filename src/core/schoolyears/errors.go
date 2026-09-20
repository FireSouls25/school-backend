package schoolyears

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
	// ErrNotFound is returned when no school year matches the given id.
	ErrNotFound = newError("schoolyears.err_not_found", "schoolyears: not found")
	// ErrInvalidID is returned when a school year id is missing or malformed.
	ErrInvalidID = newError("schoolyears.err_invalid_id", "schoolyears: invalid id")
	// ErrInvalidYear is returned when the year number is out of range.
	ErrInvalidYear = newError("schoolyears.err_invalid_year", "schoolyears: invalid year")
	// ErrInvalidPeriods is returned when the period count is out of range.
	ErrInvalidPeriods = newError("schoolyears.err_invalid_periods", "schoolyears: invalid periods")
	// ErrInvalidDates is returned when the date range is missing or inverted.
	ErrInvalidDates = newError("schoolyears.err_invalid_dates", "schoolyears: invalid dates")
	// ErrInvalidHoliday is returned when a holiday falls outside the range.
	ErrInvalidHoliday = newError("schoolyears.err_invalid_holiday", "schoolyears: invalid holiday")
	// ErrDuplicateYear is returned when the calendar year is already taken.
	ErrDuplicateYear = newError("schoolyears.err_duplicate_year", "schoolyears: duplicate year")
	// ErrHasClasses is returned when deleting a year that keeps class-groups.
	ErrHasClasses = newError("schoolyears.err_has_classes", "schoolyears: year has classes")
)

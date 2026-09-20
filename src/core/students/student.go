// Package students keeps the identity and profile of every enrolled
// student: names, class assignment, photo and a stable UUID. Attendance and
// incident histories live in their own capabilities and reference a student
// by this id.
package students

import (
	"strings"
)

// MaxPhotoSize bounds the stored photo to 2 MiB.
const MaxPhotoSize = 2 << 20

// Student is the profile of one enrolled student.
type Student struct {
	// ID is the unique, immutable UUID of the student.
	ID string
	// Names holds the given names (nombres).
	Names string
	// Surnames holds the family names (apellidos).
	Surnames string
	// ClassID references the class-group the student belongs to
	// (e.g. "9-1"). Opaque here; the classes capability owns its format.
	ClassID string
}

// FullName returns the display name with surnames first, the ordering
// convention used in Colombian school listings.
func (s Student) FullName() string {
	return strings.TrimSpace(s.Surnames + " " + s.Names)
}

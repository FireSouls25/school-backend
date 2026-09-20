// Package teachers keeps the profile of every teacher: names, national
// identity document, contact, birth data, medical conditions and homeroom
// assignment. Subjects taught are tracked by the subjects capability,
// which references a teacher by this id.
package teachers

import (
	"strings"
	"time"
)

// Teacher is the profile of one teacher.
type Teacher struct {
	// ID is the unique, immutable UUID of the teacher.
	ID string
	// Names holds the given names (nombres).
	Names string
	// Surnames holds the family names (apellidos).
	Surnames string
	// DocumentID is the national identity document number (cédula).
	DocumentID string
	// Phone is the teacher's phone number.
	Phone string
	// Address is the teacher's home address.
	Address string
	// Birthplace is the place of birth.
	Birthplace string
	// Birthdate is the date of birth. Age is always derived from it,
	// never stored.
	Birthdate time.Time
	// Email is the teacher's email address, if any.
	Email string
	// MedicalConditions is a free-text report of medical conditions, if any.
	MedicalConditions string
	// HomeroomClassID names the class-group the teacher leads as director
	// de grupo (e.g. "9-1"). Empty means the teacher leads no class-group.
	// Opaque here; the classes capability owns its format.
	HomeroomClassID string
}

// FullName returns the display name with surnames first, the ordering
// convention used in Colombian school listings.
func (t Teacher) FullName() string {
	return strings.TrimSpace(t.Surnames + " " + t.Names)
}

// IsHomeroomDirector reports whether the teacher leads a class-group.
func (t Teacher) IsHomeroomDirector() bool {
	return strings.TrimSpace(t.HomeroomClassID) != ""
}

// AgeAt returns the teacher's age in full years at ref. The second value
// is false when the birthdate is unknown or ref precedes it.
func (t Teacher) AgeAt(ref time.Time) (int, bool) {
	if t.Birthdate.IsZero() || ref.Before(t.Birthdate) {
		return 0, false
	}
	bd := t.Birthdate
	age := ref.Year() - bd.Year()
	if ref.Month() < bd.Month() ||
		(ref.Month() == bd.Month() && ref.Day() < bd.Day()) {
		age--
	}
	return age, true
}

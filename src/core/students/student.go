// Package students keeps the identity and profile of every enrolled
// student: names, class assignment, photo and a stable UUID. Attendance and
// incident histories live in their own capabilities and reference a student
// by this id.
package students

import (
	"strings"
	"time"
)

// MaxPhotoSize bounds the stored photo to 2 MiB.
const MaxPhotoSize = 2 << 20

// bloodTypes is the set of valid Colombian blood-type labels, normalized
// to uppercase (e.g. "O+", "AB-").
var bloodTypes = map[string]struct{}{
	"O+": {}, "O-": {},
	"A+": {}, "A-": {},
	"B+": {}, "B-": {},
	"AB+": {}, "AB-": {},
}

// NormalizeBloodType uppercases and validates a blood-type label.
func NormalizeBloodType(s string) (string, bool) {
	v := strings.ToUpper(strings.TrimSpace(s))
	if _, ok := bloodTypes[v]; !ok {
		return "", false
	}
	return v, true
}

// Condition is a yes/no profile item carrying a detail when affirmative,
// e.g. a vision difficulty and which one, or a specialist report.
type Condition struct {
	// Has reports whether the condition applies (sí/no).
	Has bool
	// Detail names which one when Has is true.
	Detail string
}

// Guardian holds the five contact facts kept for the mother, the father
// and the acudiente (caregiver).
type Guardian struct {
	Names      string
	DocumentID string
	Phone      string
	Occupation string
	Address    string
}

// IsEmpty reports whether no guardian data was provided.
func (g Guardian) IsEmpty() bool {
	return g.Names == "" && g.DocumentID == "" && g.Phone == "" &&
		g.Occupation == "" && g.Address == ""
}

// Sibling is one sibling enrolled in the institution.
type Sibling struct {
	Name    string
	ClassID string
}

// Status is the lifecycle state of a student. Stable identifiers;
// user-facing names live in the i18n catalog (student.status.<id>).
type Status string

const (
	// StatusActive is an enrolled student.
	StatusActive Status = "active"
	// StatusGraduated marks the end of the lifecycle (promoted from grade 11).
	StatusGraduated Status = "graduated"
)

// IsValid reports whether s is a known status.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusGraduated:
		return true
	}
	return false
}

// String returns the stable identifier of s.
func (s Status) String() string { return string(s) }

// MessageKey returns the i18n catalog key holding the user-facing name.
func (s Status) MessageKey() string { return "student.status." + s.String() }

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
	// DocumentID is the identity document number (TI/CC).
	DocumentID string
	// Phone is the student's own phone number, if any.
	Phone string
	// Address is the student's home address.
	Address string
	// Birthplace is the place of birth.
	Birthplace string
	// Birthdate is the date of birth. Age is always derived from it,
	// never stored.
	Birthdate time.Time
	// Email is the student's email address, if any.
	Email string
	// Vision records a vision difficulty and which one.
	Vision Condition
	// Hearing records a hearing (audición) difficulty and which one.
	Hearing Condition
	// BloodType is the normalized blood-type label (e.g. "O+").
	BloodType string
	// IsNew reports whether the student is new to the institution.
	IsNew bool
	// PreviousSchool is the former school, if any.
	PreviousSchool string
	// TransferReason explains a school change, if any.
	TransferReason string
	// RepeatCount is how many times the student has repeated a school year.
	RepeatCount int
	// Mother holds the mother's contact facts.
	Mother Guardian
	// Father holds the father's contact facts.
	Father Guardian
	// Caregiver holds the acudiente's contact facts.
	Caregiver Guardian
	// LivesWith names who the student lives with.
	LivesWith string
	// Siblings lists siblings enrolled in the institution; empty means none.
	Siblings []Sibling
	// MedicalReport is a free-text medical situation report.
	MedicalReport string
	// DiversityCondition describes a functional-diversity condition or
	// diagnosis, if any.
	DiversityCondition string
	// SpecialistReport records whether a specialist report exists and
	// which one.
	SpecialistReport Condition
	// Status is the lifecycle state. New students are active; graduation
	// moves it to graduated via Graduate, never through Update.
	Status Status
}

// FullName returns the display name with surnames first, the ordering
// convention used in Colombian school listings.
func (s Student) FullName() string {
	return strings.TrimSpace(s.Surnames + " " + s.Names)
}

// AgeAt returns the student's age in full years at ref. The second value
// is false when the birthdate is unknown or ref precedes it.
func (s Student) AgeAt(ref time.Time) (int, bool) {
	if s.Birthdate.IsZero() || ref.Before(s.Birthdate) {
		return 0, false
	}
	bd := s.Birthdate
	age := ref.Year() - bd.Year()
	if ref.Month() < bd.Month() ||
		(ref.Month() == bd.Month() && ref.Day() < bd.Day()) {
		age--
	}
	return age, true
}

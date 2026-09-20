// Package enrollments owns the yearly placement of students: which
// class-group each student belongs to in a school year, plus the audited
// promotion decisions moving them between years. Promotion suggestions and
// the student lifecycle flow build on these records later.
package enrollments

import (
	"strings"
	"time"
)

// Decision is how a student moves between years. Stable identifiers;
// user-facing names live in the i18n catalog (enrollment.decision.<id>).
type Decision string

const (
	// DecisionPromote moves the student to a group of the next grade.
	DecisionPromote Decision = "promote"
	// DecisionRepeat keeps the student in the same grade.
	DecisionRepeat Decision = "repeat"
	// DecisionGraduate ends the student's lifecycle (from grade 11).
	DecisionGraduate Decision = "graduate"
)

// IsValid reports whether d is one of the known decisions.
func (d Decision) IsValid() bool {
	switch d {
	case DecisionPromote, DecisionRepeat, DecisionGraduate:
		return true
	}
	return false
}

// String returns the stable identifier of d.
func (d Decision) String() string { return string(d) }

// MessageKey returns the i18n catalog key holding the user-facing name.
func (d Decision) MessageKey() string { return "enrollment.decision." + d.String() }

// Enrollment places one student in one class-group.
type Enrollment struct {
	// ID is the unique, immutable UUID of the enrollment.
	ID string
	// StudentID references the student. Opaque here.
	StudentID string
	// ClassGroupID references the class-group. Opaque here.
	ClassGroupID string
}

// Promotion is the audited decision moving a student between years:
// who decided what, when. Graduating students carry an empty ToGroupID.
type Promotion struct {
	// ID is the unique, immutable UUID of the promotion record.
	ID string
	// StudentID references the student. Opaque here.
	StudentID string
	// FromGroupID references the origin class-group. Opaque here.
	FromGroupID string
	// ToGroupID references the destination class-group, empty when
	// graduating. Opaque here.
	ToGroupID string
	// Decision is promote, repeat or graduate.
	Decision Decision
	// DecidedBy identifies who took the decision (opaque subject id).
	DecidedBy string
	// DecidedAt is when the decision was taken.
	DecidedAt time.Time
}

// normalizeDecision canonicalizes a decision value.
func normalizeDecision(s string) Decision {
	return Decision(strings.ToLower(strings.TrimSpace(s)))
}

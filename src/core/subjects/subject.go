// Package subjects owns the catalog of school subjects (materias) and the
// timeline of which teacher teaches each one. An Assignment links a teacher
// to a subject over a date range; an open range (zero EndedAt) means the
// teacher currently teaches it. Overlapping assignments across different
// subjects are allowed, so a teacher can teach física and informática at
// once while keeping the full history of each. Teacher ids are opaque here;
// the teachers capability owns the profiles. Subject ids are stable and
// reusable by future capabilities such as a calendar.
package subjects

import (
	"strings"
	"time"
)

// Subject is one teachable matter (e.g. Matemáticas, Ciencias Sociales).
type Subject struct {
	// ID is the unique, immutable UUID of the subject.
	ID string
	// Name is the display name. Unique, case-insensitively.
	Name string
	// Code is an optional short code (e.g. "MAT").
	Code string
	// Description is an optional free-text description.
	Description string
	// Active reports whether the subject is currently offered. Retiring a
	// subject (Active=false) keeps its assignment history; deleting it is
	// only allowed while it has none.
	Active bool
}

// Assignment links a teacher to a subject over a date range: the timeline
// entry answering "who taught X, when".
type Assignment struct {
	// ID is the unique, immutable UUID of the assignment.
	ID string
	// TeacherID references the teacher. Opaque here.
	TeacherID string
	// SubjectID references the subject.
	SubjectID string
	// StartedAt is the date from which the teacher teaches the subject.
	StartedAt time.Time
	// EndedAt closes the range. Zero means the assignment is open: the
	// teacher currently teaches the subject.
	EndedAt time.Time
}

// IsOpen reports whether the assignment is currently in force.
func (a Assignment) IsOpen() bool {
	return a.EndedAt.IsZero()
}

// normalizeName canonicalizes a subject name for comparison.
func normalizeName(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

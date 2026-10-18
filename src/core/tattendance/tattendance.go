// Package tattendance records the daily roll of teachers (asistencia de
// docentes): one explicit mark per teacher per day, taken by an admin.
// Unlike student sessions, there is no implicit present: a day without a
// mark simply has no record yet.
package tattendance

import (
	"strings"
	"time"
)

// Status is one teacher's state on a day.
type Status string

const (
	// StatusPresent means the teacher worked that day.
	StatusPresent Status = "present"
	// StatusAbsent means the teacher missed the day (ausencia).
	StatusAbsent Status = "absent"
	// StatusLate means the teacher arrived late (atraso).
	StatusLate Status = "late"
)

// Parse converts a case-insensitive, whitespace-trimmed string into a
// Status, accepting both the stable id and the Spanish term.
func Parse(s string) (Status, error) {
	switch strip(strings.ToLower(strings.TrimSpace(s))) {
	case "present", "presente":
		return StatusPresent, nil
	case "absent", "absence", "ausencia", "ausente":
		return StatusAbsent, nil
	case "late", "atraso", "tarde":
		return StatusLate, nil
	}
	return "", ErrUnknownStatus
}

func strip(s string) string {
	return strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	).Replace(s)
}

// Mark is one teacher's roll record for a day.
type Mark struct {
	TeacherID  string
	Day        time.Time
	Status     Status
	Note       string
	RecordedBy string
	RecordedAt time.Time
}

// Input carries the fields needed to record one mark.
type Input struct {
	TeacherID  string
	Day        time.Time
	Status     Status
	Note       string
	RecordedBy string
}

// DayMark is one teacher's mark within a day roll.
type DayMark struct {
	TeacherID string
	Status    Status
	Note      string
}

// DayInput carries a whole day roll: every mark shares the day while each
// teacher keeps its own history.
type DayInput struct {
	Day        time.Time
	Marks      []DayMark
	RecordedBy string
}

// Summary counts one teacher's history within a range.
type Summary struct {
	Present int
	Absent  int
	Late    int
}

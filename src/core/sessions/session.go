// Package sessions records attendance calls (llamados a lista): one teacher
// takes one class-group's roll on a date, freezing the roster as it was at
// that moment. Marks follow a git-like model: every change, including the
// first mark, is an append-only Revision, so both states are always kept
// and differences can be compared. The current state folds from revisions;
// students without a mark count as present.
package sessions

import (
	"strings"
	"time"
)

// MinYear and MaxYear bound sane frozen school years.
const MinYear = 2000

const MaxYear = 2100

// Mark is one student's state in a session. The zero value ("") means
// present: only non-present students carry marks.
type Mark string

const (
	// MarkAbsence is a plain absence (inasistencia).
	MarkAbsence Mark = "absence"
	// MarkEvasion is leaving or skipping class (evasión).
	MarkEvasion Mark = "evasion"
	// MarkLate is arriving late to class (atraso).
	MarkLate Mark = "late"
)

var knownMarks = [...]Mark{MarkAbsence, MarkEvasion, MarkLate}

// Parse converts a case-insensitive, whitespace-trimmed string into a
// Mark, accepting both the stable id and the Spanish term.
func Parse(s string) (Mark, error) {
	v := stripAccents(strings.ToLower(strings.TrimSpace(s)))
	if m := Mark(v); m.IsValid() {
		return m, nil
	}
	switch v {
	case "inasistencia":
		return MarkAbsence, nil
	case "evasion":
		return MarkEvasion, nil
	case "atraso":
		return MarkLate, nil
	}
	return "", ErrUnknownMark
}

// stripAccents folds Spanish accented vowels so "evasión" matches "evasion".
func stripAccents(s string) string {
	r := strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	)
	return r.Replace(s)
}

// IsValid reports whether m is one of the known marks.
func (m Mark) IsValid() bool {
	switch m {
	case MarkAbsence, MarkEvasion, MarkLate:
		return true
	}
	return false
}

// String returns the stable identifier of m.
func (m Mark) String() string { return string(m) }

// MessageKey returns the i18n catalog key holding the user-facing name.
// Marks share the attendance reason labels.
func (m Mark) MessageKey() string { return "reason." + m.String() }

// KnownMarks returns a fresh slice of all marks in stable order.
func KnownMarks() []Mark {
	out := make([]Mark, len(knownMarks))
	copy(out, knownMarks[:])
	return out
}

// RosterEntry freezes one student's identity as it was when the session
// opened. It is stored with the session and never follows later profile
// edits.
type RosterEntry struct {
	StudentID  string
	Names      string
	Surnames   string
	DocumentID string
}

// Session is one attendance call: teacher, class-group and date, with the
// roster frozen at opening time.
type Session struct {
	// ID is the unique, immutable UUID of the session.
	ID string
	// ClassGroupID references the class-group. Opaque here; the classes
	// capability owns class-groups.
	ClassGroupID string
	// ClassLabel freezes the group label (e.g. "7-1") as it was.
	ClassLabel string
	// SchoolYear freezes the academic year (e.g. 2026).
	SchoolYear int
	// TeacherID references the teacher who took the roll. Opaque here.
	TeacherID string
	// SubjectID references the subject when the roll belongs to one;
	// empty for homeroom rolls without a subject.
	SubjectID string
	// Date is the day the roll was taken.
	Date time.Time
	// Period is the academic period number (1-based).
	Period int
	// Roster freezes the students present in the group at opening time.
	Roster []RosterEntry
}

// Revision is one mark change, including the first mark: the git-like
// delta keeping both states. From is "" when the student was present.
type Revision struct {
	// ID is the unique, immutable UUID of the revision.
	ID string
	// SessionID references the session.
	SessionID string
	// Number is the 1-based sequence within the session.
	Number int
	// StudentID references the marked student.
	StudentID string
	// From is the previous mark, "" for present.
	From Mark
	// To is the new mark, "" for present again.
	To Mark
	// ChangedBy identifies who recorded the change (opaque subject id).
	ChangedBy string
	// ChangedAt is when the change was recorded, set by the Service.
	ChangedAt time.Time
	// Note is an optional free-text motive for the change.
	Note string
}

// SessionDetail is a session with its folded current marks and full
// revision history. Students in the roster but absent from Marks count
// as present.
type SessionDetail struct {
	Session   Session
	Marks     map[string]Mark
	Revisions []Revision
}

// MarkOf returns the student's current mark, "" for present.
func (d SessionDetail) MarkOf(studentID string) Mark {
	return d.Marks[studentID]
}

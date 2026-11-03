// Package schedules owns the weekly class schedules (horarios): which
// teacher teaches which subject to which class-group, on which weekday,
// at which hours. Entries repeat every week and are placed manually by
// administrators; the package never generates a schedule by itself.
// A teacher's day view and holiday skipping compose from these entries.
package schedules

import (
	"fmt"
	"strconv"
	"strings"
)

// Clock is a time of day as minutes since midnight (0–1439).
type Clock int

// ParseClock parses "HH:MM" (or "H:MM") into a Clock.
func ParseClock(s string) (Clock, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, ErrInvalidTime
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, ErrInvalidTime
	}
	return Clock(h*60 + m), nil
}

// MustClock parses s and panics on error. For tests and seeds only.
func MustClock(s string) Clock {
	c, err := ParseClock(s)
	if err != nil {
		panic(fmt.Sprintf("schedules: bad clock %q", s))
	}
	return c
}

// String renders the clock zero-padded ("07:30").
func (c Clock) String() string {
	return fmt.Sprintf("%02d:%02d", c/60, c%60)
}

// Entry is one weekly schedule slot: teacher + subject placed in a
// class-group on a weekday between Start (inclusive) and End (exclusive).
type Entry struct {
	// ID is the unique, immutable UUID of the entry.
	ID string
	// ClassGroupID references the class-group (salón + year). Opaque here.
	ClassGroupID string
	// TeacherID references the teacher. Opaque here.
	TeacherID string
	// SubjectID references the subject. Opaque here.
	SubjectID string
	// Weekday is the day of the week the slot repeats on.
	Weekday int
	// Start is when the slot begins.
	Start Clock
	// End is when the slot ends.
	End Clock
	// JointID links the entries of one joint workshop across salons:
	// same teacher, overlapping time, one entry per group. Empty means
	// a standalone entry. Only the joint flow assigns it.
	JointID string
	// Variant names the subject child ("1", "2") when the catalog lists
	// variants; empty otherwise. Display only: conflicts ignore it.
	Variant string
}

// Overlaps reports whether e collides with o: same weekday with
// intersecting [Start, End) ranges. Touching edges do not overlap.
func (e Entry) Overlaps(o Entry) bool {
	return e.Weekday == o.Weekday && e.Start < o.End && o.Start < e.End
}

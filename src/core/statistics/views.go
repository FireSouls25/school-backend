// Package statistics aggregates read models for admins: per-class-group
// reports and per-student reports built from sessions, warnings and
// faults. It owns no tables; data comes through reader ports implemented
// at the composition root over the existing capabilities.
package statistics

import (
	"context"
	"time"
)

// RosterEntryView is one frozen roster row: identity plus display names.
type RosterEntryView struct {
	StudentID string
	Names     string
	Surnames  string
}

// SessionView is one attendance call with its folded current marks.
// Marks map student ids to "absence", "evasion" or "late"; students
// without a mark count as present.
type SessionView struct {
	ID           string
	ClassGroupID string
	ClassLabel   string
	SchoolYear   int
	Date         time.Time
	Period       int
	Roster       []RosterEntryView
	Marks        map[string]string
}

// WarningView is one llamado de atención reduced to its gravity and date.
// Gravity is "mild", "moderate" or "severe".
type WarningView struct {
	Gravity string
	Date    time.Time
}

// FaultView is one fault reduced to its severity and date. Severity is
// "minor", "ordinary" or "severe".
type FaultView struct {
	Severity string
	Date     time.Time
}

// SessionSource reads attendance calls with folded marks.
type SessionSource interface {
	// GroupSessions returns every session of a class-group, newest first.
	GroupSessions(ctx context.Context, classGroupID string) ([]SessionView, error)
	// StudentSessions returns every session including the student in its
	// frozen roster, newest first.
	StudentSessions(ctx context.Context, studentID string) ([]SessionView, error)
}

// WarningSource reads llamados de atención per student.
type WarningSource interface {
	// StudentWarnings returns every warning of a student, newest first.
	StudentWarnings(ctx context.Context, studentID string) ([]WarningView, error)
}

// FaultSource reads faults per student.
type FaultSource interface {
	// StudentFaults returns every fault of a student, newest first.
	StudentFaults(ctx context.Context, studentID string) ([]FaultView, error)
}

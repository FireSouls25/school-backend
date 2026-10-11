// Package audit records who read or wrote student data. Colombian data law
// (Ley 1581 de 2012) requires traceability over records belonging to minors:
// the trail answers "who saw this student's file, when, and what happened".
//
// Entries are append-only and written by the transport layer, which knows the
// authenticated subject; the capability never authorizes anything itself and
// never fails a request when the trail is unavailable.
package audit

import (
	"context"
	"strings"
	"time"
)

// Action is what was done with the resource.
type Action string

const (
	// ActionRead covers any read of student data.
	ActionRead Action = "read"
	// ActionCreate covers creating a record that belongs to a student.
	ActionCreate Action = "create"
	// ActionUpdate covers changing an existing student record.
	ActionUpdate Action = "update"
	// ActionMark covers recording an attendance mark for a student.
	ActionMark Action = "mark"
)

// IsValid reports whether the action is known.
func (a Action) IsValid() bool {
	switch a {
	case ActionRead, ActionCreate, ActionUpdate, ActionMark:
		return true
	}
	return false
}

// Resource is the kind of record touched.
type Resource string

const (
	// ResourceStudent is a student profile.
	ResourceStudent Resource = "student"
	// ResourceWarning is the warnings history of a student.
	ResourceWarning Resource = "warning"
	// ResourceSession is an attendance call (lista) and its marks.
	ResourceSession Resource = "session"
	// ResourceReport is an aggregated statistics report.
	ResourceReport Resource = "report"
)

// IsValid reports whether the resource is known.
func (r Resource) IsValid() bool {
	switch r {
	case ResourceStudent, ResourceWarning, ResourceSession, ResourceReport:
		return true
	}
	return false
}

// Entry is one audited access. SubjectID is the authenticated caller (always
// the one acting, never taken from the request), ResourceID identifies the
// record touched (empty when the access is not about one specific record) and
// Status is the HTTP status the request ended with, so denials stay visible.
type Entry struct {
	ID         string
	SubjectID  string
	Action     Action
	Resource   Resource
	ResourceID string
	Status     int
	HappenedAt time.Time
}

// Store is the persistence port for the audit trail.
type Store interface {
	// Add persists one entry. Implementations must be safe for concurrent use.
	Add(ctx context.Context, e Entry) (Entry, error)
	// ForStudent returns every entry that touched studentID, newest first.
	ForStudent(ctx context.Context, studentID string) ([]Entry, error)
	// Recent returns the newest entries, at most limit, newest first.
	Recent(ctx context.Context, limit int) ([]Entry, error)
}

// Trimmed returns the entry with its optional fields normalized.
func (e Entry) Trimmed() Entry {
	e.ResourceID = strings.TrimSpace(e.ResourceID)
	return e
}

// Package audit keeps the access trail: who did what to which record,
// when, and with what outcome. The admin console (Bitácora) reads it;
// denied attempts stay visible too, so the trail doubles as a review
// surface. Only writes the handlers explicitly record land here: bulk
// read logging belongs to future middleware, not to every GET.
package audit

import (
	"context"
	"time"
)

// Action classifies what happened: reads, creations, updates and
// attendance marks. The set is closed so writers cannot invent kinds
// the console cannot render.
type Action string

const (
	ActionRead   Action = "read"
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionMark   Action = "mark"
)

// Resource classifies what the action touched. Student-data kinds come
// first (the console was built around them); schedule and catalog kinds
// joined with their phases.
type Resource string

const (
	ResourceStudent    Resource = "student"
	ResourceWarning    Resource = "warning"
	ResourceSession    Resource = "session"
	ResourceReport     Resource = "report"
	ResourceSchedule   Resource = "schedule"
	ResourceSubject    Resource = "subject"
	ResourceAssignment Resource = "assignment"
	ResourceGroup      Resource = "group"
	// ResourceTeacherAttendance marks the daily roll of teachers.
	ResourceTeacherAttendance Resource = "teacher-attendance"
	// ResourceUser marks a login account (lockout): the audit row says
	// who pressed the block button.
	ResourceUser Resource = "user"
	// ResourceAcademicAlert marks academic alerts: who raised them and who
	// reviewed them.
	ResourceAcademicAlert Resource = "academic-alert"
)

// Entry is one trail row. StudentID is empty when the action concerns
// no student (schedule and catalog writes): those rows appear in the
// global feed but never in a student file.
type Entry struct {
	ID         string
	SubjectID  string
	Action     Action
	Resource   Resource
	ResourceID string
	StudentID  string
	Status     int
	HappenedAt time.Time
}

// Filter narrows a listing: newest first, at most Limit rows, optionally
// only rows about one student.
type Filter struct {
	Limit     int
	StudentID string
}

// Store is the persistence port for the trail. Implementations must be
// safe for concurrent use. Replace MemoryStore with the PostgreSQL
// adapter in production.
type Store interface {
	// Record persists e and returns the stored entry.
	Record(ctx context.Context, e Entry) (Entry, error)
	// List returns trail rows newest first, honoring the filter.
	List(ctx context.Context, f Filter) ([]Entry, error)
}

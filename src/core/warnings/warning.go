package warnings

import (
	"context"
	"time"
)

// StudentSnapshot freezes the student's identity as it was when the
// warning was issued. It is stored with the warning and never follows
// later profile edits. The caller builds it from the current profile
// (see students.Student) at issue time.
type StudentSnapshot struct {
	Names          string
	Surnames       string
	DocumentID     string
	ClassID        string
	Birthdate      time.Time
	Age            int
	CaregiverName  string
	CaregiverPhone string
}

// Warning is one llamado de atención issued to a student. Warnings
// issued together share a GroupID, so one event against several students
// stays grouped while each student keeps its own history.
type Warning struct {
	ID          string
	StudentID   string
	ClassID     string
	TeacherID   string
	HappenedAt  time.Time
	Gravity     Gravity
	Title       string
	Description string
	Snapshot    StudentSnapshot
	// GroupID correlates warnings issued in one batch; empty for single
	// warnings.
	GroupID string
}

// Input carries the fields needed to issue a warning. The ID is assigned
// by the Service.
type Input struct {
	StudentID   string
	ClassID     string
	TeacherID   string
	HappenedAt  time.Time
	Gravity     Gravity
	Title       string
	Description string
	Snapshot    StudentSnapshot
	// GroupID correlates the warning with a batch; empty for singles.
	// IssueBatch assigns one shared id instead.
	GroupID string
}

// BatchInput carries one event against several students: shared fields
// plus one item per student. The Service assigns a single GroupID.
type BatchInput struct {
	ClassID     string
	TeacherID   string
	HappenedAt  time.Time
	Gravity     Gravity
	Title       string
	Description string
	Items       []BatchItem
}

// BatchItem is one student inside a batch, with its frozen snapshot.
type BatchItem struct {
	StudentID string
	Snapshot  StudentSnapshot
}

// Store is the persistence port for warnings. Implementations must be
// safe for concurrent use.
type Store interface {
	// Add persists w and returns the stored warning.
	Add(ctx context.Context, w Warning) (Warning, error)
	// ForStudent returns every warning of studentID, newest first.
	ForStudent(ctx context.Context, studentID string) ([]Warning, error)
	// Delete removes the warning with the given id. Removing an unknown
	// warning is a no-op.
	Delete(ctx context.Context, id string) error
	// ForGroup returns every warning sharing a batch id, oldest first.
	ForGroup(ctx context.Context, groupID string) ([]Warning, error)
}

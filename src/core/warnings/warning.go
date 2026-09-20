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

// Warning is one llamado de atención issued to a student.
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
}

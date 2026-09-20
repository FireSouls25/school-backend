package attendance

import (
	"context"
	"time"
)

// Record is one attendance entry for a student in a class-group on a date.
// Records keep the class reference so a student's history spans current and
// previous class-groups and years.
type Record struct {
	ID        string
	StudentID string
	ClassID   string
	Date      time.Time
	Reason    Reason
}

// Store is the persistence port for attendance records. Implementations
// must be safe for concurrent use.
type Store interface {
	// Add persists r and returns the stored record.
	Add(ctx context.Context, r Record) (Record, error)
	// ForStudent returns every record of studentID, newest first.
	ForStudent(ctx context.Context, studentID string) ([]Record, error)
	// Delete removes the record with the given id. Removing an unknown
	// record is a no-op.
	Delete(ctx context.Context, id string) error
}

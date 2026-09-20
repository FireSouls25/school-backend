package incidents

import (
	"context"
	"time"
)

// Fault is one recorded misbehavior for a student. Records keep the class
// reference so a student's history spans current and previous class-groups.
type Fault struct {
	ID          string
	StudentID   string
	ClassID     string
	Date        time.Time
	Severity    Severity
	Description string
}

// Store is the persistence port for faults. Implementations must be safe
// for concurrent use.
type Store interface {
	// Add persists f and returns the stored fault.
	Add(ctx context.Context, f Fault) (Fault, error)
	// ForStudent returns every fault of studentID, newest first.
	ForStudent(ctx context.Context, studentID string) ([]Fault, error)
	// Delete removes the fault with the given id. Removing an unknown
	// fault is a no-op.
	Delete(ctx context.Context, id string) error
}

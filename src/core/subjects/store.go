package subjects

import (
	"context"
	"time"
)

// Store is the persistence port for subjects and their assignment
// timeline. Implementations must be safe for concurrent use. Replace
// MemoryStore with the PostgreSQL adapter in production.
type Store interface {
	// CreateSubject persists s and returns the stored subject.
	CreateSubject(ctx context.Context, s Subject) (Subject, error)
	// SubjectByID returns the subject with the given id or ErrNotFound.
	SubjectByID(ctx context.Context, id string) (Subject, error)
	// ListSubjects returns every subject sorted alphabetically by name.
	ListSubjects(ctx context.Context) ([]Subject, error)
	// UpdateSubject replaces the subject with the given id.
	// Returns ErrNotFound when missing.
	UpdateSubject(ctx context.Context, s Subject) (Subject, error)
	// DeleteSubject removes the subject with the given id. Removing an
	// unknown subject is a no-op.
	DeleteSubject(ctx context.Context, id string) error
	// AddAssignment persists a and returns the stored assignment.
	AddAssignment(ctx context.Context, a Assignment) (Assignment, error)
	// AssignmentByID returns the assignment with the given id or
	// ErrAssignmentNotFound.
	AssignmentByID(ctx context.Context, id string) (Assignment, error)
	// AssignmentsForTeacher returns every assignment of teacherID ordered
	// chronologically by start date: the teacher's timeline.
	AssignmentsForTeacher(ctx context.Context, teacherID string) ([]Assignment, error)
	// AssignmentsForSubject returns every assignment of subjectID ordered
	// chronologically by start date: who taught it, when.
	AssignmentsForSubject(ctx context.Context, subjectID string) ([]Assignment, error)
	// EndAssignment closes the assignment with the given id at endedAt.
	// Returns ErrAssignmentNotFound when missing.
	EndAssignment(ctx context.Context, id string, endedAt time.Time) (Assignment, error)
	// RemoveAssignment deletes the assignment with the given id, for
	// corrections. Removing an unknown assignment is a no-op.
	RemoveAssignment(ctx context.Context, id string) error
}

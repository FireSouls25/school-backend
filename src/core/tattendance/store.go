package tattendance

import (
	"context"
	"time"
)

// Store is the persistence port for teacher attendance. Implementations
// must be safe for concurrent use.
type Store interface {
	// Upsert records one mark, replacing any previous mark for the same
	// teacher and day. Taking the roll again corrects it.
	Upsert(ctx context.Context, m Mark) (Mark, error)
	// ForDay returns every mark of the given day.
	ForDay(ctx context.Context, day time.Time) ([]Mark, error)
	// ForTeacher returns the marks of one teacher inside [from, to],
	// newest first.
	ForTeacher(ctx context.Context, teacherID string, from, to time.Time) ([]Mark, error)
	// AddRevision appends one mark change to the history.
	AddRevision(ctx context.Context, r Revision) (Revision, error)
	// RevisionsForTeacher returns the revisions of one teacher inside
	// [from, to], newest first.
	RevisionsForTeacher(ctx context.Context, teacherID string, from, to time.Time) ([]Revision, error)
	// ForRange returns every mark inside [from, to], newest day first.
	ForRange(ctx context.Context, from, to time.Time) ([]Mark, error)
	// RevisionsForDay returns every revision recorded on the given day,
	// newest first. It backs the per-day change trail.
	RevisionsForDay(ctx context.Context, day time.Time) ([]Revision, error)
}

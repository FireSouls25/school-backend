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
}

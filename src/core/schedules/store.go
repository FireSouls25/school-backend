package schedules

import (
	"context"
	"time"
)

// Checker reports whether a teacher currently teaches a subject. It is
// implemented at the composition root on top of the subjects capability,
// so schedules stays decoupled from it.
type Checker interface {
	// Teaches reports whether teacherID has an open assignment for subjectID.
	Teaches(ctx context.Context, teacherID, subjectID string) (bool, error)
}

// Store is the persistence port for schedule entries. Implementations must
// be safe for concurrent use. Replace MemoryStore with the PostgreSQL
// adapter in production.
type Store interface {
	// Create persists e and returns the stored entry.
	Create(ctx context.Context, e Entry) (Entry, error)
	// ByID returns the entry with the given id or ErrNotFound.
	ByID(ctx context.Context, id string) (Entry, error)
	// EntriesForTeacher returns every entry of a teacher ordered by
	// weekday, then start time.
	EntriesForTeacher(ctx context.Context, teacherID string) ([]Entry, error)
	// EntriesForGroup returns every entry of a class-group: its weekly
	// schedule ordered by weekday, then start time.
	EntriesForGroup(ctx context.Context, classGroupID string) ([]Entry, error)
	// EntriesForSubject returns every entry teaching a subject, ordered by
	// weekday, then start time. It backs the catalog delete-guard.
	EntriesForSubject(ctx context.Context, subjectID string) ([]Entry, error)
	// EntriesForDay returns every entry on the given weekday across all
	// groups, ordered by start time. It backs day-wide views.
	EntriesForDay(ctx context.Context, weekday time.Weekday) ([]Entry, error)
	// TeachersOnWeekday returns the distinct teacher ids with at least one
	// entry on the given weekday, sorted. It backs the daily teacher roll:
	// only teachers expected that day are listed.
	TeachersOnWeekday(ctx context.Context, weekday time.Weekday) ([]string, error)
	// Update replaces the entry with the given id.
	// Returns ErrNotFound when missing.
	Update(ctx context.Context, e Entry) (Entry, error)
	// Delete removes the entry with the given id. Removing an unknown
	// entry is a no-op.
	Delete(ctx context.Context, id string) error
}

// entriesOnDay filters entries to one weekday. time.Weekday matches the
// stored Weekday numbering (Sunday = 0).
func entriesOnDay(all []Entry, day time.Weekday) []Entry {
	out := make([]Entry, 0)
	for _, e := range all {
		if e.Weekday == int(day) {
			out = append(out, e)
		}
	}
	return out
}

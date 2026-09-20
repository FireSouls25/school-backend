package sessions

import "context"

// Store is the persistence port for sessions and their revision history.
// Implementations must be safe for concurrent use. Replace MemoryStore
// with the PostgreSQL adapter in production.
type Store interface {
	// CreateSession persists s with its roster and returns the stored
	// session.
	CreateSession(ctx context.Context, s Session) (Session, error)
	// SessionByID returns the session with the given id, roster included,
	// or ErrNotFound.
	SessionByID(ctx context.Context, id string) (Session, error)
	// SessionsForGroup returns every session of a class-group, newest
	// first, rosters included.
	SessionsForGroup(ctx context.Context, classGroupID string) ([]Session, error)
	// SessionsForTeacher returns every session taken by a teacher, newest
	// first, rosters included.
	SessionsForTeacher(ctx context.Context, teacherID string) ([]Session, error)
	// SessionsForStudent returns every session whose roster contains the
	// student, newest first, rosters included.
	SessionsForStudent(ctx context.Context, studentID string) ([]Session, error)
	// DeleteSession removes the session with the given id, cascading its
	// roster and revisions. Removing an unknown session is a no-op.
	DeleteSession(ctx context.Context, id string) error
	// AppendRevision persists r and returns the stored revision.
	AppendRevision(ctx context.Context, r Revision) (Revision, error)
	// RevisionsForSession returns every revision of a session ordered by
	// number, oldest first.
	RevisionsForSession(ctx context.Context, sessionID string) ([]Revision, error)
}

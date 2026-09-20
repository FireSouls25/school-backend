package sessions

import (
	"context"
	"sort"
	"sync"
)

// MemoryStore is an in-memory Store for tests and development.
// It is not intended for production use.
type MemoryStore struct {
	mu        sync.Mutex
	sessions  map[string]Session
	revisions map[string]Revision
}

// NewMemoryStore returns an empty, ready-to-use MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		sessions:  make(map[string]Session),
		revisions: make(map[string]Revision),
	}
}

// cloneSession returns a copy of s with its own roster slice.
func cloneSession(s Session) Session {
	s.Roster = append([]RosterEntry(nil), s.Roster...)
	return s
}

// CreateSession implements Store.
func (s *MemoryStore) CreateSession(_ context.Context, sess Session) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess = cloneSession(sess)
	s.sessions[sess.ID] = sess
	return cloneSession(sess), nil
}

// SessionByID implements Store.
func (s *MemoryStore) SessionByID(_ context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return cloneSession(sess), nil
}

// SessionsForGroup implements Store.
func (s *MemoryStore) SessionsForGroup(_ context.Context, classGroupID string) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0)
	for _, sess := range s.sessions {
		if sess.ClassGroupID == classGroupID {
			out = append(out, cloneSession(sess))
		}
	}
	sortSessions(out)
	return out, nil
}

// SessionsForTeacher implements Store.
func (s *MemoryStore) SessionsForTeacher(_ context.Context, teacherID string) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0)
	for _, sess := range s.sessions {
		if sess.TeacherID == teacherID {
			out = append(out, cloneSession(sess))
		}
	}
	sortSessions(out)
	return out, nil
}

// DeleteSession implements Store. Roster travels with the session;
// revisions are dropped with it.
func (s *MemoryStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	for rid, r := range s.revisions {
		if r.SessionID == id {
			delete(s.revisions, rid)
		}
	}
	return nil
}

// AppendRevision implements Store.
func (s *MemoryStore) AppendRevision(_ context.Context, r Revision) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revisions[r.ID] = r
	return r, nil
}

// RevisionsForSession implements Store.
func (s *MemoryStore) RevisionsForSession(_ context.Context, sessionID string) ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Revision, 0)
	for _, r := range s.revisions {
		if r.SessionID == sessionID {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// sortSessions orders sessions newest first, breaking ties by id.
func sortSessions(out []Session) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date.Equal(out[j].Date) {
			return out[i].ID < out[j].ID
		}
		return out[i].Date.After(out[j].Date)
	})
}

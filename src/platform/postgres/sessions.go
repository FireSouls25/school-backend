package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/sessions"
)

// SessionsStore implements sessions.Store on top of PostgreSQL.
type SessionsStore struct {
	db *DB
}

// NewSessionsStore returns a sessions.Store backed by db.
func NewSessionsStore(db *DB) *SessionsStore { return &SessionsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ sessions.Store = (*SessionsStore)(nil)

// sessionColumns is the full column list used by every session SELECT.
const sessionColumns = `id, class_group_id, class_label, school_year,
	teacher_id, subject_id, date, period`

// scanSessionMeta maps the current row (in sessionColumns order) onto a
// Session without roster; the caller loads entries separately.
func scanSessionMeta(scan func(dest ...any) error) (sessions.Session, error) {
	var s sessions.Session
	err := scan(
		&s.ID, &s.ClassGroupID, &s.ClassLabel, &s.SchoolYear,
		&s.TeacherID, &s.SubjectID, &s.Date, &s.Period,
	)
	if err != nil {
		return sessions.Session{}, err
	}
	return s, nil
}

// sessionArgs flattens s into the INSERT argument order.
func sessionArgs(s sessions.Session) []any {
	return []any{
		s.ID, s.ClassGroupID, s.ClassLabel, s.SchoolYear,
		s.TeacherID, s.SubjectID, s.Date.Format("2006-01-02"), s.Period,
	}
}

// roster loads the frozen roster of a session in enrollment order.
func (s *SessionsStore) roster(ctx context.Context, sessionID string) ([]sessions.RosterEntry, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT student_id, names, surnames, document_id
		FROM session_rosters
		WHERE session_id = $1`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list roster: %w", err)
	}
	defer rows.Close()

	out := make([]sessions.RosterEntry, 0)
	for rows.Next() {
		var e sessions.RosterEntry
		if err := rows.Scan(&e.StudentID, &e.Names, &e.Surnames, &e.DocumentID); err != nil {
			return nil, fmt.Errorf("postgres: scan roster: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// withRoster attaches rosters to sessions fetched by a query.
func (s *SessionsStore) withRoster(ctx context.Context, clause, arg string) ([]sessions.Session, error) {
	rows, err := s.db.pool.Query(ctx,
		`SELECT `+sessionColumns+` FROM sessions `+clause, arg)
	if err != nil {
		return nil, fmt.Errorf("postgres: list sessions: %w", err)
	}
	defer rows.Close()

	out := make([]sessions.Session, 0)
	for rows.Next() {
		sess, err := scanSessionMeta(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan session: %w", err)
		}
		roster, err := s.roster(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		sess.Roster = roster
		out = append(out, sess)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateSession implements sessions.Store. Session and roster persist
// atomically.
func (s *SessionsStore) CreateSession(ctx context.Context, sess sessions.Session) (sessions.Session, error) {
	tx, err := s.db.pool.Begin(ctx)
	if err != nil {
		return sessions.Session{}, fmt.Errorf("postgres: begin session: %w", err)
	}
	defer tx.Rollback(ctx)

	row := tx.QueryRow(ctx, `
		INSERT INTO sessions (id, class_group_id, class_label, school_year,
			teacher_id, subject_id, date, period)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+sessionColumns,
		sessionArgs(sess)...)
	out, err := scanSessionMeta(row.Scan)
	if err != nil {
		switch {
		case isForeignKeyViolationOn(err, "class_groups"):
			return sessions.Session{}, sessions.ErrInvalidClassGroup
		}
		return sessions.Session{}, fmt.Errorf("postgres: create session: %w", err)
	}
	for _, e := range sess.Roster {
		if _, err := tx.Exec(ctx, `
			INSERT INTO session_rosters (session_id, student_id, names, surnames, document_id)
			VALUES ($1, $2, $3, $4, $5)`,
			out.ID, e.StudentID, e.Names, e.Surnames, e.DocumentID); err != nil {
			switch {
			case isForeignKeyViolationOn(err, "students"):
				return sessions.Session{}, sessions.ErrInvalidStudent
			case isUniqueViolationOn(err, "session_rosters"):
				return sessions.Session{}, sessions.ErrDuplicateRoster
			}
			return sessions.Session{}, fmt.Errorf("postgres: create roster: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return sessions.Session{}, fmt.Errorf("postgres: commit session: %w", err)
	}
	out.Roster = append([]sessions.RosterEntry(nil), sess.Roster...)
	return out, nil
}

// SessionByID implements sessions.Store.
func (s *SessionsStore) SessionByID(ctx context.Context, id string) (sessions.Session, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+sessionColumns+` FROM sessions WHERE id = $1`, id)
	out, err := scanSessionMeta(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return sessions.Session{}, sessions.ErrNotFound
	}
	if err != nil {
		return sessions.Session{}, fmt.Errorf("postgres: get session: %w", err)
	}
	roster, err := s.roster(ctx, out.ID)
	if err != nil {
		return sessions.Session{}, err
	}
	out.Roster = roster
	return out, nil
}

// SessionsForGroup implements sessions.Store.
func (s *SessionsStore) SessionsForGroup(ctx context.Context, classGroupID string) ([]sessions.Session, error) {
	return s.withRoster(ctx,
		`WHERE class_group_id = $1 ORDER BY date DESC, id DESC`, classGroupID)
}

// SessionsForTeacher implements sessions.Store.
func (s *SessionsStore) SessionsForTeacher(ctx context.Context, teacherID string) ([]sessions.Session, error) {
	return s.withRoster(ctx,
		`WHERE teacher_id = $1 ORDER BY date DESC, id DESC`, teacherID)
}

// DeleteSession implements sessions.Store. Roster and revisions cascade.
func (s *SessionsStore) DeleteSession(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM sessions WHERE id = $1`, id); err != nil {
		return fmt.Errorf("postgres: delete session: %w", err)
	}
	return nil
}

// AppendRevision implements sessions.Store.
func (s *SessionsStore) AppendRevision(ctx context.Context, r sessions.Revision) (sessions.Revision, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO session_revisions (id, session_id, rev_no, student_id,
			from_mark, to_mark, changed_by, changed_at, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, session_id, rev_no, student_id,
			from_mark, to_mark, changed_by, changed_at, note`,
		r.ID, r.SessionID, r.Number, r.StudentID,
		string(r.From), string(r.To), r.ChangedBy,
		r.ChangedAt.Format(time.RFC3339), r.Note)
	var out sessions.Revision
	var from, to string
	err := row.Scan(&out.ID, &out.SessionID, &out.Number, &out.StudentID,
		&from, &to, &out.ChangedBy, &out.ChangedAt, &out.Note)
	if err != nil {
		if isForeignKeyViolationOn(err, "sessions") {
			return sessions.Revision{}, sessions.ErrNotFound
		}
		return sessions.Revision{}, fmt.Errorf("postgres: append revision: %w", err)
	}
	out.From = sessions.Mark(from)
	out.To = sessions.Mark(to)
	return out, nil
}

// RevisionsForSession implements sessions.Store.
func (s *SessionsStore) RevisionsForSession(ctx context.Context, sessionID string) ([]sessions.Revision, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT id, session_id, rev_no, student_id,
			from_mark, to_mark, changed_by, changed_at, note
		FROM session_revisions
		WHERE session_id = $1
		ORDER BY rev_no, id`,
		sessionID)
	if err != nil {
		return nil, fmt.Errorf("postgres: list revisions: %w", err)
	}
	defer rows.Close()

	out := make([]sessions.Revision, 0)
	for rows.Next() {
		var r sessions.Revision
		var from, to string
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Number, &r.StudentID,
			&from, &to, &r.ChangedBy, &r.ChangedAt, &r.Note); err != nil {
			return nil, fmt.Errorf("postgres: scan revision: %w", err)
		}
		r.From = sessions.Mark(from)
		r.To = sessions.Mark(to)
		out = append(out, r)
	}
	return out, rows.Err()
}

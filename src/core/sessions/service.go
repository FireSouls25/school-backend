package sessions

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies session policy and persists calls through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// OpenSession validates the call, assigns a fresh UUID and persists it
// with its frozen roster. The ID field of s is ignored.
func (s *Service) OpenSession(ctx context.Context, sess Session) (Session, error) {
	sess.ID = uuid.NewString()
	sess = normalize(sess)
	if err := validate(sess); err != nil {
		return Session{}, err
	}
	return s.store.CreateSession(ctx, sess)
}

// RecordMark records one mark change as a new revision, keeping both
// states: absence becomes late when the student arrives, late becomes
// present ("") on correction. The timestamp is set by the Service.
func (s *Service) RecordMark(ctx context.Context, sessionID, studentID string, to Mark, changedBy, note string) (Revision, error) {
	sess, err := s.SessionByID(ctx, sessionID)
	if err != nil {
		return Revision{}, err
	}
	studentID = strings.TrimSpace(studentID)
	if _, err := uuid.Parse(studentID); err != nil {
		return Revision{}, ErrInvalidStudent
	}
	if !inRoster(sess.Roster, studentID) {
		return Revision{}, ErrNotEnrolled
	}
	if to != "" && !to.IsValid() {
		return Revision{}, ErrUnknownMark
	}
	changedBy = strings.TrimSpace(changedBy)
	if changedBy == "" {
		return Revision{}, ErrInvalidActor
	}
	revs, err := s.store.RevisionsForSession(ctx, sess.ID)
	if err != nil {
		return Revision{}, err
	}
	from := fold(sess.ID, revs)[studentID]
	if from == to {
		return Revision{}, ErrNoChange
	}
	rev := Revision{
		ID:        uuid.NewString(),
		SessionID: sess.ID,
		Number:    len(revs) + 1,
		StudentID: studentID,
		From:      from,
		To:        to,
		ChangedBy: changedBy,
		ChangedAt: time.Now().UTC(),
		Note:      strings.TrimSpace(note),
	}
	return s.store.AppendRevision(ctx, rev)
}

// uniformOf folds the uniform history of one session: only explicit
// records appear, everything else counts as proper uniform.
func uniformOf(revs []UniformMark) map[string]bool {
	out := make(map[string]bool)
	for _, r := range revs {
		out[r.StudentID] = r.OK
	}
	return out
}

// SetUniform records one uniform-state change. Unchanged states are a
// no-op (ErrNoChange); the checkbox starts checked, so only faults and
// their corrections are stored.
func (s *Service) SetUniform(ctx context.Context, sessionID, studentID string, ok bool, changedBy string) (UniformMark, error) {
	sess, err := s.SessionByID(ctx, sessionID)
	if err != nil {
		return UniformMark{}, err
	}
	studentID = strings.TrimSpace(studentID)
	if _, err := uuid.Parse(studentID); err != nil {
		return UniformMark{}, ErrInvalidStudent
	}
	if !inRoster(sess.Roster, studentID) {
		return UniformMark{}, ErrNotEnrolled
	}
	changedBy = strings.TrimSpace(changedBy)
	if changedBy == "" {
		return UniformMark{}, ErrInvalidActor
	}
	hist, err := s.store.UniformHistory(ctx, sess.ID)
	if err != nil {
		return UniformMark{}, err
	}
	if current, explicit := uniformOf(hist)[studentID]; explicit && current == ok {
		return UniformMark{}, ErrNoChange
	} else if !explicit && ok {
		return UniformMark{}, ErrNoChange
	}
	m := UniformMark{
		ID:        uuid.NewString(),
		SessionID: sess.ID,
		StudentID: studentID,
		OK:        ok,
		ChangedBy: changedBy,
		ChangedAt: time.Now().UTC(),
	}
	return s.store.AppendUniformChange(ctx, m)
}

// SessionDetail returns the session with its folded current marks and
// full revision history.
func (s *Service) SessionDetail(ctx context.Context, id string) (SessionDetail, error) {
	sess, err := s.SessionByID(ctx, id)
	if err != nil {
		return SessionDetail{}, err
	}
	revs, err := s.store.RevisionsForSession(ctx, sess.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	uniformHist, err := s.store.UniformHistory(ctx, sess.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	defenses, err := s.store.DefensesForSession(ctx, sess.ID)
	if err != nil {
		return SessionDetail{}, err
	}
	return SessionDetail{
		Session:        sess,
		Marks:          fold(sess.ID, revs),
		Revisions:      revs,
		Uniform:        uniformOf(uniformHist),
		UniformHistory: uniformHist,
		Defenses:       defenses,
	}, nil
}

// SetDefense records (or replaces) one student's defense against an
// absence. Empty text clears it. Defenses only make sense on faults:
// present students have nothing to defend.
func (s *Service) SetDefense(ctx context.Context, sessionID, studentID, text, createdBy string) (Defense, error) {
	sess, err := s.SessionByID(ctx, sessionID)
	if err != nil {
		return Defense{}, err
	}
	studentID = strings.TrimSpace(studentID)
	if _, err := uuid.Parse(studentID); err != nil {
		return Defense{}, ErrInvalidStudent
	}
	if !inRoster(sess.Roster, studentID) {
		return Defense{}, ErrNotEnrolled
	}
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" {
		return Defense{}, ErrInvalidActor
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Defense{}, ErrEmptyDefense
	}
	revs, err := s.store.RevisionsForSession(ctx, sess.ID)
	if err != nil {
		return Defense{}, err
	}
	switch mark := fold(sess.ID, revs)[studentID]; mark {
	case MarkAbsence, MarkAbsencePermission:
		// faults worth defending
	default:
		return Defense{}, ErrDefenseNeedsFault
	}
	return s.store.UpsertDefense(ctx, Defense{
		SessionID: sess.ID,
		StudentID: studentID,
		Text:      text,
		CreatedBy: createdBy,
		CreatedAt: time.Now().UTC(),
	})
}

// ClearDefense removes one student's defense. Missing defenses are a
// no-op.
func (s *Service) ClearDefense(ctx context.Context, sessionID, studentID string) error {
	sess, err := s.SessionByID(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.store.DeleteDefense(ctx, sess.ID, strings.TrimSpace(studentID))
}

// SessionByID returns the session with the given id, roster included.
func (s *Service) SessionByID(ctx context.Context, id string) (Session, error) {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return Session{}, ErrInvalidID
	}
	return s.store.SessionByID(ctx, id)
}

// SessionsForGroup returns every session of a class-group, newest first.
func (s *Service) SessionsForGroup(ctx context.Context, classGroupID string) ([]Session, error) {
	if _, err := uuid.Parse(strings.TrimSpace(classGroupID)); err != nil {
		return nil, ErrInvalidClassGroup
	}
	return s.store.SessionsForGroup(ctx, strings.TrimSpace(classGroupID))
}

// SessionsForTeacher returns every session taken by a teacher, newest first.
func (s *Service) SessionsForTeacher(ctx context.Context, teacherID string) ([]Session, error) {
	if _, err := uuid.Parse(strings.TrimSpace(teacherID)); err != nil {
		return nil, ErrInvalidTeacher
	}
	return s.store.SessionsForTeacher(ctx, strings.TrimSpace(teacherID))
}

// SessionsForStudent returns every session including the student in its
// frozen roster, newest first: the student's session history.
func (s *Service) SessionsForStudent(ctx context.Context, studentID string) ([]Session, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.SessionsForStudent(ctx, strings.TrimSpace(studentID))
}

// LatestNotesForStudent maps session id to the latest non-empty
// revision note of one student.
func (s *Service) LatestNotesForStudent(ctx context.Context, studentID string) (map[string]string, error) {
	return s.store.LatestNotesForStudent(ctx, strings.TrimSpace(studentID))
}

// DefensesForStudent maps session id to the recorded defense text of one
// student, across every session including them.
func (s *Service) DefensesForStudent(ctx context.Context, studentID string) (map[string]string, error) {
	studentID = strings.TrimSpace(studentID)
	sessions, err := s.SessionsForStudent(ctx, studentID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for _, sess := range sessions {
		defs, err := s.store.DefensesForSession(ctx, sess.ID)
		if err != nil {
			return nil, err
		}
		if d, ok := defs[studentID]; ok && strings.TrimSpace(d.Text) != "" {
			out[sess.ID] = d.Text
		}
	}
	return out, nil
}

// RevisionsForSession returns every revision of a session, oldest first.
func (s *Service) RevisionsForSession(ctx context.Context, sessionID string) ([]Revision, error) {
	if _, err := uuid.Parse(strings.TrimSpace(sessionID)); err != nil {
		return nil, ErrInvalidID
	}
	return s.store.RevisionsForSession(ctx, strings.TrimSpace(sessionID))
}

// DeleteSession removes a session with its roster and revisions.
func (s *Service) DeleteSession(ctx context.Context, id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return s.store.DeleteSession(ctx, id)
}

// fold replays revisions into current marks. Students without a mark
// count as present and stay absent from the map.
func fold(sessionID string, revs []Revision) map[string]Mark {
	marks := make(map[string]Mark)
	for _, r := range revs {
		if r.SessionID != sessionID {
			continue
		}
		if r.To == "" {
			delete(marks, r.StudentID)
		} else {
			marks[r.StudentID] = r.To
		}
	}
	return marks
}

// inRoster reports whether studentID belongs to the frozen roster.
func inRoster(roster []RosterEntry, studentID string) bool {
	for _, e := range roster {
		if e.StudentID == studentID {
			return true
		}
	}
	return false
}

func normalize(sess Session) Session {
	trim := strings.TrimSpace
	sess.ClassGroupID = trim(sess.ClassGroupID)
	sess.ClassLabel = trim(sess.ClassLabel)
	sess.TeacherID = trim(sess.TeacherID)
	sess.SubjectID = trim(sess.SubjectID)
	for i := range sess.Roster {
		sess.Roster[i].StudentID = trim(sess.Roster[i].StudentID)
		sess.Roster[i].Names = trim(sess.Roster[i].Names)
		sess.Roster[i].Surnames = trim(sess.Roster[i].Surnames)
		sess.Roster[i].DocumentID = trim(sess.Roster[i].DocumentID)
	}
	return sess
}

func validate(sess Session) error {
	if sess.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(sess.ID); err != nil {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(sess.ClassGroupID); err != nil {
		return ErrInvalidClassGroup
	}
	if sess.ClassLabel == "" {
		return ErrInvalidClassLabel
	}
	if sess.SchoolYear < MinYear || sess.SchoolYear > MaxYear {
		return ErrInvalidSchoolYear
	}
	if _, err := uuid.Parse(sess.TeacherID); err != nil {
		return ErrInvalidTeacher
	}
	if sess.SubjectID != "" {
		if _, err := uuid.Parse(sess.SubjectID); err != nil {
			return ErrInvalidSubject
		}
	}
	if sess.Date.IsZero() {
		return ErrInvalidDate
	}
	if sess.Period < 1 {
		return ErrInvalidPeriod
	}
	if len(sess.Roster) == 0 {
		return ErrEmptyRoster
	}
	seen := make(map[string]struct{}, len(sess.Roster))
	for _, e := range sess.Roster {
		if _, err := uuid.Parse(e.StudentID); err != nil {
			return ErrInvalidStudent
		}
		if e.Names == "" || e.Surnames == "" {
			return ErrInvalidRoster
		}
		if _, dup := seen[e.StudentID]; dup {
			return ErrDuplicateRoster
		}
		seen[e.StudentID] = struct{}{}
	}
	return nil
}

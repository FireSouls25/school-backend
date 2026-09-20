package sessions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/sessions"
)

func newService() *sessions.Service {
	return sessions.NewService(sessions.NewMemoryStore())
}

const (
	groupID   = "11111111-1111-1111-1111-111111111111"
	teacherID = "22222222-2222-2222-2222-222222222222"
	subjectID = "33333333-3333-3333-3333-333333333333"
	studentA  = "44444444-4444-4444-4444-444444444444"
	studentB  = "55555555-5555-5555-5555-555555555555"
)

func validSession() sessions.Session {
	return sessions.Session{
		ClassGroupID: groupID,
		ClassLabel:   "9-1",
		SchoolYear:   2026,
		TeacherID:    teacherID,
		SubjectID:    subjectID,
		Date:         time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
		Period:       2,
		Roster: []sessions.RosterEntry{
			{StudentID: studentA, Names: "Ana", Surnames: "Gómez", DocumentID: "1234567890"},
			{StudentID: studentB, Names: "Luis", Surnames: "Pardo", DocumentID: "0987654321"},
		},
	}
}

func TestMarkParsing(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want sessions.Mark
	}{
		{"absence", sessions.MarkAbsence},
		{"  EVASION ", sessions.MarkEvasion},
		{"atraso", sessions.MarkLate},
		{"Evasión", sessions.MarkEvasion},
		{"inasistencia", sessions.MarkAbsence},
	} {
		got, err := sessions.Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := sessions.Parse("present"); err == nil {
		t.Error(`Parse("present") expected error: presence is the absence of a mark`)
	}
	if ms := sessions.KnownMarks(); len(ms) != 3 {
		t.Errorf("KnownMarks length = %d, want 3", len(ms))
	}
}

func TestOpenSessionValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*sessions.Session)
		want   error
	}{
		{"bad group", func(s *sessions.Session) { s.ClassGroupID = "x" }, sessions.ErrInvalidClassGroup},
		{"missing label", func(s *sessions.Session) { s.ClassLabel = " " }, sessions.ErrInvalidClassLabel},
		{"bad year", func(s *sessions.Session) { s.SchoolYear = 1999 }, sessions.ErrInvalidSchoolYear},
		{"bad teacher", func(s *sessions.Session) { s.TeacherID = "x" }, sessions.ErrInvalidTeacher},
		{"bad subject", func(s *sessions.Session) { s.SubjectID = "x" }, sessions.ErrInvalidSubject},
		{"zero date", func(s *sessions.Session) { s.Date = time.Time{} }, sessions.ErrInvalidDate},
		{"zero period", func(s *sessions.Session) { s.Period = 0 }, sessions.ErrInvalidPeriod},
		{"empty roster", func(s *sessions.Session) { s.Roster = nil }, sessions.ErrEmptyRoster},
		{"nameless roster entry", func(s *sessions.Session) { s.Roster[0].Names = "" }, sessions.ErrInvalidRoster},
		{"duplicate roster entry", func(s *sessions.Session) {
			s.Roster = append(s.Roster, s.Roster[0])
		}, sessions.ErrDuplicateRoster},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := validSession()
			tc.mutate(&s)
			if _, err := svc.OpenSession(ctx, s); !errors.Is(err, tc.want) {
				t.Errorf("OpenSession error = %v, want %v", err, tc.want)
			}
		})
	}

	// Homeroom rolls without a subject are allowed.
	s := validSession()
	s.SubjectID = ""
	if _, err := svc.OpenSession(ctx, s); err != nil {
		t.Errorf("OpenSession without subject: %v", err)
	}
}

func TestRecordMarkKeepsBothStates(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	sess, err := svc.OpenSession(ctx, validSession())
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	if sess.ID == "" {
		t.Error("OpenSession did not assign an id")
	}

	// First mark: present ("") becomes absence.
	r1, err := svc.RecordMark(ctx, sess.ID, studentA, sessions.MarkAbsence, "teacher-1", "")
	if err != nil {
		t.Fatalf("RecordMark absence: %v", err)
	}
	if r1.Number != 1 || r1.From != "" || r1.To != sessions.MarkAbsence {
		t.Errorf("rev1 = %+v, want number 1 from present to absence", r1)
	}

	// Correction: the student arrived late. Both states stay recorded.
	r2, err := svc.RecordMark(ctx, sess.ID, studentA, sessions.MarkLate, "teacher-1", "llegó 8:10")
	if err != nil {
		t.Fatalf("RecordMark late: %v", err)
	}
	if r2.Number != 2 || r2.From != sessions.MarkAbsence || r2.To != sessions.MarkLate {
		t.Errorf("rev2 = %+v, want number 2 from absence to late", r2)
	}

	detail, err := svc.SessionDetail(ctx, sess.ID)
	if err != nil {
		t.Fatalf("SessionDetail: %v", err)
	}
	if detail.MarkOf(studentA) != sessions.MarkLate {
		t.Errorf("current mark = %q, want late", detail.MarkOf(studentA))
	}
	if detail.MarkOf(studentB) != "" {
		t.Errorf("unmarked student = %q, want present", detail.MarkOf(studentB))
	}
	if len(detail.Revisions) != 2 {
		t.Fatalf("len(revisions) = %d, want 2", len(detail.Revisions))
	}

	// Repeating the current mark is a no-op error.
	if _, err := svc.RecordMark(ctx, sess.ID, studentA, sessions.MarkLate, "teacher-1", ""); !errors.Is(err, sessions.ErrNoChange) {
		t.Errorf("repeat error = %v, want ErrNoChange", err)
	}
}

func TestRecordMarkValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	sess, err := svc.OpenSession(ctx, validSession())
	if err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	outsider := "66666666-6666-6666-6666-666666666666"

	cases := []struct {
		name    string
		student string
		mark    sessions.Mark
		actor   string
		want    error
	}{
		{"outside roster", outsider, sessions.MarkAbsence, "teacher-1", sessions.ErrNotEnrolled},
		{"bad student id", "x", sessions.MarkAbsence, "teacher-1", sessions.ErrInvalidStudent},
		{"unknown mark", studentA, sessions.Mark("sick"), "teacher-1", sessions.ErrUnknownMark},
		{"missing actor", studentA, sessions.MarkAbsence, "  ", sessions.ErrInvalidActor},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RecordMark(ctx, sess.ID, tc.student, tc.mark, tc.actor, "")
			if !errors.Is(err, tc.want) {
				t.Errorf("RecordMark error = %v, want %v", err, tc.want)
			}
		})
	}

	if _, err := svc.RecordMark(ctx, "77777777-7777-7777-7777-777777777777", studentA, sessions.MarkAbsence, "t", ""); !errors.Is(err, sessions.ErrNotFound) {
		t.Errorf("unknown session error = %v, want ErrNotFound", err)
	}
}

func TestSessionListingsNewestFirst(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	old := validSession()
	old.Date = time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	if _, err := svc.OpenSession(ctx, old); err != nil {
		t.Fatalf("OpenSession old: %v", err)
	}
	recent := validSession()
	if _, err := svc.OpenSession(ctx, recent); err != nil {
		t.Fatalf("OpenSession recent: %v", err)
	}

	byGroup, err := svc.SessionsForGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("SessionsForGroup: %v", err)
	}
	if len(byGroup) != 2 || !byGroup[0].Date.After(byGroup[1].Date) {
		t.Errorf("SessionsForGroup not newest first: %+v", byGroup)
	}
	byTeacher, err := svc.SessionsForTeacher(ctx, teacherID)
	if err != nil {
		t.Fatalf("SessionsForTeacher: %v", err)
	}
	if len(byTeacher) != 2 {
		t.Errorf("len(byTeacher) = %d, want 2", len(byTeacher))
	}
}

func TestSessionsForStudent(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	both := validSession()
	if _, err := svc.OpenSession(ctx, both); err != nil {
		t.Fatalf("OpenSession: %v", err)
	}
	solo := validSession()
	solo.Date = time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC)
	solo.Roster = solo.Roster[:1]
	if _, err := svc.OpenSession(ctx, solo); err != nil {
		t.Fatalf("OpenSession solo: %v", err)
	}

	mine, err := svc.SessionsForStudent(ctx, studentA)
	if err != nil {
		t.Fatalf("SessionsForStudent: %v", err)
	}
	if len(mine) != 2 || !mine[0].Date.After(mine[1].Date) {
		t.Errorf("studentA sessions not newest first: %+v", mine)
	}
	theirs, err := svc.SessionsForStudent(ctx, studentB)
	if err != nil {
		t.Fatalf("SessionsForStudent: %v", err)
	}
	if len(theirs) != 1 {
		t.Errorf("len(studentB sessions) = %d, want 1", len(theirs))
	}

	if _, err := svc.SessionsForStudent(ctx, "nope"); !errors.Is(err, sessions.ErrInvalidStudent) {
		t.Errorf("bad student error = %v, want ErrInvalidStudent", err)
	}
}

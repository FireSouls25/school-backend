package subjects

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies catalog and timeline policy through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// CreateSubject validates the subject, assigns a fresh UUID and persists
// it. New subjects are active. The ID field of s is ignored.
func (s *Service) CreateSubject(ctx context.Context, subj Subject) (Subject, error) {
	subj.ID = uuid.NewString()
	subj = normalizeSubject(subj)
	subj.Active = true
	if err := validateSubject(subj); err != nil {
		return Subject{}, err
	}
	if err := s.ensureUniqueName(ctx, subj); err != nil {
		return Subject{}, err
	}
	return s.store.CreateSubject(ctx, subj)
}

// SubjectByID returns the subject with the given id.
func (s *Service) SubjectByID(ctx context.Context, id string) (Subject, error) {
	if err := validateID(id); err != nil {
		return Subject{}, err
	}
	return s.store.SubjectByID(ctx, id)
}

// ListSubjects returns every subject ordered alphabetically.
func (s *Service) ListSubjects(ctx context.Context) ([]Subject, error) {
	return s.store.ListSubjects(ctx)
}

// UpdateSubject replaces an existing subject. The name must stay unique.
func (s *Service) UpdateSubject(ctx context.Context, subj Subject) (Subject, error) {
	subj.ID = strings.TrimSpace(subj.ID)
	subj = normalizeSubject(subj)
	if err := validateSubject(subj); err != nil {
		return Subject{}, err
	}
	if err := s.ensureUniqueName(ctx, subj); err != nil {
		return Subject{}, err
	}
	return s.store.UpdateSubject(ctx, subj)
}

// DeleteSubject removes a subject. It is blocked while the subject keeps
// assignment history; retire it with Active=false instead.
func (s *Service) DeleteSubject(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	assigns, err := s.store.AssignmentsForSubject(ctx, strings.TrimSpace(id))
	if err != nil {
		return err
	}
	if len(assigns) > 0 {
		return ErrHasAssignments
	}
	return s.store.DeleteSubject(ctx, id)
}

// Assign starts a new timeline entry: teacherID teaches subjectID from
// startedAt on. Different subjects may overlap; the same pair cannot be
// open twice.
func (s *Service) Assign(ctx context.Context, teacherID, subjectID string, startedAt time.Time) (Assignment, error) {
	a := Assignment{
		ID:        uuid.NewString(),
		TeacherID: strings.TrimSpace(teacherID),
		SubjectID: strings.TrimSpace(subjectID),
		StartedAt: startedAt,
	}
	if err := validateTeacherID(a.TeacherID); err != nil {
		return Assignment{}, err
	}
	if err := validateID(a.SubjectID); err != nil {
		return Assignment{}, ErrInvalidSubject
	}
	if a.StartedAt.IsZero() {
		return Assignment{}, ErrInvalidDate
	}
	if _, err := s.store.SubjectByID(ctx, a.SubjectID); err != nil {
		return Assignment{}, err
	}
	timeline, err := s.store.AssignmentsForTeacher(ctx, a.TeacherID)
	if err != nil {
		return Assignment{}, err
	}
	for _, open := range timeline {
		if open.SubjectID == a.SubjectID && open.IsOpen() {
			return Assignment{}, ErrAlreadyAssigned
		}
	}
	return s.store.AddAssignment(ctx, a)
}

// EndAssignment closes an open assignment at endedAt. History is kept:
// ending matemática and assigning física afterwards preserves both
// periods on the timeline.
func (s *Service) EndAssignment(ctx context.Context, id string, endedAt time.Time) (Assignment, error) {
	if err := validateID(id); err != nil {
		return Assignment{}, err
	}
	if endedAt.IsZero() {
		return Assignment{}, ErrInvalidEnd
	}
	a, err := s.store.AssignmentByID(ctx, strings.TrimSpace(id))
	if err != nil {
		return Assignment{}, err
	}
	if !a.IsOpen() {
		return Assignment{}, ErrAlreadyEnded
	}
	if endedAt.Before(a.StartedAt) {
		return Assignment{}, ErrInvalidEnd
	}
	return s.store.EndAssignment(ctx, a.ID, endedAt)
}

// RemoveAssignment deletes an assignment entry, for corrections.
// Ending (not removing) is the way to close a finished period.
func (s *Service) RemoveAssignment(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.RemoveAssignment(ctx, id)
}

// AssignmentsForTeacher returns the teacher's full timeline ordered
// chronologically by start date.
func (s *Service) AssignmentsForTeacher(ctx context.Context, teacherID string) ([]Assignment, error) {
	if err := validateTeacherID(teacherID); err != nil {
		return nil, err
	}
	return s.store.AssignmentsForTeacher(ctx, strings.TrimSpace(teacherID))
}

// CurrentForTeacher returns the open assignments: what the teacher
// teaches right now.
func (s *Service) CurrentForTeacher(ctx context.Context, teacherID string) ([]Assignment, error) {
	timeline, err := s.AssignmentsForTeacher(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	out := make([]Assignment, 0)
	for _, a := range timeline {
		if a.IsOpen() {
			out = append(out, a)
		}
	}
	return out, nil
}

// AssignmentsForSubject returns who taught the subject and when, ordered
// chronologically.
func (s *Service) AssignmentsForSubject(ctx context.Context, subjectID string) ([]Assignment, error) {
	if err := validateID(subjectID); err != nil {
		return nil, err
	}
	return s.store.AssignmentsForSubject(ctx, strings.TrimSpace(subjectID))
}

func normalizeSubject(s Subject) Subject {
	s.Name = strings.TrimSpace(s.Name)
	s.Code = strings.ToUpper(strings.TrimSpace(s.Code))
	s.Description = strings.TrimSpace(s.Description)
	return s
}

func validateSubject(s Subject) error {
	if s.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(s.ID); err != nil {
		return ErrInvalidID
	}
	if s.Name == "" {
		return ErrInvalidName
	}
	return nil
}

// ensureUniqueName rejects names already used by another subject,
// case-insensitively.
func (s *Service) ensureUniqueName(ctx context.Context, subj Subject) error {
	all, err := s.store.ListSubjects(ctx)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.ID != subj.ID && normalizeName(other.Name) == normalizeName(subj.Name) {
			return ErrDuplicateSubject
		}
	}
	return nil
}

func validateID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidID
	}
	return nil
}

func validateTeacherID(id string) error {
	if _, err := uuid.Parse(strings.TrimSpace(id)); err != nil {
		return ErrInvalidTeacher
	}
	return nil
}

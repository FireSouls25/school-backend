package schedules

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxClock is the last valid minute of the day.
const maxClock = Clock(24*60 - 1)

// Service applies schedule policy and persists entries through a Store.
// Teacher/subject pairing is verified through a Checker supplied by the
// composition root.
type Service struct {
	store   Store
	checker Checker
}

// NewService wires a Service onto the provided Store and Checker.
func NewService(store Store, checker Checker) *Service {
	return &Service{store: store, checker: checker}
}

// Create validates the slot, checks conflicts and persists it.
// The ID field of e is ignored.
func (s *Service) Create(ctx context.Context, e Entry) (Entry, error) {
	e.ID = uuid.NewString()
	e = normalize(e)
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	if err := s.checkConflicts(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkTeaches(ctx, e); err != nil {
		return Entry{}, err
	}
	return s.store.Create(ctx, e)
}

// ByID returns the entry with the given id.
func (s *Service) ByID(ctx context.Context, id string) (Entry, error) {
	if err := validateID(id); err != nil {
		return Entry{}, err
	}
	return s.store.ByID(ctx, id)
}

// EntriesForTeacher returns every entry of a teacher ordered by weekday,
// then start time.
func (s *Service) EntriesForTeacher(ctx context.Context, teacherID string) ([]Entry, error) {
	if err := validateTeacherID(teacherID); err != nil {
		return nil, err
	}
	return s.store.EntriesForTeacher(ctx, strings.TrimSpace(teacherID))
}

// EntriesForTeacherOnDay returns a teacher's slots for one weekday: the
// day view. Holiday skipping composes on top using the year's list.
func (s *Service) EntriesForTeacherOnDay(ctx context.Context, teacherID string, day time.Weekday) ([]Entry, error) {
	if err := validateWeekday(int(day)); err != nil {
		return nil, err
	}
	all, err := s.EntriesForTeacher(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	return entriesOnDay(all, day), nil
}

// EntriesForGroup returns a class-group's weekly schedule ordered by
// weekday, then start time.
func (s *Service) EntriesForGroup(ctx context.Context, classGroupID string) ([]Entry, error) {
	if _, err := uuid.Parse(strings.TrimSpace(classGroupID)); err != nil {
		return nil, ErrInvalidClassGroup
	}
	return s.store.EntriesForGroup(ctx, strings.TrimSpace(classGroupID))
}

// Update replaces an existing entry, re-checking conflicts and pairing.
func (s *Service) Update(ctx context.Context, e Entry) (Entry, error) {
	e.ID = strings.TrimSpace(e.ID)
	e = normalize(e)
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	if err := s.checkConflicts(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkTeaches(ctx, e); err != nil {
		return Entry{}, err
	}
	return s.store.Update(ctx, e)
}

// Delete removes a schedule entry.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

func normalize(e Entry) Entry {
	e.ClassGroupID = strings.TrimSpace(e.ClassGroupID)
	e.TeacherID = strings.TrimSpace(e.TeacherID)
	e.SubjectID = strings.TrimSpace(e.SubjectID)
	return e
}

func validate(e Entry) error {
	if e.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(e.ID); err != nil {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(e.ClassGroupID); err != nil {
		return ErrInvalidClassGroup
	}
	if err := validateTeacherID(e.TeacherID); err != nil {
		return err
	}
	if _, err := uuid.Parse(e.SubjectID); err != nil {
		return ErrInvalidSubject
	}
	if err := validateWeekday(e.Weekday); err != nil {
		return err
	}
	if e.Start < 0 || e.Start > maxClock || e.End < 0 || e.End > maxClock || e.Start >= e.End {
		return ErrInvalidTime
	}
	return nil
}

// checkConflicts rejects slots overlapping the teacher's or the group's
// other entries. The entry itself is excluded, so updates are stable.
func (s *Service) checkConflicts(ctx context.Context, e Entry) error {
	owned, err := s.store.EntriesForTeacher(ctx, e.TeacherID)
	if err != nil {
		return err
	}
	for _, other := range owned {
		if other.ID != e.ID && e.Overlaps(other) {
			return ErrTeacherConflict
		}
	}
	grouped, err := s.store.EntriesForGroup(ctx, e.ClassGroupID)
	if err != nil {
		return err
	}
	for _, other := range grouped {
		if other.ID != e.ID && e.Overlaps(other) {
			return ErrClassConflict
		}
	}
	return nil
}

// checkTeaches verifies the pairing against current subject assignments.
func (s *Service) checkTeaches(ctx context.Context, e Entry) error {
	ok, err := s.checker.Teaches(ctx, e.TeacherID, e.SubjectID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotTeachingSubject
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

func validateWeekday(w int) error {
	if w < int(time.Sunday) || w > int(time.Saturday) {
		return ErrInvalidWeekday
	}
	return nil
}

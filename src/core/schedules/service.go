package schedules

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// maxClock is the last valid minute of the day.
const maxClock = Clock(24*60 - 1)

// School hours bound every schedule entry: classes run from 07:00 and the
// last slot ends at 17:00 (media ends 16:35), with a 45-minute minimum
// duration (jardín slots run 50-55 minutes). Per-school
// configuration may replace these constants later.
const (
	schoolDayStart = Clock(7 * 60)
	schoolDayEnd   = Clock(17 * 60)
	minDuration    = Clock(45)
)

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
// The ID field of e is ignored. A JointID never rides along: joint
// workshops go through CreateJoint so the whole set is validated.
func (s *Service) Create(ctx context.Context, e Entry) (Entry, error) {
	e.ID = uuid.NewString()
	e = normalize(e)
	if e.JointID != "" {
		return Entry{}, ErrInvalidJoint
	}
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	if err := s.checkConflicts(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkTeaches(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkVariant(ctx, e); err != nil {
		return Entry{}, err
	}
	return s.store.Create(ctx, e)
}

// CreateJoint creates one joint-workshop entry linked to existing ones:
// same teacher, overlapping time, one entry per group, all sharing one
// joint key. The new entry carries the joint onward; existing siblings
// are tagged with it. Group overlaps still fail: joints span groups,
// never collide inside one.
func (s *Service) CreateJoint(ctx context.Context, e Entry, joinIDs []string) (Entry, error) {
	e.ID = uuid.NewString()
	e = normalize(e)
	if len(joinIDs) == 0 {
		return Entry{}, ErrInvalidJoint
	}
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	if err := s.checkTeaches(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkVariant(ctx, e); err != nil {
		return Entry{}, err
	}
	siblings := make([]Entry, 0, len(joinIDs))
	joint := ""
	for _, id := range joinIDs {
		other, err := s.store.ByID(ctx, strings.TrimSpace(id))
		if err != nil {
			return Entry{}, err
		}
		if other.TeacherID != e.TeacherID || !e.Overlaps(other) {
			return Entry{}, ErrInvalidJoint
		}
		if other.JointID != "" {
			if joint != "" && other.JointID != joint {
				return Entry{}, ErrInvalidJoint
			}
			joint = other.JointID
		}
		siblings = append(siblings, other)
	}
	if joint == "" {
		joint = uuid.NewString()
	}
	e.JointID = joint
	if err := s.checkConflictsExcept(ctx, e, joinSet(joinIDs)); err != nil {
		return Entry{}, err
	}
	created, err := s.store.Create(ctx, e)
	if err != nil {
		return Entry{}, err
	}
	for _, other := range siblings {
		if other.JointID == joint {
			continue
		}
		other.JointID = joint
		if _, err := s.store.Update(ctx, other); err != nil {
			return Entry{}, err
		}
	}
	return created, nil
}

// Conflicts lists the teacher's entries overlapping the probe, excluding
// the probe itself by id. It powers the joint-workshop offer: same
// detection as creation, read-only. Only teacher, weekday and clocks
// matter; group and subject are irrelevant to the probe.
func (s *Service) Conflicts(ctx context.Context, probe Entry) ([]Entry, error) {
	probe = normalize(probe)
	if err := validateTeacherID(probe.TeacherID); err != nil {
		return nil, err
	}
	if err := validateWeekday(probe.Weekday); err != nil {
		return nil, err
	}
	if probe.Start < 0 || probe.Start > maxClock || probe.End < 0 || probe.End > maxClock || probe.Start >= probe.End {
		return nil, ErrInvalidTime
	}
	owned, err := s.store.EntriesForTeacher(ctx, probe.TeacherID)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0)
	for _, other := range owned {
		if other.ID != probe.ID && probe.Overlaps(other) {
			out = append(out, other)
		}
	}
	return out, nil
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

// EntriesForDay returns every entry on the given weekday across all
// groups, ordered by start time.
func (s *Service) EntriesForDay(ctx context.Context, day time.Weekday) ([]Entry, error) {
	if err := validateWeekday(int(day)); err != nil {
		return nil, err
	}
	return s.store.EntriesForDay(ctx, day)
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

// EntriesForSubject returns every entry teaching a subject, ordered by
// weekday, then start time. The catalog delete-guard counts these.
func (s *Service) EntriesForSubject(ctx context.Context, subjectID string) ([]Entry, error) {
	if _, err := uuid.Parse(strings.TrimSpace(subjectID)); err != nil {
		return nil, ErrInvalidSubject
	}
	return s.store.EntriesForSubject(ctx, strings.TrimSpace(subjectID))
}

// TeachersOnWeekday returns the distinct teacher ids expected on the
// given weekday (at least one entry), sorted. Days without entries yield
// an empty list: nobody is expected, nobody is listed.
func (s *Service) TeachersOnWeekday(ctx context.Context, weekday time.Weekday) ([]string, error) {
	if weekday < time.Sunday || weekday > time.Saturday {
		return nil, ErrInvalidWeekday
	}
	return s.store.TeachersOnWeekday(ctx, weekday)
}

// Update replaces an existing entry, re-checking conflicts and pairing.
// Joint membership is immutable here: the stored key carries over so an
// edit never silently unlinks (or forges) a joint workshop.
func (s *Service) Update(ctx context.Context, e Entry) (Entry, error) {
	e.ID = strings.TrimSpace(e.ID)
	if err := validateID(e.ID); err != nil {
		return Entry{}, err
	}
	stored, err := s.store.ByID(ctx, e.ID)
	if err != nil {
		return Entry{}, err
	}
	e = normalize(e)
	e.JointID = stored.JointID
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	if err := s.checkConflicts(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkTeaches(ctx, e); err != nil {
		return Entry{}, err
	}
	if err := s.checkVariant(ctx, e); err != nil {
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
	e.JointID = strings.TrimSpace(e.JointID)
	e.Variant = strings.TrimSpace(e.Variant)
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
	if e.Start < schoolDayStart || e.End > schoolDayEnd || e.End-e.Start < minDuration {
		return ErrOutsideSchoolHours
	}
	return nil
}

// checkConflicts rejects slots overlapping the teacher's or the group's
// other entries. The entry itself is excluded, so updates are stable.
// Entries sharing a joint key never clash on the teacher side.
func (s *Service) checkConflicts(ctx context.Context, e Entry) error {
	return s.checkConflictsExcept(ctx, e, nil)
}

// checkConflictsExcept is checkConflicts skipping extra ids: the joint
// flow excludes the siblings it is about to tag.
func (s *Service) checkConflictsExcept(ctx context.Context, e Entry, exclude map[string]bool) error {
	owned, err := s.store.EntriesForTeacher(ctx, e.TeacherID)
	if err != nil {
		return err
	}
	for _, other := range owned {
		if other.ID == e.ID || exclude[other.ID] {
			continue
		}
		if e.JointID != "" && other.JointID == e.JointID && e.Overlaps(other) {
			continue
		}
		if e.Overlaps(other) {
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

// joinSet normalizes join ids into an exclusion set.
func joinSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			out[id] = true
		}
	}
	return out
}

// checkVariant enforces the catalog's numbered children: entries name
// one of the listed variants, or none at all when the subject lists none.
func (s *Service) checkVariant(ctx context.Context, e Entry) error {
	variants, err := s.checker.VariantsOf(ctx, e.SubjectID)
	if err != nil {
		return err
	}
	if len(variants) == 0 {
		if e.Variant != "" {
			return ErrInvalidVariant
		}
		return nil
	}
	for _, v := range variants {
		if v == e.Variant {
			return nil
		}
	}
	return ErrInvalidVariant
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

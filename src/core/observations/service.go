package observations

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Store is the persistence port for observations. Implementations must
// be safe for concurrent use.
type Store interface {
	// Add persists o and returns the stored observation.
	Add(ctx context.Context, o Observation) (Observation, error)
	// ForTeacher returns a teacher's observations, newest day first.
	ForTeacher(ctx context.Context, teacherID string) ([]Observation, error)
}

// Service applies the observation policy and persists records through
// a Store. Kinds always come from the catalog, never the caller.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Recorded input for one observation: snapshots plus the resolved kind.
type Record struct {
	StudentID    string
	StudentName  string
	TeacherID    string
	ClassGroupID string
	ClassLabel   string
	SituationID  string
	Note         string
	Date         time.Time
}

// Record validates the draft against the catalog, stamps the
// server-decided kind and persists the observation. The ID field of r
// is ignored.
func (s *Service) Record(ctx context.Context, r Record) (Observation, error) {
	r.StudentID = strings.TrimSpace(r.StudentID)
	r.StudentName = strings.TrimSpace(r.StudentName)
	r.TeacherID = strings.TrimSpace(r.TeacherID)
	r.ClassGroupID = strings.TrimSpace(r.ClassGroupID)
	r.ClassLabel = strings.TrimSpace(r.ClassLabel)
	r.SituationID = strings.TrimSpace(r.SituationID)
	r.Note = strings.TrimSpace(r.Note)
	if r.StudentID == "" || r.TeacherID == "" {
		return Observation{}, ErrInvalidStudent
	}
	kind, ok := KindOf(r.SituationID)
	if !ok {
		return Observation{}, ErrInvalidSituation
	}
	if r.Date.IsZero() {
		r.Date = todayUTC()
	}
	r.Date = time.Date(r.Date.Year(), r.Date.Month(), r.Date.Day(), 0, 0, 0, 0, time.UTC)
	if r.Date.After(todayUTC()) {
		return Observation{}, ErrInvalidDate
	}
	return s.store.Add(ctx, Observation{
		ID:           uuid.NewString(),
		StudentID:    r.StudentID,
		StudentName:  r.StudentName,
		TeacherID:    r.TeacherID,
		ClassGroupID: r.ClassGroupID,
		ClassLabel:   r.ClassLabel,
		SituationID:  r.SituationID,
		Kind:         kind,
		Note:         r.Note,
		Date:         r.Date,
	})
}

// Mine returns the caller's observations, newest day first.
func (s *Service) Mine(ctx context.Context, teacherID string) ([]Observation, error) {
	if strings.TrimSpace(teacherID) == "" {
		return nil, ErrInvalidStudent
	}
	return s.store.ForTeacher(ctx, strings.TrimSpace(teacherID))
}

func todayUTC() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

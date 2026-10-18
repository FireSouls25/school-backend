package audit

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// MaxLimit caps listings: the trail is append-only and unbounded, so an
// open-ended read would eventually hurt.
const MaxLimit = 200

// DefaultLimit applies when the caller passes no (or a non-positive) limit.
const DefaultLimit = 50

// Service validates and persists trail entries.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Record validates one trail row and persists it. Recording never fails
// the action it trails: callers log and continue on error.
func (s *Service) Record(ctx context.Context, e Entry) (Entry, error) {
	e.ID = uuid.NewString()
	e.SubjectID = strings.TrimSpace(e.SubjectID)
	e.ResourceID = strings.TrimSpace(e.ResourceID)
	e.StudentID = strings.TrimSpace(e.StudentID)
	e.HappenedAt = time.Now().UTC()
	if _, err := uuid.Parse(e.SubjectID); err != nil {
		return Entry{}, ErrInvalidSubject
	}
	switch e.Action {
	case ActionRead, ActionCreate, ActionUpdate, ActionDelete, ActionMark:
	default:
		return Entry{}, ErrInvalidAction
	}
	switch e.Resource {
	case ResourceStudent, ResourceWarning, ResourceSession, ResourceReport,
		ResourceSchedule, ResourceSubject, ResourceAssignment, ResourceGroup,
		ResourceTeacherAttendance:
	default:
		return Entry{}, ErrInvalidResource
	}
	if e.ResourceID != "" {
		if _, err := uuid.Parse(e.ResourceID); err != nil {
			return Entry{}, ErrInvalidResource
		}
	}
	if e.StudentID != "" {
		if _, err := uuid.Parse(e.StudentID); err != nil {
			return Entry{}, ErrInvalidResource
		}
	}
	if e.Status < 100 || e.Status > 599 {
		return Entry{}, ErrInvalidStatus
	}
	return s.store.Record(ctx, e)
}

// List returns trail rows newest first, honoring the filter. Limits clamp
// to [1, MaxLimit]; a student filter matches only rows about that student.
func (s *Service) List(ctx context.Context, f Filter) ([]Entry, error) {
	if f.Limit <= 0 {
		f.Limit = DefaultLimit
	}
	if f.Limit > MaxLimit {
		f.Limit = MaxLimit
	}
	f.StudentID = strings.TrimSpace(f.StudentID)
	if f.StudentID != "" {
		if _, err := uuid.Parse(f.StudentID); err != nil {
			return nil, ErrInvalidResource
		}
	}
	return s.store.List(ctx, f)
}

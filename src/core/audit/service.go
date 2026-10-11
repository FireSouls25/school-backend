package audit

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies the audit policy and persists entries through a Store.
// Timestamps are always set by the Service, never by the caller.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// MaxRecent caps how many entries Recent will ever return.
const MaxRecent = 200

// Record validates and persists one audited access, stamping the time and
// assigning the id. Callers are the transport layer: the subject must be the
// authenticated caller.
func (s *Service) Record(ctx context.Context, subjectID string, action Action, resource Resource, resourceID string, status int) (Entry, error) {
	e := Entry{
		ID:         uuid.NewString(),
		SubjectID:  strings.TrimSpace(subjectID),
		Action:     action,
		Resource:   resource,
		ResourceID: strings.TrimSpace(resourceID),
		Status:     status,
		HappenedAt: time.Now().UTC(),
	}
	if err := validate(e); err != nil {
		return Entry{}, err
	}
	return s.store.Add(ctx, e)
}

// ForStudent returns every access touching the given student, newest first.
func (s *Service) ForStudent(ctx context.Context, studentID string) ([]Entry, error) {
	if _, err := uuid.Parse(strings.TrimSpace(studentID)); err != nil {
		return nil, ErrInvalidStudent
	}
	return s.store.ForStudent(ctx, studentID)
}

// Recent returns the newest entries, capped at MaxRecent.
func (s *Service) Recent(ctx context.Context, limit int) ([]Entry, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}
	if limit > MaxRecent {
		limit = MaxRecent
	}
	return s.store.Recent(ctx, limit)
}

func validate(e Entry) error {
	if _, err := uuid.Parse(strings.TrimSpace(e.SubjectID)); err != nil {
		return ErrInvalidSubject
	}
	if !e.Action.IsValid() {
		return ErrInvalidAction
	}
	if !e.Resource.IsValid() {
		return ErrInvalidResource
	}
	return nil
}

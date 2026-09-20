package schoolyears

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Service applies school year policy and persists years through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Create validates the year, assigns a fresh UUID and persists it.
// The ID field of y is ignored.
func (s *Service) Create(ctx context.Context, y SchoolYear) (SchoolYear, error) {
	y.ID = uuid.NewString()
	y = normalize(y)
	if err := validate(y); err != nil {
		return SchoolYear{}, err
	}
	if err := s.ensureUniqueYear(ctx, y); err != nil {
		return SchoolYear{}, err
	}
	return s.store.Create(ctx, y)
}

// ByID returns the school year with the given id.
func (s *Service) ByID(ctx context.Context, id string) (SchoolYear, error) {
	if err := validateID(id); err != nil {
		return SchoolYear{}, err
	}
	return s.store.ByID(ctx, id)
}

// ByYear returns the school year with the given calendar year.
func (s *Service) ByYear(ctx context.Context, year int) (SchoolYear, error) {
	return s.store.ByYear(ctx, year)
}

// List returns every school year ordered by year, newest first.
func (s *Service) List(ctx context.Context) ([]SchoolYear, error) {
	return s.store.List(ctx)
}

// Update replaces an existing school year. The year number must stay unique.
func (s *Service) Update(ctx context.Context, y SchoolYear) (SchoolYear, error) {
	y.ID = strings.TrimSpace(y.ID)
	y = normalize(y)
	if err := validate(y); err != nil {
		return SchoolYear{}, err
	}
	if err := s.ensureUniqueYear(ctx, y); err != nil {
		return SchoolYear{}, err
	}
	return s.store.Update(ctx, y)
}

// Delete removes a school year. It is blocked while the year keeps
// class-groups; the database enforces this with ErrHasClasses.
func (s *Service) Delete(ctx context.Context, id string) error {
	if err := validateID(id); err != nil {
		return err
	}
	return s.store.Delete(ctx, id)
}

// normalize truncates dates to UTC midnight and canonicalizes holidays.
func normalize(y SchoolYear) SchoolYear {
	toDate := func(t time.Time) time.Time {
		if t.IsZero() {
			return t
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}
	y.StartDate = toDate(y.StartDate)
	y.EndDate = toDate(y.EndDate)
	y.Holidays = normalizeHolidays(y.Holidays)
	return y
}

func validate(y SchoolYear) error {
	if y.ID == "" {
		return ErrInvalidID
	}
	if _, err := uuid.Parse(y.ID); err != nil {
		return ErrInvalidID
	}
	if y.Year < MinYear || y.Year > MaxYear {
		return ErrInvalidYear
	}
	if y.Periods < MinPeriods || y.Periods > MaxPeriods {
		return ErrInvalidPeriods
	}
	if y.StartDate.IsZero() || y.EndDate.IsZero() || !y.StartDate.Before(y.EndDate) {
		return ErrInvalidDates
	}
	for _, h := range y.Holidays {
		if !containsDate(h, y.StartDate, y.EndDate) {
			return ErrInvalidHoliday
		}
	}
	return nil
}

// ensureUniqueYear rejects calendar years already taken by another record.
func (s *Service) ensureUniqueYear(ctx context.Context, y SchoolYear) error {
	all, err := s.store.List(ctx)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.ID != y.ID && other.Year == y.Year {
			return ErrDuplicateYear
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

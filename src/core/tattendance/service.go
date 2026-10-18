package tattendance

import (
	"context"
	"strings"
	"time"
)

// MaxHistoryDays bounds history ranges so a ficha never pulls years.
const MaxHistoryDays = 366

// Service applies teacher-attendance policy and persists through a Store.
type Service struct {
	store Store
}

// NewService wires a Service onto the provided Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// RecordDay replaces the whole day roll: every mark shares the day while
// each teacher keeps its own history. All marks validate before anything
// persists, and recording again corrects the day.
func (s *Service) RecordDay(ctx context.Context, in DayInput) ([]Mark, error) {
	day, err := normalizeDay(in.Day)
	if err != nil {
		return nil, err
	}
	if len(in.Marks) == 0 {
		return nil, ErrEmptyDay
	}
	marks := make([]Mark, 0, len(in.Marks))
	for _, dm := range in.Marks {
		status := dm.Status
		if status == "" {
			status = StatusPresent
		}
		if status != StatusPresent && status != StatusAbsent && status != StatusLate {
			return nil, ErrUnknownStatus
		}
		teacherID := strings.TrimSpace(dm.TeacherID)
		if teacherID == "" {
			return nil, ErrInvalidTeacher
		}
		marks = append(marks, Mark{
			TeacherID:  teacherID,
			Day:        day,
			Status:     status,
			Note:       strings.TrimSpace(dm.Note),
			RecordedBy: strings.TrimSpace(in.RecordedBy),
			RecordedAt: time.Now().UTC(),
		})
	}
	out := make([]Mark, 0, len(marks))
	for _, m := range marks {
		stored, err := s.store.Upsert(ctx, m)
		if err != nil {
			return nil, err
		}
		out = append(out, stored)
	}
	return out, nil
}

// Day returns every mark of the given day.
func (s *Service) Day(ctx context.Context, day time.Time) ([]Mark, error) {
	d, err := normalizeDay(day)
	if err != nil {
		return nil, err
	}
	return s.store.ForDay(ctx, d)
}

// History returns one teacher's marks inside [from, to], newest first,
// with its summary counts.
func (s *Service) History(ctx context.Context, teacherID string, from, to time.Time) ([]Mark, Summary, error) {
	if strings.TrimSpace(teacherID) == "" {
		return nil, Summary{}, ErrInvalidTeacher
	}
	if to.IsZero() {
		to = time.Now().UTC()
	}
	if from.IsZero() {
		from = to.AddDate(0, 0, -30)
	}
	from = startOfDay(from)
	to = startOfDay(to)
	if to.Before(from) || to.Sub(from).Hours() > MaxHistoryDays*24 {
		return nil, Summary{}, ErrInvalidRange
	}
	marks, err := s.store.ForTeacher(ctx, strings.TrimSpace(teacherID), from, to)
	if err != nil {
		return nil, Summary{}, err
	}
	var sum Summary
	for _, m := range marks {
		switch m.Status {
		case StatusPresent:
			sum.Present++
		case StatusAbsent:
			sum.Absent++
		case StatusLate:
			sum.Late++
		}
	}
	return marks, sum, nil
}

// normalizeDay truncates to the day and rejects future days: the roll
// records what happened, never what will happen.
func normalizeDay(day time.Time) (time.Time, error) {
	if day.IsZero() {
		return time.Time{}, ErrInvalidDay
	}
	d := startOfDay(day)
	if d.After(startOfDay(time.Now().UTC())) {
		return time.Time{}, ErrInvalidDay
	}
	return d, nil
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

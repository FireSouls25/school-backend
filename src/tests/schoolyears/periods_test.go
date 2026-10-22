package schoolyears_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/schoolyears"
)

func rangedYear() schoolyears.SchoolYear {
	y := validYear()
	y.PeriodRanges = []schoolyears.PeriodRange{
		{Start: date(2026, 2, 2), End: date(2026, 4, 17)},
		{Start: date(2026, 4, 20), End: date(2026, 7, 17)},
		{Start: date(2026, 7, 27), End: date(2026, 11, 27)},
	}
	return y
}

func TestPeriodRangesValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*schoolyears.SchoolYear)
	}{
		{"miscounted ranges", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges = y.PeriodRanges[:2]
		}},
		{"inverted range", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges[0].Start, y.PeriodRanges[0].End = y.PeriodRanges[0].End, y.PeriodRanges[0].Start
		}},
		{"range outside year", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges[2].End = date(2026, 12, 20)
		}},
		{"unordered ranges", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges[0], y.PeriodRanges[1] = y.PeriodRanges[1], y.PeriodRanges[0]
		}},
		{"overlapping ranges", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges[1].Start = date(2026, 4, 10)
		}},
		{"touching borders", func(y *schoolyears.SchoolYear) {
			y.PeriodRanges[1].Start = date(2026, 4, 17)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y := rangedYear()
			tc.mutate(&y)
			if _, err := svc.Create(ctx, y); !errors.Is(err, schoolyears.ErrInvalidPeriodRanges) {
				t.Errorf("Create error = %v, want ErrInvalidPeriodRanges", err)
			}
		})
	}
}

func TestLegacyYearWithoutRanges(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	y, err := svc.Create(ctx, validYear())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(y.PeriodRanges) != 0 {
		t.Fatalf("ranges = %v, want empty legacy", y.PeriodRanges)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	y, err := svc.Create(ctx, rangedYear())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	closed, err := svc.Close(ctx, y.ID)
	if err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !closed.Closed {
		t.Fatal("closed flag false, want true")
	}
	again, err := svc.Close(ctx, closed.ID)
	if err != nil || !again.Closed {
		t.Fatalf("second Close = %v, %v; want closed, nil", again.Closed, err)
	}
	if _, err := svc.Close(ctx, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, schoolyears.ErrNotFound) {
		t.Fatalf("unknown Close error = %v, want ErrNotFound", err)
	}
}

func TestNoClassDaysValidationAndKind(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	y := rangedYear()
	y.Year = 2031
	y.NoClassDays = []time.Time{date(2026, 5, 4)}
	if _, err := svc.Create(ctx, y); err != nil {
		t.Fatalf("Create with no-class day: %v", err)
	}
	holiday, noClass := y.DayKind(date(2026, 5, 4))
	if holiday || !noClass {
		t.Fatalf("DayKind = %v,%v; want false,true", holiday, noClass)
	}
	if y.IsSchoolDay(date(2026, 5, 4)) {
		t.Fatal("IsSchoolDay true on no-class day, want false")
	}
	// Overlap with an official holiday reports both.
	y2 := rangedYear()
	y2.Year = 2032
	y2.Holidays = []time.Time{date(2026, 5, 4)}
	y2.NoClassDays = []time.Time{date(2026, 5, 4)}
	if _, err := svc.Create(ctx, y2); err != nil {
		t.Fatalf("Create with overlap: %v", err)
	}
	if h, n := y2.DayKind(date(2026, 5, 4)); !h || !n {
		t.Fatalf("DayKind overlap = %v,%v; want true,true", h, n)
	}
	// Out-of-range no-class day fails like a bad holiday.
	bad := rangedYear()
	bad.Year = 2033
	bad.NoClassDays = []time.Time{date(2027, 1, 5)}
	if _, err := svc.Create(ctx, bad); !errors.Is(err, schoolyears.ErrInvalidHoliday) {
		t.Fatalf("Create error = %v, want ErrInvalidHoliday", err)
	}
}

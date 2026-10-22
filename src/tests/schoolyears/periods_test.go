package schoolyears_test

import (
	"context"
	"errors"
	"testing"

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

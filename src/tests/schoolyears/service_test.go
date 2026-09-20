package schoolyears_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"grade/src/core/schoolyears"
)

func newService() *schoolyears.Service {
	return schoolyears.NewService(schoolyears.NewMemoryStore())
}

func date(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// validYear returns a valid school year exercising every field.
func validYear() schoolyears.SchoolYear {
	return schoolyears.SchoolYear{
		Year: 2026, Periods: 3,
		StartDate: date(2026, 2, 2), EndDate: date(2026, 11, 27),
		Holidays: []time.Time{date(2026, 5, 1), date(2026, 7, 20)},
	}
}

func TestServiceCreateValidation(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	cases := []struct {
		name   string
		mutate func(*schoolyears.SchoolYear)
		want   error
	}{
		{"year too low", func(y *schoolyears.SchoolYear) { y.Year = 1999 }, schoolyears.ErrInvalidYear},
		{"year too high", func(y *schoolyears.SchoolYear) { y.Year = 2101 }, schoolyears.ErrInvalidYear},
		{"zero periods", func(y *schoolyears.SchoolYear) { y.Periods = 0 }, schoolyears.ErrInvalidPeriods},
		{"too many periods", func(y *schoolyears.SchoolYear) { y.Periods = 7 }, schoolyears.ErrInvalidPeriods},
		{"missing dates", func(y *schoolyears.SchoolYear) { y.StartDate = time.Time{} }, schoolyears.ErrInvalidDates},
		{"inverted dates", func(y *schoolyears.SchoolYear) {
			y.StartDate, y.EndDate = y.EndDate, y.StartDate
		}, schoolyears.ErrInvalidDates},
		{"holiday out of range", func(y *schoolyears.SchoolYear) {
			y.Holidays = []time.Time{date(2027, 1, 1)}
		}, schoolyears.ErrInvalidHoliday},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y := validYear()
			tc.mutate(&y)
			if _, err := svc.Create(ctx, y); !errors.Is(err, tc.want) {
				t.Errorf("Create error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestHolidaysNormalized(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	y := validYear()
	// Out of order, duplicated, with hour components.
	y.Holidays = []time.Time{
		time.Date(2026, 7, 20, 15, 30, 0, 0, time.FixedZone("COT", -5*3600)),
		date(2026, 5, 1),
		time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC),
	}
	got, err := svc.Create(ctx, y)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := []time.Time{date(2026, 5, 1), date(2026, 7, 20)}
	if len(got.Holidays) != len(want) {
		t.Fatalf("Holidays = %v, want %v", got.Holidays, want)
	}
	for i := range want {
		if !got.Holidays[i].Equal(want[i]) {
			t.Errorf("Holidays[%d] = %v, want %v", i, got.Holidays[i], want[i])
		}
	}
}

func TestDuplicateYearAndCRUD(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	a, err := svc.Create(ctx, validYear())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.ID == "" {
		t.Error("Create did not assign an id")
	}
	if _, err := svc.Create(ctx, validYear()); !errors.Is(err, schoolyears.ErrDuplicateYear) {
		t.Errorf("duplicate year error = %v, want ErrDuplicateYear", err)
	}

	got, err := svc.ByYear(ctx, 2026)
	if err != nil {
		t.Fatalf("ByYear: %v", err)
	}
	if got.ID != a.ID {
		t.Errorf("ByYear = %+v, want id %q", got, a.ID)
	}
	if _, err := svc.ByYear(ctx, 2030); !errors.Is(err, schoolyears.ErrNotFound) {
		t.Errorf("ByYear unknown error = %v, want ErrNotFound", err)
	}

	// Custom period counts are allowed.
	y := validYear()
	y.Year, y.Periods = 2027, 4
	b, err := svc.Create(ctx, y)
	if err != nil {
		t.Fatalf("Create 4-period year: %v", err)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].Year != 2027 || list[1].Year != 2026 {
		t.Errorf("List = %+v, want newest first", list)
	}

	// Closing a year is an update.
	b.Closed = true
	upd, err := svc.Update(ctx, b)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !upd.Closed {
		t.Errorf("Update = %+v, want closed", upd)
	}

	if err := svc.Delete(ctx, a.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := svc.ByID(ctx, a.ID); !errors.Is(err, schoolyears.ErrNotFound) {
		t.Errorf("ByID after delete error = %v, want ErrNotFound", err)
	}
}

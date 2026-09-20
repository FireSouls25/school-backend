package schoolyears_test

import (
	"testing"
	"time"

	"grade/src/core/schoolyears"
)

func holiday(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// TestColombianHolidays2026 pins the festivos list against the official
// 2026 calendar: 18 holidays with movable feasts on their Emiliani Monday.
func TestColombianHolidays2026(t *testing.T) {
	got := schoolyears.ColombianHolidays(2026)
	if len(got) != 18 {
		t.Fatalf("len(holidays) = %d, want 18", len(got))
	}
	for i := 1; i < len(got); i++ {
		if !got[i-1].Before(got[i]) {
			t.Fatalf("holidays not sorted/unique: %v", got)
		}
	}
	for _, want := range []time.Time{
		holiday(2026, time.January, 1),
		holiday(2026, time.January, 12), // Reyes (Tue 6 -> Mon 12)
		holiday(2026, time.March, 23),   // San José (Thu 19 -> Mon 23)
		holiday(2026, time.April, 2),    // Jueves Santo (stays)
		holiday(2026, time.April, 3),    // Viernes Santo (stays)
		holiday(2026, time.May, 1),
		holiday(2026, time.May, 18),  // Ascensión (Thu 14 -> Mon 18)
		holiday(2026, time.June, 8),  // Corpus (Thu 4 -> Mon 8)
		holiday(2026, time.June, 15), // Sagrado Corazón (Fri 12 -> Mon 15)
		holiday(2026, time.June, 29), // San Pedro (already Monday)
		holiday(2026, time.July, 20),
		holiday(2026, time.August, 7),
		holiday(2026, time.August, 17),   // Asunción (Sat 15 -> Mon 17)
		holiday(2026, time.October, 12),  // Raza (already Monday)
		holiday(2026, time.November, 2),  // Todos los Santos (Sun 1 -> Mon 2)
		holiday(2026, time.November, 16), // Cartagena (Wed 11 -> Mon 16)
		holiday(2026, time.December, 8),
		holiday(2026, time.December, 25),
	} {
		found := false
		for _, h := range got {
			if h.Equal(want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("holiday %v missing from %v", want.Format("2006-01-02"), got)
		}
	}
}

func TestIsSchoolDay(t *testing.T) {
	y := schoolyears.SchoolYear{
		StartDate: holiday(2026, time.February, 2),
		EndDate:   holiday(2026, time.November, 27),
		Holidays:  []time.Time{holiday(2026, time.May, 1)},
	}
	for _, tc := range []struct {
		date time.Time
		want bool
	}{
		{holiday(2026, time.March, 3), true},     // regular Tuesday
		{holiday(2026, time.March, 7), true},     // Saturday counts: entries decide
		{holiday(2026, time.May, 1), false},      // holiday
		{holiday(2026, time.January, 15), false}, // before range
		{holiday(2026, time.December, 1), false}, // after range
	} {
		if got := y.IsSchoolDay(tc.date); got != tc.want {
			t.Errorf("IsSchoolDay(%v) = %v, want %v", tc.date.Format("2006-01-02"), got, tc.want)
		}
	}
}

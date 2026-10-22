// Package schoolyears owns the academic years (años lectivos): the year
// number, how many periods it has, its date range, Colombian holidays and
// whether it is closed. Class-groups, enrollments and schedules reference
// a year by its id, so every record can be filtered per year.
package schoolyears

import (
	"sort"
	"time"
)

// MinYear and MaxYear bound sane academic years.
const MinYear = 2000

const MaxYear = 2100

// MinPeriods and MaxPeriods bound the customizable period count.
// Schools usually run 3 periods; more are allowed up to MaxPeriods.
const MinPeriods = 1

const MaxPeriods = 6

// PeriodRange bounds one academic period (evaluative term): its start
// and end dates, date-only UTC. Ranges must sit inside the school year,
// run in order and never overlap; exact borders belong to a single
// period, so the next range must start strictly after the previous end.
type PeriodRange struct {
	// Start is the first day of the period.
	Start time.Time
	// End is the last day of the period.
	End time.Time
}

// SchoolYear is one academic year.
type SchoolYear struct {
	// ID is the unique, immutable UUID of the school year.
	ID string
	// Year is the calendar year (e.g. 2026). Unique.
	Year int
	// Periods is how many academic periods the year has.
	Periods int
	// PeriodRanges dates each period. Empty means a legacy year without
	// dated periods; new years always carry one range per period.
	PeriodRanges []PeriodRange
	// StartDate is the first school day.
	StartDate time.Time
	// EndDate is the last school day.
	EndDate time.Time
	// Holidays lists non-school days (festivos) within the range,
	// date-only, sorted, deduplicated.
	Holidays []time.Time
	// NoClassDays lists manually marked days without classes (locally
	// moved holidays, emergencies): non-school days that are not
	// official festivos. Date-only, sorted, deduplicated; may overlap
	// Holidays (both badges show).
	NoClassDays []time.Time
	// Closed reports whether the year is finished. Closed years accept
	// no new enrollments, schedules or sessions; promotion closes a year.
	Closed bool
}

// normalizeHolidays truncates every holiday to its date, drops
// out-of-range entries is left to validate, sorts and deduplicates.
func normalizeHolidays(in []time.Time) []time.Time {
	seen := make(map[time.Time]struct{}, len(in))
	out := make([]time.Time, 0, len(in))
	for _, h := range in {
		d := time.Date(h.Year(), h.Month(), h.Day(), 0, 0, 0, 0, time.UTC)
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// containsDate reports whether d (date-only) falls within [start, end].
func containsDate(d, start, end time.Time) bool {
	return !d.Before(start) && !d.After(end)
}

// normalizeRanges truncates every range to date-only UTC.
func normalizeRanges(in []PeriodRange) []PeriodRange {
	out := make([]PeriodRange, 0, len(in))
	for _, r := range in {
		out = append(out, PeriodRange{
			Start: toDate(r.Start),
			End:   toDate(r.End),
		})
	}
	return out
}

// toDate truncates t to UTC midnight, preserving zero.
func toDate(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// validateRanges checks dated periods against the year range: one range
// per period, each inside [start, end], ordered and non-overlapping.
func validateRanges(ranges []PeriodRange, periods int, start, end time.Time) error {
	if len(ranges) != periods {
		return ErrInvalidPeriodRanges
	}
	for i, r := range ranges {
		if r.Start.IsZero() || r.End.IsZero() || !r.Start.Before(r.End) {
			return ErrInvalidPeriodRanges
		}
		if !containsDate(r.Start, start, end) || !containsDate(r.End, start, end) {
			return ErrInvalidPeriodRanges
		}
		if i > 0 && !r.Start.After(ranges[i-1].End) {
			return ErrInvalidPeriodRanges
		}
	}
	return nil
}

// DayKind reports whether t is an official holiday and/or a manually
// marked no-class day. Both can hold at once (locally moved festivos).
func (y SchoolYear) DayKind(t time.Time) (holiday, noClass bool) {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	for _, h := range y.Holidays {
		if d.Equal(h) {
			holiday = true
			break
		}
	}
	for _, h := range y.NoClassDays {
		if d.Equal(h) {
			noClass = true
			break
		}
	}
	return holiday, noClass
}

// IsSchoolDay reports whether t is a school day in the year: inside the
// date range and neither a holiday nor a marked no-class day. Weekday
// filtering composes on top using schedule entries, since some schools
// teach on Saturdays. Explicit roll calls stay allowed on non-school
// days: the call is deliberate, so it records and counts.
func (y SchoolYear) IsSchoolDay(t time.Time) bool {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if y.StartDate.IsZero() || y.EndDate.IsZero() || !containsDate(d, y.StartDate, y.EndDate) {
		return false
	}
	holiday, noClass := y.DayKind(d)
	return !holiday && !noClass
}

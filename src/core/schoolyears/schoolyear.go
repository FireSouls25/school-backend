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

// SchoolYear is one academic year.
type SchoolYear struct {
	// ID is the unique, immutable UUID of the school year.
	ID string
	// Year is the calendar year (e.g. 2026). Unique.
	Year int
	// Periods is how many academic periods the year has.
	Periods int
	// StartDate is the first school day.
	StartDate time.Time
	// EndDate is the last school day.
	EndDate time.Time
	// Holidays lists non-school days (festivos) within the range,
	// date-only, sorted, deduplicated.
	Holidays []time.Time
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

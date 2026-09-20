package schoolyears

import (
	"sort"
	"time"
)

// ColombianHolidays returns the year's public holidays (festivos) under
// Ley 51 de 1983 (Emiliani): fixed feasts stay on their date, movable ones
// shift to the next Monday unless already Monday, while Maundy Thursday
// and Good Friday always stay put. The result seeds SchoolYear.Holidays;
// administrators remain free to edit the list.
func ColombianHolidays(year int) []time.Time {
	easter := easterSunday(year)
	at := func(month time.Month, day int) time.Time {
		return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	}
	// Fixed feasts.
	out := []time.Time{
		at(time.January, 1),
		at(time.May, 1),
		at(time.July, 20),
		at(time.August, 7),
		at(time.December, 8),
		at(time.December, 25),
	}
	// Holy week stays put.
	out = append(out,
		easter.AddDate(0, 0, -3), // Jueves Santo
		easter.AddDate(0, 0, -2), // Viernes Santo
	)
	// Movable feasts shift to the next Monday.
	movable := []time.Time{
		at(time.January, 6),      // Reyes Magos
		at(time.March, 19),       // San José
		easter.AddDate(0, 0, 39), // Ascensión
		easter.AddDate(0, 0, 60), // Corpus Christi
		easter.AddDate(0, 0, 68), // Sagrado Corazón
		at(time.June, 29),        // San Pedro y San Pablo
		at(time.August, 15),      // Asunción
		at(time.October, 12),     // Día de la Raza
		at(time.November, 1),     // Todos los Santos
		at(time.November, 11),    // Independencia de Cartagena
	}
	for _, m := range movable {
		out = append(out, nextMonday(m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	// Deduplicate in case two feasts ever coincide.
	deduped := out[:0]
	for i, d := range out {
		if i == 0 || !d.Equal(out[i-1]) {
			deduped = append(deduped, d)
		}
	}
	return deduped
}

// nextMonday shifts d to the next Monday, keeping Mondays in place.
func nextMonday(d time.Time) time.Time {
	return d.AddDate(0, 0, (8-int(d.Weekday()))%7)
}

// easterSunday computes Western Easter with the Anonymous Gregorian
// algorithm (Meeus/Jones/Butcher), valid for the supported year range.
func easterSunday(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := ((h + l - 7*m + 114) % 31) + 1
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

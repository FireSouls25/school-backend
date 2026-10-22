package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"grade/src/core/schoolyears"
)

// periodRangeRequest dates one period. Dates travel as RFC3339 (or
// date-only, parsed as UTC midnight by the frontend convention below).
type periodRangeRequest struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// yearRequest is the body for creating or replacing a school year.
// New years always carry one range per period; the service keeps legacy
// rows without ranges readable.
type yearRequest struct {
	Year         int                  `json:"year"`
	Periods      int                  `json:"periods"`
	StartDate    time.Time            `json:"startDate"`
	EndDate      time.Time            `json:"endDate"`
	Holidays     []time.Time          `json:"holidays"`
	PeriodRanges []periodRangeRequest `json:"periodRanges"`
}

func (b yearRequest) toYear(id string) schoolyears.SchoolYear {
	ranges := make([]schoolyears.PeriodRange, 0, len(b.PeriodRanges))
	for _, r := range b.PeriodRanges {
		ranges = append(ranges, schoolyears.PeriodRange{Start: r.Start, End: r.End})
	}
	return schoolyears.SchoolYear{
		ID:           id,
		Year:         b.Year,
		Periods:      b.Periods,
		StartDate:    b.StartDate,
		EndDate:      b.EndDate,
		Holidays:     append([]time.Time(nil), b.Holidays...),
		PeriodRanges: ranges,
	}
}

// handleListYears returns every school year with its dated periods,
// newest first. Teachers read it to resolve the current period.
func handleListYears(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, err := deps.Years.List(r.Context())
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if list == nil {
			list = []schoolyears.SchoolYear{}
		}
		writeJSON(w, http.StatusOK, list)
	}
}

// handleGetYear returns one school year with its dated periods.
func handleGetYear(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		y, err := deps.Years.ByID(r.Context(), strings.TrimSpace(chi.URLParam(r, "id")))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, y)
	}
}

// handleCreateYear creates a school year. New years always date their
// periods: an empty periodRanges fails with invalid_period_ranges.
func handleCreateYear(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body yearRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		if len(body.PeriodRanges) == 0 {
			WriteError(w, r, schoolyears.ErrInvalidPeriodRanges)
			return
		}
		y, err := deps.Years.Create(r.Context(), body.toYear(""))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, y)
	}
}

// frozenFields reports whether protected calendar fields changed: once a
// year grew class-groups, periods, ranges and dates are frozen and only
// holidays still move (until close).
func frozenFields(a, b schoolyears.SchoolYear) bool {
	if a.Year != b.Year || a.Periods != b.Periods {
		return true
	}
	if !a.StartDate.Equal(b.StartDate) || !a.EndDate.Equal(b.EndDate) {
		return true
	}
	if len(a.PeriodRanges) != len(b.PeriodRanges) {
		return true
	}
	for i := range a.PeriodRanges {
		if !a.PeriodRanges[i].Start.Equal(b.PeriodRanges[i].Start) ||
			!a.PeriodRanges[i].End.Equal(b.PeriodRanges[i].End) {
			return true
		}
	}
	return false
}

// handleUpdateYear replaces a school year. Started years (with salons)
// keep periods and dates frozen; closed years accept no edits at all.
func handleUpdateYear(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body yearRequest
		if !decodeJSON(w, r, &body) {
			return
		}
		id := strings.TrimSpace(chi.URLParam(r, "id"))
		stored, err := deps.Years.ByID(r.Context(), id)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		if stored.Closed {
			WriteError(w, r, schoolyears.ErrYearClosed)
			return
		}
		next := body.toYear(id)
		next.Closed = stored.Closed
		if deps.Classes != nil {
			groups, err := deps.Classes.ListByYear(r.Context(), id)
			if err != nil {
				WriteError(w, r, err)
				return
			}
			if len(groups) > 0 && frozenFields(stored, next) {
				WriteError(w, r, schoolyears.ErrYearStarted)
				return
			}
		}
		updated, err := deps.Years.Update(r.Context(), next)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, updated)
	}
}

// handleCloseYear finishes a school year: no enrollments afterwards and
// no edits at all. Closing twice is a no-op returning the stored year.
func handleCloseYear(deps Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		y, err := deps.Years.Close(r.Context(), strings.TrimSpace(chi.URLParam(r, "id")))
		if err != nil {
			WriteError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, y)
	}
}

// yearQueryYear parses ?year=AAAA for group-scoped routes.
func yearQueryYear(r *http.Request) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("year"))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < schoolyears.MinYear || n > schoolyears.MaxYear {
		return 0, false
	}
	return n, true
}

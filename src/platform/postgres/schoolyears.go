package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"grade/src/core/schoolyears"
)

// SchoolYearsStore implements schoolyears.Store on top of PostgreSQL.
type SchoolYearsStore struct {
	db *DB
}

// NewSchoolYearsStore returns a schoolyears.Store backed by db.
func NewSchoolYearsStore(db *DB) *SchoolYearsStore { return &SchoolYearsStore{db: db} }

// Compile-time check that the adapter satisfies the port.
var _ schoolyears.Store = (*SchoolYearsStore)(nil)

// schoolYearColumns is the full column list used by every year SELECT.
const schoolYearColumns = `id, year, periods, start_date, end_date, holidays, no_class_days, closed`

// scanSchoolYear maps the current row (in schoolYearColumns order) onto
// a SchoolYear.
func scanSchoolYear(scan func(dest ...any) error) (schoolyears.SchoolYear, error) {
	var y schoolyears.SchoolYear
	var start, end time.Time
	var holidays, noClass []time.Time
	err := scan(&y.ID, &y.Year, &y.Periods, &start, &end, &holidays, &noClass, &y.Closed)
	if err != nil {
		return schoolyears.SchoolYear{}, err
	}
	y.StartDate = start
	y.EndDate = end
	y.Holidays = append([]time.Time(nil), holidays...)
	y.NoClassDays = append([]time.Time(nil), noClass...)
	return y, nil
}

// schoolYearArgs flattens y into the INSERT/UPDATE argument order.
func schoolYearArgs(y schoolyears.SchoolYear) []any {
	holidays := append([]time.Time(nil), y.Holidays...)
	if holidays == nil {
		holidays = []time.Time{}
	}
	noClass := append([]time.Time(nil), y.NoClassDays...)
	if noClass == nil {
		noClass = []time.Time{}
	}
	return []any{
		y.ID, y.Year, y.Periods,
		y.StartDate.Format("2006-01-02"), y.EndDate.Format("2006-01-02"),
		holidays, noClass, y.Closed,
	}
}

// saveRanges replaces every dated period of a year inside tx.
func saveRanges(ctx context.Context, tx pgx.Tx, y schoolyears.SchoolYear) error {
	if _, err := tx.Exec(ctx, `DELETE FROM school_year_periods WHERE year_id = $1`, y.ID); err != nil {
		return fmt.Errorf("postgres: replace school year periods: %w", err)
	}
	for i, r := range y.PeriodRanges {
		if _, err := tx.Exec(ctx, `
			INSERT INTO school_year_periods (year_id, n, start_date, end_date)
			VALUES ($1, $2, $3, $4)`,
			y.ID, i+1, r.Start.Format("2006-01-02"), r.End.Format("2006-01-02")); err != nil {
			return fmt.Errorf("postgres: save school year period: %w", err)
		}
	}
	return nil
}

// loadRanges attaches dated periods (ordered) to y.
func (s *SchoolYearsStore) loadRanges(ctx context.Context, y *schoolyears.SchoolYear) error {
	rows, err := s.db.pool.Query(ctx, `
		SELECT start_date, end_date FROM school_year_periods
		WHERE year_id = $1 ORDER BY n`, y.ID)
	if err != nil {
		return fmt.Errorf("postgres: list school year periods: %w", err)
	}
	defer rows.Close()
	ranges := make([]schoolyears.PeriodRange, 0)
	for rows.Next() {
		var start, end time.Time
		if err := rows.Scan(&start, &end); err != nil {
			return fmt.Errorf("postgres: scan school year period: %w", err)
		}
		ranges = append(ranges, schoolyears.PeriodRange{Start: start, End: end})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("postgres: list school year periods: %w", err)
	}
	y.PeriodRanges = ranges
	return nil
}

// Create implements schoolyears.Store. Year and periods persist atomically.
func (s *SchoolYearsStore) Create(ctx context.Context, y schoolyears.SchoolYear) (schoolyears.SchoolYear, error) {
	tx, err := s.db.pool.Begin(ctx)
	if err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: begin school year: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		INSERT INTO school_years (id, year, periods, start_date, end_date, holidays, no_class_days, closed)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING `+schoolYearColumns,
		schoolYearArgs(y)...)
	out, err := scanSchoolYear(row.Scan)
	if err != nil {
		if isUniqueViolationOn(err, "school_years") {
			return schoolyears.SchoolYear{}, schoolyears.ErrDuplicateYear
		}
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: create school year: %w", err)
	}
	if err := saveRanges(ctx, tx, y); err != nil {
		return schoolyears.SchoolYear{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: commit school year: %w", err)
	}
	out.PeriodRanges = append([]schoolyears.PeriodRange(nil), y.PeriodRanges...)
	return out, nil
}

// ByID implements schoolyears.Store.
func (s *SchoolYearsStore) ByID(ctx context.Context, id string) (schoolyears.SchoolYear, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+schoolYearColumns+` FROM school_years WHERE id = $1`, id)
	out, err := scanSchoolYear(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return schoolyears.SchoolYear{}, schoolyears.ErrNotFound
	}
	if err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: get school year: %w", err)
	}
	if err := s.loadRanges(ctx, &out); err != nil {
		return schoolyears.SchoolYear{}, err
	}
	return out, nil
}

// ByYear implements schoolyears.Store.
func (s *SchoolYearsStore) ByYear(ctx context.Context, year int) (schoolyears.SchoolYear, error) {
	row := s.db.pool.QueryRow(ctx,
		`SELECT `+schoolYearColumns+` FROM school_years WHERE year = $1`, year)
	out, err := scanSchoolYear(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return schoolyears.SchoolYear{}, schoolyears.ErrNotFound
	}
	if err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: get school year: %w", err)
	}
	if err := s.loadRanges(ctx, &out); err != nil {
		return schoolyears.SchoolYear{}, err
	}
	return out, nil
}

// List implements schoolyears.Store.
func (s *SchoolYearsStore) List(ctx context.Context) ([]schoolyears.SchoolYear, error) {
	rows, err := s.db.pool.Query(ctx, `
		SELECT `+schoolYearColumns+`
		FROM school_years
		ORDER BY year DESC`)
	if err != nil {
		return nil, fmt.Errorf("postgres: list school years: %w", err)
	}
	defer rows.Close()

	out := make([]schoolyears.SchoolYear, 0)
	for rows.Next() {
		y, err := scanSchoolYear(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("postgres: scan school year: %w", err)
		}
		out = append(out, y)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if err := s.loadRanges(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Update implements schoolyears.Store. Year and periods persist atomically.
func (s *SchoolYearsStore) Update(ctx context.Context, y schoolyears.SchoolYear) (schoolyears.SchoolYear, error) {
	tx, err := s.db.pool.Begin(ctx)
	if err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: begin school year: %w", err)
	}
	defer tx.Rollback(ctx)
	row := tx.QueryRow(ctx, `
		UPDATE school_years
		SET year = $2, periods = $3, start_date = $4, end_date = $5,
			holidays = $6, no_class_days = $7, closed = $8, updated_at = now()
		WHERE id = $1
		RETURNING `+schoolYearColumns,
		schoolYearArgs(y)...)
	out, err := scanSchoolYear(row.Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return schoolyears.SchoolYear{}, schoolyears.ErrNotFound
	}
	if err != nil {
		if isUniqueViolationOn(err, "school_years") {
			return schoolyears.SchoolYear{}, schoolyears.ErrDuplicateYear
		}
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: update school year: %w", err)
	}
	if err := saveRanges(ctx, tx, y); err != nil {
		return schoolyears.SchoolYear{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: commit school year: %w", err)
	}
	out.PeriodRanges = append([]schoolyears.PeriodRange(nil), y.PeriodRanges...)
	return out, nil
}

// Delete implements schoolyears.Store. Years with class-groups are
// protected by the database.
func (s *SchoolYearsStore) Delete(ctx context.Context, id string) error {
	if _, err := s.db.pool.Exec(ctx, `DELETE FROM school_years WHERE id = $1`, id); err != nil {
		if isForeignKeyViolationOn(err, "class_groups") {
			return schoolyears.ErrHasClasses
		}
		return fmt.Errorf("postgres: delete school year: %w", err)
	}
	return nil
}

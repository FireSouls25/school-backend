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
const schoolYearColumns = `id, year, periods, start_date, end_date, holidays, closed`

// scanSchoolYear maps the current row (in schoolYearColumns order) onto
// a SchoolYear.
func scanSchoolYear(scan func(dest ...any) error) (schoolyears.SchoolYear, error) {
	var y schoolyears.SchoolYear
	var start, end time.Time
	var holidays []time.Time
	err := scan(&y.ID, &y.Year, &y.Periods, &start, &end, &holidays, &y.Closed)
	if err != nil {
		return schoolyears.SchoolYear{}, err
	}
	y.StartDate = start
	y.EndDate = end
	y.Holidays = append([]time.Time(nil), holidays...)
	return y, nil
}

// schoolYearArgs flattens y into the INSERT/UPDATE argument order.
func schoolYearArgs(y schoolyears.SchoolYear) []any {
	holidays := append([]time.Time(nil), y.Holidays...)
	if holidays == nil {
		holidays = []time.Time{}
	}
	return []any{
		y.ID, y.Year, y.Periods,
		y.StartDate.Format("2006-01-02"), y.EndDate.Format("2006-01-02"),
		holidays, y.Closed,
	}
}

// Create implements schoolyears.Store.
func (s *SchoolYearsStore) Create(ctx context.Context, y schoolyears.SchoolYear) (schoolyears.SchoolYear, error) {
	row := s.db.pool.QueryRow(ctx, `
		INSERT INTO school_years (id, year, periods, start_date, end_date, holidays, closed)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING `+schoolYearColumns,
		schoolYearArgs(y)...)
	out, err := scanSchoolYear(row.Scan)
	if err != nil {
		if isUniqueViolationOn(err, "school_years") {
			return schoolyears.SchoolYear{}, schoolyears.ErrDuplicateYear
		}
		return schoolyears.SchoolYear{}, fmt.Errorf("postgres: create school year: %w", err)
	}
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
	return out, rows.Err()
}

// Update implements schoolyears.Store.
func (s *SchoolYearsStore) Update(ctx context.Context, y schoolyears.SchoolYear) (schoolyears.SchoolYear, error) {
	row := s.db.pool.QueryRow(ctx, `
		UPDATE school_years
		SET year = $2, periods = $3, start_date = $4, end_date = $5,
			holidays = $6, closed = $7, updated_at = now()
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

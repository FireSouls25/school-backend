# School years, class-groups, enrollments and schedules

This document covers four related capabilities: `schoolyears` (años
lectivos), `classes` (salones por año), `enrollments` (matrículas y
auditoría de promociones) and `schedules` (horarios semanales), plus their
PostgreSQL persistence. Sessions and the promotion flow build on these
records in later phases.

## Overview

- **SchoolYear**: one academic year. Calendar year (unique), customizable
  period count (1–6, usually 3), start/end dates, Colombian holidays
  (date-only, sorted, deduplicated, all within range) and a `Closed` flag.
  Closing a year freezes it: no new enrollments, schedules or sessions.
- **ClassGroup**: one classroom within one year: grade (1–11) + group
  number. The display label (`Label()`, e.g. `"7-1"`) derives from both,
  never stored. Groups are per-year rows: `7-1/2026` and `7-1/2025` are
  different records, so adding a group means creating it and removing one
  (last year up to `6-5`, this year only to `6-4`) means simply not
  creating it.
- **Enrollment**: one student placed in one class-group. The roster of a
  group is its enrollments; alphabetical ordering is composed upstream by
  joining student profiles.
- **Promotion**: the audited decision moving a student between years
  (`promote` / `repeat` / `graduate`, who decided, when). Graduating
  students carry an empty destination: end of the lifecycle. Records are
  append-only.
- **Festivos**: `ColombianHolidays(year)` generates the year's public
  holidays under Ley 51 de 1983 (fixed feasts stay, movable ones shift to
  the next Monday, Maundy Thursday and Good Friday stay put). It seeds
  `SchoolYear.Holidays`; administrators remain free to edit the list.
  `IsSchoolDay(t)` reports whether a date is inside the range and not a
  holiday (weekday filtering composes on top, since some schools teach on
  Saturdays).

Every historical record keeps working per year because groups belong to
exactly one year: filtering by year is a join away, and per-student
histories span years through the student id.

## Layout

```
src/core/schoolyears/  years: model, Store port, Service, MemoryStore
src/core/classes/      groups: model, Store port, Service, MemoryStore
src/core/enrollments/  placements + promotions: model, Store port, Service, MemoryStore
src/platform/postgres/ production adapters + embedded schema.sql
src/tests/schoolyears|classes|enrollments|postgres/
```

## Ports

Each capability owns its persistence interface; adapters are injected from
the composition root.

```go
// schoolyears.Store
Create(ctx, SchoolYear) (SchoolYear, error)
ByID(ctx, id string) (SchoolYear, error)         // ErrNotFound when missing
ByYear(ctx, year int) (SchoolYear, error)        // ErrNotFound when missing
List(ctx) ([]SchoolYear, error)                  // newest year first
Update(ctx, SchoolYear) (SchoolYear, error)
Delete(ctx, id string) error                     // no-op when absent; ErrHasClasses if grouped

// classes.Store
Create(ctx, ClassGroup) (ClassGroup, error)
ByID(ctx, id string) (ClassGroup, error)         // ErrNotFound when missing
ListByYear(ctx, schoolYearID) ([]ClassGroup, error) // by grade, then group
Update(ctx, ClassGroup) (ClassGroup, error)
Delete(ctx, id string) error                     // no-op when absent; ErrHasEnrollments if enrolled

// enrollments.Store
AddEnrollment(ctx, Enrollment) (Enrollment, error)
EnrollmentsForGroup(ctx, classGroupID) ([]Enrollment, error)   // enrollment order
EnrollmentsForStudent(ctx, studentID) ([]Enrollment, error)    // enrollment order
RemoveEnrollment(ctx, id string) error           // no-op when absent, corrections only
RecordPromotion(ctx, Promotion) (Promotion, error) // append-only
PromotionsForStudent(ctx, studentID) ([]Promotion, error)      // oldest first

// schedules.Store
Create(ctx, Entry) (Entry, error)
ByID(ctx, id string) (Entry, error)                 // ErrNotFound when missing
EntriesForTeacher(ctx, teacherID) ([]Entry, error) // by weekday, then start
EntriesForGroup(ctx, classGroupID) ([]Entry, error) // the weekly schedule
Update(ctx, Entry) (Entry, error)
Delete(ctx, id string) error                       // no-op when absent
```

`Service` types wrap each store with validation and UUID assignment
(`google/uuid`). Domain errors implement the i18n `Coded` contract, e.g.
`classes.ErrNotFound` → catalog key `classes.err_not_found`.

### Cross-capability protection without imports

Services never import another capability. Protection rules that span
packages rely on the database instead, translated to coded errors by the
adapters (`src/platform/postgres/pgerrors.go`):

- Deleting a year with groups → `RESTRICT` → `schoolyears.ErrHasClasses`.
- Deleting a group with enrollments → `RESTRICT` → `classes.ErrHasEnrollments`.
- Enrolling an unknown student/group → FK violation → `enrollments.ErrInvalidStudent` / `ErrInvalidClassGroup`.
- Duplicate year / group / enrollment → unique violation → `ErrDuplicateYear` / `ErrDuplicateClass` / `ErrDuplicateEnrollment` (race-safe behind the service-level checks, which keep memory stores consistent).

Within one package the service checks first (unique names, duplicate
pairs), so both adapters behave alike.

### Schedules: manual placement with guarded pairing

`Entry{ClassGroupID, TeacherID, SubjectID, Weekday 0–6, Start, End}` repeats
every week; administrators place each slot by hand, reusing the teachers'
existing subject assignments so new years never start from zero. The
package generates nothing by itself. Rules enforced by
`schedules.Service`:

- Teacher and group conflicts: no overlapping `[Start, End)` slots for the
  same teacher or the same group on one weekday (`ErrTeacherConflict` /
  `ErrClassConflict`); touching edges are allowed.
- Pairing: the teacher must currently teach the subject
  (`ErrNotTeachingSubject`), verified through the `Checker` port that the
  composition root implements on top of `subjects` (`CurrentForTeacher`).
  Schedules stays decoupled from subjects.
- Times are `Clock` (minutes since midnight, `"07:30"`); weekdays follow
  `time.Weekday` numbering (Sunday = 0).
- Read models: `EntriesForTeacherOnDay` (the teacher's day view; holiday
  skipping composes on top via `SchoolYear.IsSchoolDay`) and
  `EntriesForGroup` (the salon's weekly schedule, e.g. 7-1/2026).

## Database schema

`src/platform/postgres/schema.sql` is embedded and applied idempotently at
connect time (`CREATE TABLE IF NOT EXISTS`, `CREATE UNIQUE INDEX IF NOT
EXISTS`).

```
school_years(id uuid PK, year UNIQUE, periods CHECK 1..6,
             start_date DATE NOT NULL, end_date DATE NOT NULL CHECK start < end,
             holidays DATE[] NOT NULL DEFAULT '{}', closed,
             created_at, updated_at)
class_groups(id uuid PK,
             school_year_id uuid FK->school_years ON DELETE RESTRICT,
             grade CHECK 1..11, group_no CHECK >= 1,
             UNIQUE (school_year_id, grade, group_no), created_at, updated_at)
enrollments(id uuid PK,
            student_id uuid FK->students ON DELETE CASCADE,
            class_group_id uuid FK->class_groups ON DELETE RESTRICT,
            UNIQUE (student_id, class_group_id), created_at)
promotions(id uuid PK,
           student_id uuid FK->students ON DELETE CASCADE,
           from_group_id uuid FK->class_groups ON DELETE RESTRICT,
           to_group_id uuid NULL FK->class_groups ON DELETE RESTRICT,
           decision CHECK IN ('promote','repeat','graduate'),
           decided_by, decided_at, created_at)
schedule_entries(id uuid PK,
                 class_group_id uuid FK->class_groups ON DELETE CASCADE,
                 teacher_id uuid FK->teachers ON DELETE CASCADE,
                 subject_id uuid FK->subjects ON DELETE CASCADE,
                 weekday CHECK 0..6, start_min, end_min CHECK start < end,
                 created_at, updated_at)
```

Deleting a student cascades its enrollments and promotions (consistent
with attendance/fault/warning histories). Schedule entries cascade with
their group, teacher or subject: plans are derived data, rebuilt by
administrators. Groups and years survive: they are shared records, deleted
explicitly once empty.

## Configuration

- `DATABASE_URL`: PostgreSQL connection string. When set, the composition
  root wires the pgx-backed stores; when empty it falls back to in-memory
  stores (development only, logged as a warning).
- `TEST_DATABASE_URL`: used only by integration tests
  (`src/tests/postgres`). When unset those tests skip.

## Error codes and HTTP mapping

| Code | HTTP |
|---|---|
| `schoolyears.err_not_found` | 404 |
| `schoolyears.err_invalid_id` / `err_invalid_year` / `err_invalid_periods` / `err_invalid_dates` / `err_invalid_holiday` / `err_duplicate_year` / `err_has_classes` | 400 |
| `classes.err_not_found` | 404 |
| `classes.err_invalid_id` / `err_invalid_school_year` / `err_invalid_grade` / `err_invalid_group` / `err_duplicate_class` / `err_has_enrollments` | 400 |
| `enrollments.err_invalid_id` / `err_invalid_student` / `err_invalid_class_group` / `err_duplicate_enrollment` / `err_invalid_decision` / `err_invalid_promotion` / `err_invalid_date` / `err_invalid_actor` | 400 |
| `schedules.err_not_found` | 404 |
| `schedules.err_invalid_id` / `err_invalid_class_group` / `err_invalid_teacher` / `err_invalid_subject` / `err_invalid_weekday` / `err_invalid_time` / `err_teacher_conflict` / `err_class_conflict` / `err_not_teaching_subject` | 400 |

All messages are localized through `src/platform/i18n/catalogs/es.json`
(`enrollment.decision.*` holds the display names Promovido/Repite/Graduado).

## What's next (later phases)

- **Sessions**: attendance calls reference `ClassGroupID` and freeze the
  roster from its enrollments + student profiles.
- **Promotion flow**: suggestions + per-student destination choice write
  next-year enrollments, update `Student.ClassID` and record `Promotion`
  rows; graduating from grade 11 marks the student graduated.
- **Statistics**: aggregations over these records need no new tables.

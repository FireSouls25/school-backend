# Student profiles and histories

This document covers three related capabilities: `students`, `attendance`
and `incidents`, plus their PostgreSQL persistence.

## Overview

A student is identified by an immutable UUID generated at creation time.
The profile keeps:

- **Identity**: given names (`Names`) and family names (`Surnames`).
  `FullName()` renders surnames first, the convention used in Colombian
  school listings and the basis for alphabetical ordering.
- **Class assignment**: an opaque `ClassID` (e.g. `9-1`). The format is
  owned by the future `classes` capability; students treats it as a string.
- **Photo**: raw image bytes (JPEG/PNG), capped at `students.MaxPhotoSize`
  (2 MiB). Stored in Postgres as `BYTEA`.
- **Attendance history**: records typed as *inasistencia* (`absence`) or
  *evasión* (`evasion`), one per student per day per class-group.
- **Fault history**: misbehavior records graded *leve* (`minor`),
  *normal* (`ordinary`) or `grave` (`severe`) with a required description.

Histories live in their own capabilities and reference a student by UUID,
so a student's full record spans current **and** previous class-groups and
years. Deleting a student cascades to all their attendance and fault
records.

## Layout

```
src/core/students/     profile: model, Store port, Service, MemoryStore
src/core/attendance/   assistance: Reason enum, Record, Store port, Service
src/core/incidents/    faults: Severity enum, Fault, Store port, Service
src/platform/postgres/ production adapters + embedded schema.sql
src/tests/students|attendance|incidents|postgres/
```

## Ports

Each capability owns its persistence interface; adapters are injected from
the composition root.

```go
// students.Store
Create(ctx, Student) (Student, error)            // assigns nothing; id must be set
ByID(ctx, id string) (Student, error)            // ErrNotFound when missing
ListByClass(ctx, classID) ([]Student, error)     // ordered alphabetically
Update(ctx, Student) (Student, error)
Delete(ctx, id string) error                     // no-op when absent
PutPhoto(ctx, studentID, Photo) error            // ErrNotFound on unknown student
Photo(ctx, studentID) (Photo, error)

// attendance.Store
Add(ctx, Record) (Record, error)
ForStudent(ctx, studentID) ([]Record, error)     // newest first
Delete(ctx, id string) error

// incidents.Store
Add(ctx, Fault) (Fault, error)
ForStudent(ctx, studentID) ([]Fault, error)      // newest first
Delete(ctx, id string) error
```

`Service` types wrap each store with validation and UUID assignment
(`google/uuid`). Domain errors implement the i18n `Coded` contract, e.g.
`students.ErrNotFound` → catalog key `students.err_not_found`.

## Database schema

`src/platform/postgres/schema.sql` is embedded and applied idempotently at
connect time (`CREATE TABLE IF NOT EXISTS`).

```
students(id uuid PK, names text, surnames text, class_id text,
         photo bytea, created_at, updated_at)
attendance_records(id uuid PK, student_id uuid FK->students ON DELETE CASCADE,
                   class_id text, date date,
                   reason text CHECK IN ('absence','evasion'))
incident_faults(id uuid PK, student_id uuid FK->students ON DELETE CASCADE,
                class_id text, date date,
                severity text CHECK IN ('minor','ordinary','severe'),
                description text)
```

Indexes exist on every `student_id` and on `students.class_id`. Enum-like
values are stored as stable lowercase identifiers (English), never as
display text; Spanish labels come from the i18n catalog
(`reason.absence` → "Inasistencia", `severity.severe` → "Grave", ...).

## Configuration

- `DATABASE_URL`: PostgreSQL connection string. When set, the composition
  root wires the pgx-backed stores; when empty it falls back to in-memory
  stores (development only, logged as a warning).
- `TEST_DATABASE_URL`: used only by integration tests
  (`src/tests/postgres`). When unset those tests skip.

## Error codes and HTTP mapping

| Code | HTTP |
|---|---|
| `students.err_not_found` | 404 |
| `students.err_invalid_id` / `err_invalid_name` / `err_invalid_class` | 400 |
| `students.err_photo_too_large` | 413 |
| `attendance.err_unknown_reason` / `err_invalid_student` / `err_invalid_class` / `err_invalid_date` | 400 |
| `attendance.err_not_found` | 404 |
| `incidents.err_unknown_severity` / `err_invalid_student` / `err_invalid_class` / `err_invalid_date` / `err_empty_description` | 400 |
| `incidents.err_not_found` | 404 |

All messages are localized through `src/platform/i18n/catalogs/es.json`.

## Data protection notes (Ley 1581 de 2012)

- Photos of minors are personal data: access must be authorized (role
  system gates future endpoints) and the size cap limits abuse.
- Deletion of a student removes dependent histories (cascade), supporting
  data-minimization requests.
- Audit logging of reads/writes is still pending (see roadmap).

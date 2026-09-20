# Teachers and subjects

This document covers two related capabilities: `teachers` (perfil del
docente) and `subjects` (catálogo de materias y línea de tiempo de
asignaciones), plus their PostgreSQL persistence.

## Overview

A teacher is identified by an immutable UUID generated at creation time,
exactly like a student. The national identity document (`DocumentID`,
cédula) is a profile field on both, never the internal identifier. The
profile keeps:

- **Identity**: given names (`Names`), family names (`Surnames`).
  `FullName()` renders surnames first, the convention used in Colombian
  school listings and the basis for alphabetical ordering.
- **Document and contact**: identity document number (`DocumentID`),
  phone, address and email (validated when present).
- **Birth data**: place (`Birthplace`) and date (`Birthdate`) of birth.
  Age is never stored; `AgeAt(ref)` derives full years from the birthdate.
- **Health**: free-text medical conditions (`MedicalConditions`).
- **Homeroom**: `HomeroomClassID` names the class-group the teacher leads
  as director de grupo (e.g. `"9-1"`). Empty means the teacher leads no
  class-group; `IsHomeroomDirector()` reports which case applies.

What each teacher teaches is **not** a profile field: it lives in the
separate `subjects` capability so subject ids stay reusable by future
capabilities such as a calendar.

## Subjects and the assignment timeline

- **Subject**: one teachable matter (`Name`, e.g. Matemáticas, Ciencias
  Sociales), with optional `Code` (`MAT`) and `Description`, plus an
  `Active` flag. Names are unique, case-insensitively.
- **Assignment**: one timeline entry linking `TeacherID` to `SubjectID`
  over a date range (`StartedAt`, `EndedAt`). A zero `EndedAt` means the
  range is open: the teacher currently teaches that subject.

The timeline answers "who taught X, when" and preserves every period:

- Teaching matemática for 5 years, then switching to física, is recorded
  by ending the matemática assignment and starting a física one. Both
  periods stay on the teacher's timeline.
- Teaching física and informática at once is two overlapping open
  assignments. Informática counts only from its own start date until it
  is ended; física is unaffected.

Rules enforced by `subjects.Service`:

- `Assign` requires an existing subject and rejects a second open
  assignment of the same teacher+subject pair (`ErrAlreadyAssigned`).
  Different subjects may freely overlap.
- `EndAssignment` closes an open assignment; the end date cannot precede
  the start (`ErrInvalidEnd`), and re-closing fails (`ErrAlreadyEnded`).
- `DeleteSubject` is blocked while the subject keeps assignment history
  (`ErrHasAssignments`); retire it with `Active=false` instead, which
  `UpdateSubject` supports. `RemoveAssignment` exists only for corrections.
- Read models: `AssignmentsForTeacher` (chronological timeline),
  `CurrentForTeacher` (open assignments: what is taught now),
  `AssignmentsForSubject` (who taught it, when).

Teacher ids are opaque to `subjects`, mirroring how `attendance` treats
student ids. Referential integrity is enforced by a foreign key, so
assignments cascade when a teacher is deleted.

## Layout

```
src/core/teachers/     profile: model, Store port, Service, MemoryStore
src/core/subjects/     catalog + timeline: model, Store port, Service, MemoryStore
src/platform/postgres/ production adapters + embedded schema.sql
src/tests/teachers|subjects|postgres/
```

## Ports

Each capability owns its persistence interface; adapters are injected from
the composition root.

```go
// teachers.Store
Create(ctx, Teacher) (Teacher, error)
ByID(ctx, id string) (Teacher, error)            // ErrNotFound when missing
List(ctx) ([]Teacher, error)                     // ordered alphabetically
Update(ctx, Teacher) (Teacher, error)
Delete(ctx, id string) error                     // no-op when absent; assignments cascade

// subjects.Store
CreateSubject(ctx, Subject) (Subject, error)
SubjectByID(ctx, id string) (Subject, error)     // ErrNotFound when missing
ListSubjects(ctx) ([]Subject, error)             // ordered alphabetically
UpdateSubject(ctx, Subject) (Subject, error)
DeleteSubject(ctx, id string) error              // no-op when absent
AddAssignment(ctx, Assignment) (Assignment, error)
AssignmentByID(ctx, id string) (Assignment, error) // ErrAssignmentNotFound
AssignmentsForTeacher(ctx, teacherID) ([]Assignment, error) // chronological
AssignmentsForSubject(ctx, subjectID) ([]Assignment, error) // chronological
EndAssignment(ctx, id string, endedAt time.Time) (Assignment, error)
RemoveAssignment(ctx, id string) error           // no-op when absent, corrections only
```

`Service` types wrap each store with validation and UUID assignment
(`google/uuid`). Domain errors implement the i18n `Coded` contract, e.g.
`teachers.ErrNotFound` → catalog key `teachers.err_not_found`.

### teachers.Service validation

Required: names, surnames, document number. Validated when present: email
format, birthdate not in the future. All free text is trimmed and email
lowercased before storage. New teachers are always active (`Active=true`,
ignored on input); retiring sets `Active=false` through `Update` instead
of deleting the profile, so history keeps resolving.

## Database schema

`src/platform/postgres/schema.sql` is embedded and applied idempotently at
connect time (`CREATE TABLE IF NOT EXISTS`, `CREATE UNIQUE INDEX IF NOT
EXISTS`).

```
teachers(id uuid PK, names, surnames, document_id, phone, address,
         birthplace, birthdate DATE NULL, email, medical_conditions,
         homeroom_class_id, active, created_at, updated_at)
subjects(id uuid PK, name, code, description, active,
         created_at, updated_at;
         UNIQUE (lower(name)))
subject_assignments(id uuid PK,
                    teacher_id uuid FK->teachers ON DELETE CASCADE,
                    subject_id uuid FK->subjects ON DELETE CASCADE,
                    started_at DATE NOT NULL, ended_at DATE NULL,
                    CHECK (ended_at IS NULL OR ended_at >= started_at);
                    UNIQUE (teacher_id, subject_id) WHERE ended_at IS NULL)
```

The partial unique index mirrors the service rule: one open assignment
per teacher+subject pair, enforced even under concurrency. Indexes exist
on `teachers.homeroom_class_id` and on both assignment foreign keys.

## Configuration

- `DATABASE_URL`: PostgreSQL connection string. When set, the composition
  root wires the pgx-backed stores; when empty it falls back to in-memory
  stores (development only, logged as a warning).
- `TEST_DATABASE_URL`: used only by integration tests
  (`src/tests/postgres`). When unset those tests skip.

## Error codes and HTTP mapping

| Code | HTTP |
|---|---|
| `teachers.err_not_found` | 404 |
| `teachers.err_invalid_id` / `err_invalid_name` / `err_invalid_document` / `err_invalid_birth` / `err_invalid_email` | 400 |
| `subjects.err_not_found` / `err_assignment_not_found` | 404 |
| `subjects.err_invalid_id` / `err_invalid_name` / `err_invalid_teacher` / `err_invalid_subject` / `err_invalid_date` / `err_invalid_end` / `err_duplicate_subject` / `err_already_assigned` / `err_already_ended` / `err_has_assignments` | 400 |

All messages are localized through `src/platform/i18n/catalogs/es.json`.

## Data protection notes (Ley 1581 de 2012)

- Teacher contact and medical data are personal data: access must be
  authenticated and authorized through the role system once HTTP endpoints
  exist.
- Deleting a teacher removes its assignment timeline (cascade), while
  subjects survive: the catalog is shared and reusable.
- Audit logging of reads/writes is still pending (see roadmap).

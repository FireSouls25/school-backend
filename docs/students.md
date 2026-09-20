# Student profiles and histories

This document covers four related capabilities: `students`, `attendance`,
`incidents` and `warnings` (llamados de atención), plus their PostgreSQL
persistence.

## Overview

A student is identified by an immutable UUID generated at creation time.
The profile keeps:

- **Identity**: given names (`Names`), family names (`Surnames`).
  `FullName()` renders surnames first, the convention used in Colombian
  school listings and the basis for alphabetical ordering.
- **Class assignment**: an opaque `ClassID` (e.g. `9-1`). The format is
  owned by the future `classes` capability; students treats it as a string.
- **Photo**: raw image bytes (JPEG/PNG), capped at `students.MaxPhotoSize`
  (2 MiB). Stored in Postgres as `BYTEA`.
- **Document and contact**: identity document number (`DocumentID`, TI/CC),
  phone, address and email (validated when present).
- **Birth data**: place (`Birthplace`) and date (`Birthdate`) of birth.
  Age is never stored; `AgeAt(ref)` derives full years from the birthdate.
- **Health**: vision and hearing (audición) difficulties as
  `Condition{Has, Detail}` (detail required when affirmative), blood type
  (validated against O/A/B/AB ±), free-text medical report, functional-
  diversity condition / diagnosis, and specialist report (`Condition`).
- **School trajectory**: whether the student is new (`IsNew`), previous
  school, transfer reason, and how many times a year was repeated
  (`RepeatCount >= 0`).
- **Family**: mother, father and acudiente (`Caregiver`), each a `Guardian`
  with names, document, phone, occupation and address. The acudiente name
  is required: it is the notification contact.
- **Household**: who the student lives with (`LivesWith`) and siblings
  enrolled in the institution (`[]Sibling{Name, ClassID}`; empty means
  none).
- **Attendance history**: records typed *inasistencia* (`absence`),
  *evasión* (`evasion`) or *atraso* (`late`), one per student per day per
  class-group.
- **Fault history**: misbehavior records graded *leve* (`minor`),
  *normal* (`ordinary`) or *grave* (`severe`) with a required description.
- **Warning history** (llamados de atención): formal notices with title,
  gravity *leve* (`mild`) / *medio* (`moderate`) / *grave* (`severe`),
  description, event date and hour, issuing teacher, and a frozen
  `StudentSnapshot` of the student's identity at issue time.

Histories live in their own capabilities and reference a student by UUID,
so a student's full record spans current **and** previous class-groups and
years. Deleting a student cascades to all their attendance, fault and
warning records.

## Layout

```
src/core/students/     profile: model, Store port, Service, MemoryStore
src/core/attendance/   assistance: Reason enum, Record, Store port, Service
src/core/incidents/    faults: Severity enum, Fault, Store port, Service
src/core/warnings/     llamados: Gravity enum, Warning+Snapshot, Store, Service
src/platform/postgres/ production adapters + embedded schema.sql
src/tests/students|attendance|incidents|warnings|postgres/
```

## Ports

Each capability owns its persistence interface; adapters are injected from
the composition root.

```go
// students.Store
Create(ctx, Student) (Student, error)
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

// warnings.Store
Add(ctx, Warning) (Warning, error)
ForStudent(ctx, studentID) ([]Warning, error)    // newest first
ForGroup(ctx, groupID string) ([]Warning, error) // oldest first
Delete(ctx, id string) error
```

`Service` types wrap each store with validation and UUID assignment
(`google/uuid`). Domain errors implement the i18n `Coded` contract, e.g.
`students.ErrNotFound` → catalog key `students.err_not_found`.

### students.Service validation

Required: names, surnames, class, document number, acudiente name.
Validated when present: email format, blood-type label, birthdate not in
the future, non-negative repeat count, health/specialist detail when
affirmative, sibling name + class. All free text is trimmed; blood type is
uppercased and email lowercased before storage. New students are always
active; `Graduate(id)` moves `Status` to graduated (end of the lifecycle),
refusing repeats with `ErrAlreadyGraduated`. `Update` preserves the stored
status: graduation only happens through `Graduate`, which the future
promotion flow calls.

### warnings.Service and the snapshot

`Issue(ctx, Input)` persists a warning with a caller-supplied
`StudentSnapshot` (names, surnames, document, class, birthdate, age,
caregiver name/phone). The snapshot is a frozen copy: later profile edits
never rewrite it. The future HTTP layer builds it from the current
profile (`AgeAt` computes the age at event time). The teacher is an opaque
id today; the future `users`/`auth` feature will own it.

`IssueBatch(ctx, BatchInput)` issues one event against several students:
all items validate before anything persists, then every warning shares a
fresh `GroupID` while each student keeps its own history (`ForStudent`).
`ForGroup` returns the whole event, oldest first. Warnings stay immutable
after issue: corrections belong to sessions, not to formal notices.

## Database schema

`src/platform/postgres/schema.sql` is embedded and applied idempotently at
connect time (`CREATE TABLE IF NOT EXISTS` plus `ADD COLUMN IF NOT EXISTS`
migrations for installs created before the extended profile).

```
students(id uuid PK, names, surnames, class_id,
         document_id, phone, address, birthplace, birthdate DATE NULL,
         email, vision_has, vision_detail, hearing_has, hearing_detail,
         blood_type, is_new, previous_school, transfer_reason, repeat_count,
         mother_*, father_*, caregiver_* (name, document, phone, occupation, address),
         lives_with, siblings JSONB, medical_report, diversity_condition,
         specialist_has, specialist_detail, status CHECK IN ('active','graduated'),
         photo bytea, created_at, updated_at)
attendance_records(id uuid PK, student_id uuid FK->students ON DELETE CASCADE,
                   class_id text, date date,
                   reason text CHECK IN ('absence','evasion','late'))
incident_faults(id uuid PK, student_id uuid FK->students ON DELETE CASCADE,
                class_id text, date date,
                severity text CHECK IN ('minor','ordinary','severe'),
                description text)
warnings(id uuid PK, student_id uuid FK->students ON DELETE CASCADE,
         class_id text, teacher_id text, happened_at timestamptz,
         gravity text CHECK IN ('mild','moderate','severe'),
         title text, description text, group_id TEXT ('' = single),
         snap_* (frozen identity: names, surnames, document, class,
                 birthdate, age, caregiver name/phone))
```

Indexes exist on every `student_id` and on `students.class_id`. Enum-like
values are stored as stable lowercase identifiers (English), never as
display text; Spanish labels come from the i18n catalog
(`reason.late` → "Atraso", `warning.gravity.moderate` → "Medio", ...).

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
| `students.err_invalid_id` / `err_invalid_name` / `err_invalid_class` / `err_invalid_document` / `err_invalid_birth` / `err_invalid_blood_type` / `err_invalid_email` / `err_invalid_guardian` / `err_invalid_sibling` / `err_invalid_health` / `err_invalid_repeat_count` / `err_already_graduated` | 400 |
| `students.err_photo_too_large` | 413 |
| `attendance.err_unknown_reason` / `err_invalid_student` / `err_invalid_class` / `err_invalid_date` | 400 |
| `attendance.err_not_found` | 404 |
| `incidents.err_unknown_severity` / `err_invalid_student` / `err_invalid_class` / `err_invalid_date` / `err_empty_description` | 400 |
| `incidents.err_not_found` | 404 |
| `warnings.err_unknown_gravity` / `err_invalid_student` / `err_invalid_class` / `err_invalid_teacher` / `err_invalid_date` / `err_empty_title` / `err_empty_description` / `err_invalid_snapshot` / `err_invalid_group` / `err_empty_batch` | 400 |
| `warnings.err_not_found` | 404 |

All messages are localized through `src/platform/i18n/catalogs/es.json`.

## Data protection notes (Ley 1581 de 2012)

- Photos, health data, diversity conditions and specialist reports concern
  minors and are sensitive personal data: access must be authorized (role
  system gates future endpoints), collect only what the school requires.
- Deletion of a student removes dependent histories (cascade), supporting
  data-minimization requests.
- Audit logging of reads/writes is still pending (see roadmap).

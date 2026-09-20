# Architecture

## Principles

- **Screaming architecture**: the tree screams the business capabilities, not
  the tech. A capability is a folder under `src/core/` with its own domain
  model, ports and logic. There is no `handlers/`, `services/` or
  `repositories/` layer slicing across features.
- **Dependency inversion**: each feature defines the interfaces (ports) it
  needs and consumes them; implementations (adapters) are supplied at the
  composition root. Core never imports platform; platform imports core only
  through ports.
- **Robustness**: prefer well-maintained libraries over custom code
  (`chi` for routing, `go-i18n/v2` for translations, stdlib `testing`).
- **Language**: user-facing text is always pulled from i18n catalogs (Spanish
  today). Everything a user never sees - identifiers, comments, logs, internal
  error text - stays in English.

## Layout

```
src/
  cmd/server/   composition root. Only place allowed to wire concrete types.
  core/         business capabilities. Each folder is a feature.
  platform/     cross-cutting infrastructure (config, http, i18n). Depends on
                core only through ports.
  tests/        test suites, kept apart from production code. Each subfolder
                is an external test package (package <name>_test) exercising
                the package under test through its public API.
```

Dependency direction: `cmd -> platform -> core`. `core` has no dependencies
outside the standard library. Tests in `src/tests` are external test packages,
so they also only see the public API of the packages they cover.

## Role system (`src/core/roles`)

### Domain

- `Role` is a string value type (`student | teacher | admin`) with tolerant
  parsing and validation. Role identifiers are stable; display names come from
  the catalog (`role.<id>`).
- `Permission` is a fine-grained action. The permission matrix maps each role
  to the permissions it grants; `admin` is expanded to the union of all
  permissions at init.
- Domain errors implement a structural `Code() string` (see
  `Coded` in `src/platform/i18n`), giving every error a stable catalog key.

### Ports

```go
// Persistence: assign, revoke and query a subject's roles.
type Store interface {
    AssignRole(ctx, subjectID string, role Role) error
    RemoveRole(ctx, subjectID string, role Role) error
    RolesFor(ctx, subjectID string) ([]Role, error)
}

// Consumed by downstream features (students, incidents, statistics).
type Authorizer interface {
    Can(ctx, subjectID string, permission Permission) (bool, error)
}

type RoleGetter interface {
    Roles(ctx, subjectID string) ([]Role, error)
}
```

`Store` implementations: `MemoryStore` now, a PostgreSQL adapter later, both
injected from `cmd/server`. `Service` implements `Authorizer` and `RoleGetter`
so feature handlers depend on the port, never on the concrete store.

`subjectID` is intentionally opaque: roles does not know about users. The
users/auth feature will produce real identifiers and enforce authentication;
roles only authorizes.

## Students (`src/core/students`)

- `Student` is the profile aggregate: a stable UUID, names, surnames and an
  opaque `ClassID` (owned by the future classes capability). `FullName`
  renders surnames first (Colombian listing convention).
- `Store` port covers create/read/list-per-class/update/delete plus photo
  upload (`PutPhoto`, capped at `MaxPhotoSize`, 2 MiB) and retrieval.
  Implementations must return lists ordered alphabetically.
- `Service` validates input, assigns UUIDs (`google/uuid`) and maps errors to
  coded domain errors.

## Attendance (`src/core/attendance`)

- `Record` keeps one entry per student per day with a `Reason`:
  `absence` (inasistencia) or `evasion`. Records carry the `ClassID` so a
  student's history spans current and previous class-groups and years.
- `Store` port: add, list per student (newest first), delete. History is
  cascade-deleted with the student.

## Incidents (`src/core/incidents`)

- `Fault` records misbehavior with a `Severity`: `minor` (leve),
  `ordinary` (normal) or `severe` (grave). `Parse` also accepts the Spanish
  terms. Like attendance records, faults keep the `ClassID` for full-history
  queries and cascade-delete with the student.

## Persistence (`src/platform/postgres`)

- Production adapters implement the three Store ports on `pgx/v5`
  (`pgxpool`). The idempotent schema lives in `schema.sql` (embedded) and is
  applied at connect time.
- The composition root selects PostgreSQL when `DATABASE_URL` is set,
  otherwise in-memory stores (development only). Integration tests run
  against `TEST_DATABASE_URL` and skip when unset.

## Multilanguage support (`src/platform/i18n`)

- Catalogs are embedded JSON files, one per locale, under
  `src/platform/i18n/catalogs/`. Only `es.json` ships now.
- Message keys are stable identifiers; keys can carry template variables
  (`{{.Name}}`) and plural forms.
- Language is chosen per request: `?lang=` query parameter wins, then the
  `Accept-Language` header, defaulting to `es`. The locale middleware stores a
  `Translator` in the request context.
- Error responses use a fixed envelope: `{"error": {"code", "message"}}`.
  `code` is the stable key; `message` is localized. Status codes are mapped
  from known codes in `src/platform/http/respond.go`.
- A per-user language preference is not implemented yet; it belongs to the
  users feature. Non-HTTP consumers (WhatsApp notifications) will pass an
  explicit language.

## Colombian data law (Ley 1581 de 2012)

Student data concerns minors and is subject to habeas data rules. Design
consequences:

- Access must be authenticated and authorized (the role system gates this;
  teachers cannot view student information without permission).
- Minimize stored data; collect only what the school requires.
- Audit who reads and writes student records (add later).
- Notifications go only to a guardian's number registered for the student.

## Roadmap (future `src/core/` capabilities)

- `users` / `auth` - authentication, user lifecycle, per-user language pref
- `classes` - grades 1-11, groups (9-1, 9-2, ...), years, periods
- `notifications` - WhatsApp delivery to guardians
- `statistics` - per-class and per-student aggregations

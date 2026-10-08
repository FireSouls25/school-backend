# API conventions and access control

Base path: `/v1` (plus public `/healthz`). All responses are JSON;
all user-facing text is Spanish from the i18n catalogs.

## Identity (pre-auth)

Real accounts already exist in the `users` table (bcrypt passwords,
`docs/users.md`), but no login/token endpoint issues sessions yet, so the
caller still identifies with the `X-Subject-ID` header carrying a valid
UUID (which is a `User.ID` once the account is created):

- Missing or malformed id → `401 http.err_unauthorized`, nothing runs.
- The header is trusted transport, not proof: it is only acceptable
  because no login exists yet. A token scheme replaces the header
  without touching handlers (they only read the context subject).

## Test accounts (dev only)

Roles live in memory, so every restart wipes them. `SEED_ADMINS` and
`SEED_TEACHERS` (comma-separated subject UUIDs) grant roles at boot:

```sh
SEED_ADMINS=aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa \
SEED_TEACHERS=bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb \
python3 scripts/dev.py run
```

Never set these in production. The frontend dev directory
(`frontend/src/domains/auth/accounts.ts`) maps test usernames to these
ids until real authentication replaces it.

## Authorization

Routes declare one required permission, enforced by
`RequirePermission` against `roles.Authorizer`:

| Route | Permission | Who |
|---|---|---|
| `GET /v1/me` | authenticated only | anyone with an id |
| `GET /v1/students/{id}` | `view-students`, or `view-own-history` on self | teacher, admin / student (own) |
| `GET /v1/students/{id}/warnings` | same as above | teacher, admin / student (own) |
| `GET /v1/statistics/class/{groupID}` | `view-class-statistics` | teacher, admin |
| `GET /v1/statistics/student/{studentID}` | `manage-system` | admin only |
| `POST /v1/sessions/{sessionID}/marks` | `record-attendance` | teacher, admin |
| `GET /v1/classes?year=AAAA` | `view-class-statistics` | teacher, admin |
| `GET /v1/classes/{groupID}/roster` | `view-class-statistics` | teacher, admin |
| `GET /v1/teachers/me/schedule?weekday=N` | `view-class-statistics` | teacher, admin |
| `POST /v1/sessions/open` | `record-attendance` | teacher, admin |
| `POST /v1/warnings/batch` | `record-incidents` | teacher, admin |
| `POST /v1/incidents` | `record-incidents` | teacher, admin |
| `GET /v1/students/{id}/incidents` | `view-students`, or `view-own-history` on self | teacher, admin / student (own) |
| `GET /v1/dashboard/summary?year=AAAA` | `manage-system` | admin only |

Self-scoped reads (`view-own-history`) additionally require
`subject == id`; anything else is denied even with the permission.
Unknown subjects (valid UUID, no roles) are denied everywhere guarded.
Authorizer failures fail closed: no data, `500` with no details.

### User accounts (admin only)

`POST /v1/users` (`manage-users`) creates a login account with an
initial password: no self-registration. Body
`{"username","email","password","role?"}`; `201` returns the sanitized
account `{ID, Username, Email, Active, Roles}`. The optional `role`
additionally requires `manage-roles`. Teachers get their account (and
role) from this endpoint when the admin creates their profile; they only
ever log in. Frontend flow in `docs/users.md`.

## Class groups, rosters and schedules (teacher + admin)

- `GET /v1/classes?year=AAAA` (`view-class-statistics`) lists the
  salones of a calendar year. `year` is required and must fall between
  2000 and 2100; unknown years answer `404`. The year number resolves
  to the id the classes service asks for.
- `GET /v1/classes/{groupID}/roster` (`view-class-statistics`)
  returns the group nómina: enrolled students with their current
  profiles, alphabetically by surnames then names (same criterion as
  the class report). Enrollments pointing at removed profiles are
  skipped, so a deleted profile never blocks the roll; unknown groups
  answer `404`.
- `GET /v1/teachers/me/schedule?weekday=N`
  (`view-class-statistics`) returns the caller's slots for one day:
  `weekday` is required, `0` (Sunday) to `6` (Saturday) as
  `time.Weekday`. Anything else fails with `400`.

## Sessions, warnings and faults

- `POST /v1/sessions/open` (`record-attendance`) opens an attendance
  call with its roster frozen as it is at that moment. Body
  `{"classGroupID","period","date?","subjectID?"}`: `date` is
  optional RFC3339 (defaults to now, UTC) and `subjectID` optional;
  label, school year and teacher resolve server-side (the teacher is
  always the caller). A group with nobody enrolled fails with `400`
  (`sessions.err_empty_roster`); unknown groups answer `404`. `201`
  returns the stored session.
- `POST /v1/warnings/batch` (`record-incidents`) issues one event
  against several students. Body
  `{"classID","gravity","title","description","happenedAt?","studentIDs"}`:
  `gravity` accepts the stable id and the Spanish term
  (`leve/medio/moderado/grave`); `happenedAt` is optional RFC3339
  (defaults to now, UTC). Snapshots freeze from the current profiles
  and the teacher is always the caller. Every warning shares a fresh
  group id while each student keeps its own history; profiles resolve
  before anything persists, so an unknown student fails the whole
  batch with `404` and an empty batch with `400`. `201` returns the
  stored warnings.
- `POST /v1/incidents` (`record-incidents`) records one fault. Body
  `{"studentID","classID","date?","severity","description"}`:
  `severity` accepts the stable id and the Spanish term
  (`leve/normal/grave`); `date` is optional RFC3339 (defaults to now,
  UTC). `201` returns the stored fault.
- `GET /v1/students/{id}/incidents` returns one student's fault
  history, newest first: same readers as the warning history
  (`view-students`, or `view-own-history` on self).

## Admin dashboard

- `GET /v1/dashboard/summary?year=AAAA` (`manage-system`, admin only)
  returns the landing aggregate for a calendar year (`year` required,
  same rules as `/v1/classes`): staff/student/group totals, llamados
  and attendance marks of the last 7 days, the top inasistencia risk
  ranking (max 8) and the latest llamados (max 6).

Documented limits (pending, never invented):

- `Totals.Teachers` counts every teacher on staff, not only those
  teaching that year: no year-scoped staff list exists yet.
- Attendance and top-risk fold at most 25 groups per year
  (`GroupsTruncated: true` past the cap, first groups in grade/group
  order). Totals still count every group. The full sweep stays
  pending until a cheaper cross-group attendance source exists.
- Anything else stays out of the summary rather than guessed.

## Denials and frontend navigation

Denials never leak data and always carry a stable code:

- `401 http.err_unauthorized` — no usable identity. Go to login.
- `403 http.err_forbidden` — authenticated but not allowed.

The backend never redirects API calls (fetch/Tauri clients follow
redirects opaquely, and non-GET requests cannot be bounced
meaningfully). Instead the SPA implements "send back":

1. On `403`, navigate back in history.
2. If there is no previous page, fall back to the role home from
   `GET /v1/me` (`home` field): `/admin`, `/docente`, `/estudiante`,
   `/` for subjects without roles.

So a teacher opening an admin-only report gets `403
http.err_forbidden`, sees nothing, and lands back where they were —
or on `/docente` by default.

## Request/response rules (kept consistent)

- Success: domain objects as-is (`200`), created revisions (`201`).
  No envelope on success; errors always use
  `{"error": {"code", "message"}}`.
- Bodies are capped at 1 MiB, strict JSON (`DisallowUnknownFields`,
  single value): unknown fields fail with `400 http.err_bad_request`
  instead of being silently ignored.
- The acting author is always the authenticated caller (e.g. mark
  `ChangedBy`), never a body field — clients cannot spoof authorship.
- Ids in paths and bodies must be UUIDs; domain services validate and
  map to `400` codes, unknown ids to `404`.
- `POST /v1/sessions/{id}/marks` accepts `"mark": ""` to clear back to
  present (correction flow) and Spanish aliases (`"atraso"`, …).
- CORS reflects only origins in `ALLOWED_ORIGINS` (comma-separated);
  empty means same-origin only. Non-browser clients are unaffected.
- Server timeouts: 5s headers, 10s read, 15s write, 60s idle.

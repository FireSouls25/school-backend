# API conventions and access control

Base path: `/v1` (plus public `/healthz`). All responses are JSON;
all user-facing text is Spanish from the i18n catalogs.

## Identity (Bearer + refresh rotation)

Logins live in the `users` table (bcrypt, `docs/users.md`).
`POST /v1/login {identifier, password}` mints a short access bearer
(15m default, `AUTH_ACCESS_TTL`) plus a rotating refresh token (7d
default, `AUTH_REFRESH_TTL`):

- Access travels as `Authorization: Bearer <token>`; only `typ=access`
  opens API routes. Refresh tokens (`typ=refresh`) only open
  `POST /v1/refresh`.
- Refresh rotates on every use: reuse of a rotated token fails with
  `401 auth.err_revoked_token`. `POST /v1/logout` revokes the presented
  refresh (cookie/body); `{all:true}` with an access bearer revokes every
  session. The rotation cookie is `HttpOnly, SameSite=Lax, Path=/v1`
  (`Secure` on TLS); JSON keeps a transitional copy for the SPA.
- Every access carries `ver` (account token version) and `jti`. Password
  rotation and disable bump the version: old access dies at the next
  request (`401`), old refresh at the next rotation. Unknown/wrong/inactive
  share `401 users.err_invalid_credentials` (no enumeration); throttled
  logins answer `429 http.err_rate_limited`. Budgets are persistent
  (`login_attempts`: 10/min per identifier+IP on login, 30/min per IP on
  refresh) and shared across restarts/replicas; denied attempts never
  extend the window. Behind a reverse proxy set `TRUSTED_PROXIES` so
  `X-Forwarded-For` is trusted for the client IP only from those proxies.
  Every guess is audited (identifier, IP, success, time); there is no
  permanent lockout by design (anti-DoS): disable the account to cut
  access immediately (version bump kills bearers, refresh is rejected).

## Password recovery and lockout

- `POST /v1/recover/request {identifier}` always answers `204`, whether
  the account exists or not. Valid active accounts get a single-use token
  (`AUTH_RECOVERY_TTL`, default 1h, hashed at rest); the raw token is
  logged only when the server runs outside production
  (`RecoveryLogTokens`), until mail/WhatsApp delivery lands.
- `POST /v1/recover/reset {token, newPassword}` consumes the token once:
  unknown, expired and reused tokens share `401
  users.err_invalid_recovery_token`; weak passwords answer `400`.
  Success bumps the token version and revokes every refresh row, so all
  sessions die with the old password.
- Lockout response is administrative: `PUT /v1/users/{id}/active
  {active:false}` (`manage-users`) disables a login without deleting
  history; bearers fail at the next request and refresh at the next
  rotation. Re-enable with `{active:true}` (re-login required).
- `X-Subject-ID` survives only when `AllowDevIdentity` holds
  (development/tests, never production). Missing/invalid identity →
  `401 http.err_unauthorized`.

## Test accounts (dev only)

Demo school seeds mock logins with username = role and password = role + `123*`: `admin`, `docente`, `secretaria` and `estudiante`
(`go run ./src/cmd/devseed`, `SEED_DEMO`). Never set these in production;
`APP_ENV=production` rejects them and requires `AUTH_SECRET`.

## Authorization

Routes declare one required permission, enforced by
`RequirePermission` against `roles.Authorizer`:

| Route | Permission | Who |
|---|---|---|
| `POST /v1/login` | public (throttled 10/min) | anyone with credentials |
| `POST /v1/refresh` | refresh token (throttled 30/min) | session holder (cookie/body) |
| `POST /v1/logout` | refresh token, or access bearer for `all:true` | session holder |
| `POST /v1/recover/request` | public (throttled 5/min) | anyone (always 204, no enumeration) |
| `POST /v1/recover/reset` | reset token (throttled 5/min) | password-forgotten holder |
| `GET /v1/teachers/{id}/account` | `manage-users` | admin only (login or honest 404) |
| `PUT /v1/users/{id}/active` | `block-access` | admin + secretary (never admin targets without manage-users; trailed with subject) |
| `GET /v1/me` | authenticated only | anyone with an id |
| `PUT /v1/me/password` | authenticated only | anyone with an account row |
| `GET /v1/me/photo` | authenticated only | anyone (photo lives on the account; linked docentes keep the teacher profile photo) |
| `PUT /v1/me/photo` | authenticated only | anyone (photo lives on the account; linked docentes keep the teacher profile photo) |
| `GET /v1/students/{id}` | `view-students`, or `view-own-history` on self | teacher, admin / student (own) |
| `GET /v1/students/{id}/warnings` | same as above | teacher, admin / student (own) |
| `GET /v1/statistics/class/{groupID}` | `view-class-statistics` | teacher, admin |
| `GET /v1/statistics/student/{studentID}` | `manage-system` | admin only |
| `POST /v1/sessions/{sessionID}/marks` | `record-attendance` | teacher, admin |
| `GET /v1/classes?year=AAAA` | `view-class-statistics` | teacher, admin |
| `POST /v1/classes` | `manage-users` | admin only |
| `GET /v1/schoolyears` | `view-class-statistics` | teacher, admin |
| `GET /v1/schoolyears/{id}` | `view-class-statistics` | teacher, admin |
| `GET /v1/holidays?year=AAAA` | `view-class-statistics` | teacher, admin |
| `POST /v1/schoolyears` | `manage-users` | admin only |
| `PUT /v1/schoolyears/{id}` | `manage-users` | admin only |
| `POST /v1/schoolyears/{id}/close` | `manage-users` | admin only |
| `POST /v1/enrollments` | `manage-students` | admin only |
| `GET /v1/enrollments?groupID=&studentID=` | `manage-students` | admin only (exactly one filter) |
| `DELETE /v1/enrollments/{id}` | `manage-students` | admin only |
| `POST /v1/promotions` | `manage-students` | admin only |
| `GET /v1/promotions?studentID=&groupID=` | `manage-students` | admin only (exactly one filter) |
| `GET /v1/classes/{groupID}/roster` | `view-class-statistics` | teacher, admin |
| `GET /v1/teachers/me/schedule?weekday=N` | `view-class-statistics` | teacher, admin |
| `GET /v1/teachers/me/photo` | authenticated only | anyone with a linked teacher profile |
| `PUT /v1/teachers/me/photo` | authenticated only | anyone with a linked teacher profile |
| `PUT /v1/teachers/me` | authenticated only | own contact/health ficha (identity, homeroom and active stay admin-only) |
| `POST /v1/sessions/open` | `record-attendance` | teacher, admin |
| `POST /v1/warnings/batch` | `record-incidents` | teacher, admin |
| `POST /v1/academic-alerts` | `record-incidents` | teacher raises, admin + secretary read |
| `GET /v1/academic-alerts` | `view-warnings` | open alerts, newest first |
| `GET /v1/students/{id}/alerts` | auth + own history | teacher, admin / student (own) |
| `POST /v1/academic-alerts/{id}/review` | `manage-system` | admin only; closes the pending |
| `POST /v1/incidents` | `record-incidents` | teacher, admin |
| `GET /v1/students/{id}/incidents` | `view-students`, or `view-own-history` on self | teacher, admin / student (own) |
| `GET /v1/dashboard/summary?year=AAAA` | `view-class-statistics` | teacher, admin, secretary |
| `POST /v1/students` | `manage-students` | admin only |
| `PUT /v1/students/{id}` | `manage-students` | admin only |
| `POST /v1/teachers` | `manage-users` | admin only |
| `PUT /v1/teachers/{id}` | `manage-users` | admin only |
| `GET /v1/teachers` | `view-teachers` | admin + secretary |
| `GET /v1/teachers/{id}` | `view-teachers` | admin + secretary |
| `GET /v1/subjects` | `view-class-statistics` | teacher, admin |
| `GET /v1/subjects/{id}` | `view-class-statistics` | teacher, admin |
| `POST /v1/subjects` | `manage-users` | admin only |
| `PUT /v1/subjects/{id}` | `manage-users` | admin only |
| `DELETE /v1/subjects/{id}` | `manage-users` | admin only |
| `GET /v1/subjects/{id}/assignments` | `view-class-statistics` | teacher, admin |
| `POST /v1/assignments` | `manage-users` | admin only |
| `POST /v1/assignments/{id}/end` | `manage-users` | admin only |
| `DELETE /v1/assignments/{id}` | `manage-users` | admin only |
| `GET /v1/teachers/{id}/assignments` | `manage-users` | admin only |
| `GET /v1/warnings/recent?limit=N` | `view-warnings` | admin + secretary |
| `POST /v1/teachers/attendance` | `manage-teachers` | admin only |
| `GET /v1/teachers/attendance?date=YYYY-MM-DD` | `manage-teachers` | admin only |
| `GET /v1/teachers/{id}/attendance?from=&to=` | `manage-teachers` | admin only |
| `GET /v1/teachers/roll?date=YYYY-MM-DD` | `manage-teachers` | admin only |
| `GET /v1/teachers/attendance/history?from=&to=` | `manage-teachers` | admin only |
| `PUT /v1/teachers/{id}/photo` | `manage-users` | admin only |
| `GET /v1/teachers/{id}/photo` | `manage-users` | admin only |
| `GET /v1/teachers/{id}/sessions` | `manage-teachers` | admin only |
| `GET /v1/teachers/{id}/warnings-made` | `manage-teachers` | admin only |
| `GET /v1/sessions/history?classGroupID=&from=&to=` | `view-class-statistics` | teacher, admin |
| `GET /v1/sessions/{id}` | `view-class-statistics` | teacher, admin |
| `GET /v1/incidents/recent?limit=N` | `manage-system` | admin only |
| `GET /v1/schedule?teacherID=&classGroupID=&weekday=` | `view-class-statistics` | teacher, admin |
| `POST /v1/schedule` | `manage-schedules` | admin + secretary |
| `PUT /v1/schedule/{id}` | `manage-schedules` | admin + secretary |
| `DELETE /v1/schedule/{id}` | `manage-users` | admin only |
| `GET /v1/audit?limit=&studentID=` | `manage-system` | admin only |
| `POST /v1/roles` | `manage-roles` | admin only |

Self-scoped reads (`view-own-history`) additionally require
`subject == id`; anything else is denied even with the permission.
Unknown subjects (valid UUID, no roles) are denied everywhere guarded.
Authorizer failures fail closed: no data, `500` with no details.

`GET /v1/me` returns navigation plus the personal profile of the login:
`{subject, roles, home, username, email, active, createdAt, teacherID,
teacher?}`. `teacher` is the linked teaching profile summarized for the
profile panel (`{id, names, surnames, documentID, phone, email,
birthdate, sex, active}`); address and medical conditions stay out. Dev
identities without a `users` row still get subject, roles and home, with
the account fields empty — never invented.

`PUT /v1/me/password` lets the signed-in account swap its own password:
body `{"currentPassword","newPassword"}`, `204` on success. The current
password is verified first, so a stolen bearer cannot silently take over
the account; a wrong one answers `401` (`users.err_invalid_credentials`),
a weak new one `400` (`users.err_weak_password`), and an unknown account
`404`. Tokens already issued stay valid until expiry — there is no
denylist yet.

`GET /v1/me/photo` / `PUT /v1/me/photo` are the account-level twins of
the photo endpoints (auth only): the picture of the caller, resolved as
the linked teacher photo when it exists, the login's own `users.photo`
otherwise. The PUT accepts the same data URL/base64 payload, 2 MiB cap
(`400` `teachers.err_photo_too_large` on teacher profiles,
`users.err_photo_too_large` otherwise) and an empty `data` clears (back
to an honest `404`). Student logins stay out until accounts link into
students — there is no `StudentID` link yet.

### User accounts (admin only)

`POST /v1/users` (`manage-users`) creates a login account with an
initial password: no self-registration. Body
`{"username","email","password","role?"}`; `201` returns the sanitized
account `{ID, Username, Email, Active, Roles}`. The optional `role`
additionally requires `manage-roles`. Teachers get their account (and
role) from this endpoint when the admin creates their profile; they only
ever log in. Frontend flow in `docs/users.md`.

## School years (teacher reads, admin writes)

- `GET /v1/schoolyears` (`view-class-statistics`) lists every year with
  its dated periods, newest first — the admin year picker and the
  "suggest last period count" default read it.
- `GET /v1/schoolyears/{id}` (`view-class-statistics`) returns one year.
- `POST /v1/schoolyears` (`manage-users`) creates the year: `year`,
  `periods` (1–6), `startDate`/`endDate` (RFC3339), `holidays`
  and `periodRanges` (one `{start,end}` per period, in range, ordered,
  non-overlapping; empty ranges fail with
  `schoolyears.err_invalid_period_ranges`). `201` returns the stored year.
- `PUT /v1/schoolyears/{id}` (`manage-users`) replaces the year. Once it
  grew class-groups, periods, ranges and dates freeze
  (`schoolyears.err_year_started`); only holidays and no-class days still
  move — plus completing empty ranges once for legacy years. Closed
  years accept no edits (`schoolyears.err_year_closed`).
- `POST /v1/schoolyears/{id}/close` (`manage-users`) finishes the year
  (idempotent): afterwards it takes no enrollments and no edits.
- `GET /v1/holidays?year=AAAA` (`view-class-statistics`) returns the
  official Colombian holidays (Ley 51 de 1983) as `{"year","holidays"}`
  with `YYYY-MM-DD` days: the editable seed for the year form.
- `GET /v1/teachers/roll?date=` returns nobody expected on non-school
  days (festivos, marked no-class days); recorded marks stay visible and
  explicit rolls remain allowed: the call is deliberate, so it records.
- `GET /v1/schoolyears` entries carry `holidays` plus `noClassDays`
  (manually marked days without classes: locally moved holidays,
  emergencies). Both are date-only, sorted, deduplicated, inside the
  range; overlap is allowed and both badges show. `IsSchoolDay` is false
  for either; explicit roll calls always record.

## Enrollments and promotions (admin only)

- `POST /v1/enrollments {studentID, classGroupID}` (`manage-students`)
  matriculates: unknown student/group answer `404`, duplicates `400`,
  closed years `400 schoolyears.err_closed`. `201` returns the enrollment.
- `GET /v1/enrollments` lists by `groupID` (roster source) or `studentID`
  (placement history); exactly one filter is required.
- `DELETE /v1/enrollments/{id}` removes one enrollment (corrections).
- `POST /v1/promotions {studentID, fromGroupID, toGroupID?, decision}`
  records the audited year-end decision and applies it: promote/repeat
  enroll the destination (already-placed is tolerated, so retries never
  fail), repeat bumps `RepeatCount`, graduate ends the lifecycle
  (`students.Graduate`). Grade 11 only graduates or repeats; graduation
  needs no destination and rejects one. `DecidedBy`/`DecidedAt` come from
  the server. `201` returns the promotion record.
- `GET /v1/promotions` lists by `studentID` (history) or `groupID`
  (the year-end review: decided vs pending per salon).

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
- `POST /v1/schedule` and `PUT /v1/schedule/{id}` assign the
  teacher-subject pairing automatically when no open pairing exists
  (started today); overlaps and out-of-hours still fail.

## Sessions, warnings and faults

- `GET /v1/sessions/history?classGroupID=&from=&to=`
  (`view-class-statistics`) returns the roll calls of one salon in a
  range, newest first (capped at 50): date, group, period, subject
  and teacher resolved, with
  present/absent/permission/evasion/late counts folded
  from the revision trail.
- `GET /v1/sessions/{id}` (`view-class-statistics`) returns one roll
  call with its current marks and full revision trail (student,
  before/after, author, timestamp, note).

- `POST /v1/teachers/attendance` (`manage-teachers`, admin only)
  records the daily roll of teachers. Body
  `{"day","marks":[{"teacherID","status?","note?"}]}`: `day` is
  `YYYY-MM-DD` or RFC3339 (today or past, never future); an empty
  `status` means present and Spanish terms parse
  (`presente/ausente/atraso`). Unknown teachers fail the whole day
  with `404`; recording again corrects the day (upsert) and each mark
  is audited. `GET /v1/teachers/attendance?date=` returns the day with
  display names; `GET /v1/teachers/{id}/attendance?from=&to=` returns
  one teacher's history newest first with summary counts and the
  revision trail (old/new status, who, when).
- `GET /v1/teachers/attendance/history?from=&to=` (`manage-teachers`)
  returns one card per recorded day, newest first: total, present,
  absent and late counts. Powers the roll history feed.
- `GET /v1/teachers/attendance/revisions?date=` (`manage-teachers`)
  returns every change recorded one day, newest first: teacher,
  old/new status, note, author and timestamp.
- `GET /v1/teachers/roll?date=` (`manage-teachers`) returns the
  teachers expected that weekday (from the weekly schedule) with
  their recorded marks, if any. Unmarked teachers carry an empty
  status: the roll screen shows nothing pre-selected.
- `PUT /v1/teachers/{id}` (`manage-users`) applies a partial profile
  update: every body field is optional (`{"Names","Surnames","DocumentID",
  "Phone","Address","Birthplace","Birthdate","Email","Sex",
  "MedicalConditions","HomeroomClassID","Active"}`) and what the body
  omits keeps its stored value, so a closed form can never silently
  erase contact data or retire a teacher. Validation runs through the
  service (`400` `teachers.err_*` on bad input); `200` returns the
  updated profile.
- `PUT /v1/teachers/{id}/photo` / `GET /v1/teachers/{id}/photo`
  (`manage-users`) store and return the teacher picture (2 MiB max,
  empty photo is an honest 404), same contract as students.
- `PUT /v1/teachers/me/photo` / `GET /v1/teachers/me/photo`
  (authenticated only) are the self-service twins: the caller's own
  teacher photo, resolved through the account link (the subject itself
  is tried when it already is a teacher id). Same 2 MiB cap and honest
  404; callers without a teacher profile get `403`.
- `GET /v1/teachers/{id}/sessions` (`manage-teachers`) returns the
  roll calls taken by one teacher, newest first (capped at 20), with
  absence counts folded from the revision trail.
- `GET /v1/teachers/{id}/warnings-made` (`manage-teachers`) returns
  the llamados issued from the logins linked to one teacher (capped
  at 50), newest first with display names.

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
- `GET /v1/warnings/recent?limit=N` (`view-warnings`, admin + secretary)
  returns the latest llamados newest first, each with the stored
  warning plus `StudentName`, `ClassLabel` and `AuthorName` resolved
  best-effort (live records win, the frozen snapshot stands in
  otherwise; admins show their username, linked logins their teacher
  name). Powers the admin history at `/admin/llamados`, where an
  admin can also issue llamados through the same batch endpoint.
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
  present (correction flow) and Spanish aliases (`"atraso"`). Marks are
  `absence` (falta injustificada), `absence-permission` (falta con
  permiso — also `"permiso"`), `evasion` and `late`.
- CORS reflects only origins in `ALLOWED_ORIGINS` (comma-separated);
  empty means same-origin only. Non-browser clients are unaffected.
- Server timeouts: 5s headers, 10s read, 15s write, 60s idle.

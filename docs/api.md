# API conventions and access control

Base path: `/v1` (plus public `/healthz`). All responses are JSON;
all user-facing text is Spanish from the i18n catalogs.

## Identity (pre-auth)

Until `users`/`auth` lands, the caller identifies with the
`X-Subject-ID` header carrying a valid UUID:

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

Self-scoped reads (`view-own-history`) additionally require
`subject == id`; anything else is denied even with the permission.
Unknown subjects (valid UUID, no roles) are denied everywhere guarded.
Authorizer failures fail closed: no data, `500` with no details.

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

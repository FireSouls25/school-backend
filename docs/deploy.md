# Deployment and local setup

Two shapes share one binary: a zero-configuration development stack with
demo data, and a production deployment that cannot run demo data, known
development passwords, or without a real database.

## Quick start (development)

```sh
python3 scripts/dev.py up
```

That builds the API, starts Postgres, waits until it is healthy, and starts
the API with the demo school seeded. Then:

```sh
curl localhost:8080/healthz
curl -H "X-Subject-ID: aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" localhost:8080/v1/me
# {"subject":"aaaaaaaa-...","roles":["admin"],"home":"/admin"}
```

Demo logins (development only, the SPA dev build uses the same pair):

| User | Password | Role |
|---|---|---|
| `admin` | `admin123*` | admin |
| `carlos.mendoza` | `docente123*` | teacher |

The seeded school: one school year with the Colombian holidays, seven
class-groups (7-1 … 11-1), five teachers with subject assignments, five
subjects, twenty students with guardians and enrollments, three roll calls
for 9-1 with marks, one warning and one fault — enough for the lists,
statistics and reports to have real data.

Every other task:

```sh
python3 scripts/dev.py logs     # follow the API
python3 scripts/dev.py down     # stop (add --volumes to wipe the database)
python3 scripts/dev.py reset    # stop and wipe: clean slate
python3 scripts/dev.py seed     # create the demo data (idempotent)
python3 scripts/dev.py test     # unit tests with the race detector
python3 scripts/dev.py testdb   # PostgreSQL integration suite
python3 scripts/dev.py lint
```

### Without Docker

```sh
python3 scripts/dev.py run   # in-memory stores, demo data seeded
```

No `DATABASE_URL` means the in-memory stores: useful for UI work, but all
data is lost on restart. When the compose database is reachable, `run` and
`seed` use it automatically.

## Configuration

`scripts/dev.py` and `compose.yaml` read `.env`; copy the documented
reference and adjust:

```sh
cp .env.example .env
```

| Variable | Default | Meaning |
|---|---|---|
| `APP_ENV` | `development` | `development` or `production`. Enforces the rules below. |
| `PORT` | `8080` | Listen port. |
| `DATABASE_URL` | — | PostgreSQL DSN. Empty means in-memory stores. |
| `ALLOWED_ORIGINS` | — | Comma-separated CORS origins. Empty means same-origin only. |
| `SEED_DEMO` | on in development | Demo school at boot. Ignored when `APP_ENV=production`. |
| `BOOTSTRAP_ADMIN_USERNAME` | — | First admin, created at boot. |
| `BOOTSTRAP_ADMIN_EMAIL` | — | Optional email for that account. |
| `BOOTSTRAP_ADMIN_PASSWORD` | — | Password for that account. |
| `BOOTSTRAP_ADMIN_PASSWORD_FILE` | — | Read the password from a file instead. Wins over the value above. |
| `SEED_ADMINS`, `SEED_TEACHERS` | — | Legacy: grant roles to raw UUIDs (see below). |

A malformed value is a startup error, never a silent default: an unknown
`APP_ENV`, a non-boolean `SEED_DEMO`, or a bootstrap username without its
password all refuse to boot. A half-configured bootstrap would otherwise
leave an installation nobody can administer.

### The first admin

An empty installation has nobody who can create users, so
`BOOTSTRAP_ADMIN_*` creates one account at boot and grants it the admin
role. Both steps are idempotent: an existing account is left alone (its
password is never reset), only the role is ensured. Remove the variables
once real accounts exist.

Prefer `BOOTSTRAP_ADMIN_PASSWORD_FILE` where your platform provides
secrets (docker secrets, Kubernetes) so the password never appears in
`docker inspect` or a process listing.

### Production rules

`APP_ENV=production` refuses to start when:

- `DATABASE_URL` is empty — an in-memory deployment loses every restart.
- `SEED_DEMO` is on — it is forced off regardless of the value.
- `BOOTSTRAP_ADMIN_PASSWORD` is one of the shipped development passwords
  (`admin123*`, `docente123*`, `temporal123*`).

```sh
APP_ENV=production \
DATABASE_URL=postgres://user:pass@db:5432/gradedb?sslmode=require \
BOOTSTRAP_ADMIN_USERNAME=directora \
BOOTSTRAP_ADMIN_PASSWORD_FILE=/run/secrets/bootstrap_admin_password \
ALLOWED_ORIGINS=https://observador.colegio.edu \
python3 scripts/dev.py up
```

The Docker image already sets `APP_ENV=production`; override `DATABASE_URL`
and the bootstrap variables through your platform's secret store.

## Image

`Dockerfile` is a two-stage build: a static `CGO_ENABLED=0` binary, then
`alpine:3.20` with only `ca-certificates` (outbound TLS) and `tzdata`
(Colombia dates). The API runs as a non-root user (uid 10001) and carries a
`HEALTHCHECK` hitting `/healthz`, so orchestrators can use the built-in
probe. The schema is applied at connect time and is idempotent, so the only
deploy step is starting the new image.

`.dockerignore` keeps `.env` and the docs out of the build context, so a
local secret file cannot end up in an image layer.

## What still needs work before real data

Honest status, so nobody assumes this is production-ready:

- **No authentication tokens.** Identity is still the `X-Subject-ID`
  placeholder header; the `users` table stores real accounts with bcrypt
  passwords, but nothing issues a session yet. Until that lands, treat
  `ALLOWED_ORIGINS` and network placement as the only access controls.
  See `docs/users.md`.
- **Roles live in memory.** `POST /v1/roles` and the bootstrap grants are
  lost on restart (the Postgres adapter for `roles.Store` lands with the
  auth phase). Demo data re-seeds the two dev roles, which hides this.
- **No audit log.** Colombian law (Ley 1581 de 2012) requires recording
  who read and wrote student data. Not implemented yet; it belongs with the
  auth phase.
- **Notifications are not wired.** WhatsApp delivery to guardians is still
  on the roadmap.
- **Frontend must be served from somewhere.** It is a static SPA; for a
  same-origin production setup, put it behind a reverse proxy alongside the
  API, or set `ALLOWED_ORIGINS` to its real origin.

## Legacy: `SEED_ADMINS` / `SEED_TEACHERS`

These grant roles to raw subject UUIDs and exist only for the placeholder
identity, before real authentication. They are a development hook: roles do
not survive a restart, and anyone holding a listed id is admin. Prefer
`POST /v1/users` (creates the account and grants the role) or the bootstrap
admin.
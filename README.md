# grade backend

REST API for school student-history management (attendance, faults and
misbehavior records, WhatsApp notifications). Written in idiomatic Go using a
screaming architecture: code is organized by business capability, not by layer.

## Requirements

- Go 1.26.5
- Python 3.8+ (only for the task runner)
- Docker (runs the local stack: Postgres + the API image)

## Quick start

```sh
python3 scripts/dev.py up
```

Builds the API, starts Postgres, and starts the API with a demo school
seeded, so there is something to click through right away.

```sh
curl localhost:8080/healthz
curl -H "X-Subject-ID: aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" localhost:8080/v1/me
```

Demo logins (development only): `admin` / `admin123*` and
`carlos.mendoza` / `docente123*`. The seeded school has a school year with
Colombian holidays, seven class-groups, five teachers, five subjects, twenty
students with enrollments, roll calls with marks, a warning and a fault.

Without Docker, `python3 scripts/dev.py run` starts the API on the in-memory
stores with the same demo data (lost on every restart).

## Commands

```sh
python3 scripts/dev.py up      # start the stack (db + api, demo data)
python3 scripts/dev.py logs    # follow the api logs
python3 scripts/dev.py down    # stop (add --volumes to wipe the database)
python3 scripts/dev.py reset   # stop and wipe: clean slate
python3 scripts/dev.py seed    # create the demo data (idempotent)
python3 scripts/dev.py run     # run the API locally (PORT env, default 8080)
python3 scripts/dev.py test    # run all tests with the race detector
python3 scripts/dev.py testdb  # run the Postgres integration suite
python3 scripts/dev.py lint    # gofmt + go vet
python3 scripts/dev.py tidy    # go mod tidy
```

## Configuration

`.env` (gitignored) holds the settings; `.env.example` documents every
variable: `APP_ENV`, `PORT`, `DATABASE_URL`, `ALLOWED_ORIGINS`, `SEED_DEMO`,
`BOOTSTRAP_ADMIN_*`. Compose reads it automatically.

`APP_ENV=production` refuses to start without `DATABASE_URL`, forces
`SEED_DEMO` off, and rejects the shipped development passwords for the
bootstrap admin. The first admin is created at boot from
`BOOTSTRAP_ADMIN_*` (or `..._PASSWORD_FILE` for secret stores); an existing
account is never overwritten. Identity is still the placeholder
`X-Subject-ID` header until the token scheme lands, although the `users`
table already stores real accounts with bcrypt passwords.

The schema in `src/platform/postgres/schema.sql` applies automatically on
connect and is idempotent, so deploying is just starting the new image.

Full instructions, including the honest list of what still needs work before
real data (tokens, persistent roles, audit log): `docs/deploy.md`.

## Structure

```
src/
  cmd/server/      # the API binary: config, bootstrap, HTTP router
  cmd/devseed/     # the demo data CLI (idempotent)
  app/             # composition root: adapter selection + service wiring
  devseed/         # demo school generator (development only)
  core/            # business capabilities (screaming architecture)
    roles/         # role system: Role, permission matrix, Store port, Authorizer
    users/         # login accounts: username/email, bcrypt password hash, Active flag
    students/      # student profiles: ficha completa, foto, acudiente, hermanos
    attendance/    # assistance history: inasistencia / evasión / atraso
    incidents/     # faults graded leve / normal / grave
    warnings/      # llamados de atención leve / medio / grave con snapshot
    teachers/      # teacher profiles: documento, contacto, salud, dirección de grupo
    subjects/      # subject catalog + teacher assignment timeline
    schoolyears/   # años lectivos: periodos, fechas, festivos
    classes/       # salones por año: grado 1-11 + grupo
    enrollments/   # matrículas por salón + auditoría de promociones
    schedules/     # horarios semanales por salón
    sessions/      # llamados a lista con nómina congelada + revisiones
    statistics/    # informes agregados por salón y estudiante (solo lectura)
  platform/        # cross-cutting infrastructure
    config/        # environment configuration
    http/          # chi router, locale middleware, error responses
    i18n/          # multilanguage support (go-i18n/v2), catalogs
    postgres/      # pgx/v5 adapters for the core Store ports + schema
  tests/           # external test packages, one folder per package under test
```

User-facing text is never hardcoded: it lives in the i18n catalogs
(`src/platform/i18n/catalogs/`). Spanish is the only shipped locale; code and
internal error text stay in English.

See `docs/architecture.md` for the design principles,
`docs/deploy.md` for setup and deployment,
`docs/users.md` for login accounts and passwords,
`docs/students.md` for the student profile and history feature,
`docs/teachers.md` for teachers and subjects, and
`docs/school.md` for years, class-groups and enrollments.
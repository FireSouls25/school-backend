# grade backend

REST API for school student-history management (attendance, faults and
misbehavior records, WhatsApp notifications). Written in idiomatic Go using a
screaming architecture: code is organized by business capability, not by layer.

## Requirements

- Go 1.26.5
- Python 3.8+ (only for the task runner)
- PostgreSQL for production (set `DATABASE_URL`); dev falls back to in-memory
  stores. Integration tests use `TEST_DATABASE_URL`.
- Docker (only to run the local test database, nothing else is containerized)

## Commands

```sh
python3 scripts/dev.py run     # start the API server (PORT env, default 8080)
python3 scripts/dev.py test    # run all tests with the race detector
python3 scripts/dev.py testdb  # run the Postgres integration suite
python3 scripts/dev.py lint    # gofmt + go vet
python3 scripts/dev.py tidy    # go mod tidy
```

## Local test database

`compose.yaml` runs a single Postgres for tests and manual trials:

```sh
docker compose up -d            # start Postgres on localhost:5433
python3 scripts/dev.py testdb   # integration suite (uses the compose DSN
                                # unless TEST_DATABASE_URL is already set)
```

Point the API at it with
`DATABASE_URL=postgres://grade:grade@localhost:5433/gradedb?sslmode=disable`.
The schema in `src/platform/postgres/schema.sql` applies automatically on
connect and is idempotent, so wiping data is just
`docker compose down -v`.

## Structure

```
src/
  cmd/server/      # composition root: wires config, stores, services, router
  core/            # business capabilities (screaming architecture)
    roles/         # role system: Role, permission matrix, Store port, Authorizer
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
`docs/students.md` for the student profile and history feature,
`docs/teachers.md` for teachers and subjects, and
`docs/school.md` for years, class-groups and enrollments.

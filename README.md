# grade backend

REST API for school student-history management (attendance, faults and
misbehavior records, WhatsApp notifications). Written in idiomatic Go using a
screaming architecture: code is organized by business capability, not by layer.

## Requirements

- Go 1.26.5
- Python 3.8+ (only for the task runner)
- (optional) PostgreSQL in mind for production; dev uses an in-memory store

## Commands

```sh
python3 scripts/dev.py run     # start the API server (PORT env, default 8080)
python3 scripts/dev.py test    # run all tests with the race detector
python3 scripts/dev.py lint    # gofmt + go vet
python3 scripts/dev.py tidy    # go mod tidy
```

## Structure

```
src/
  cmd/server/      # composition root: wires config, stores, services, router
  core/            # business capabilities (screaming architecture)
    roles/         # role system: Role, permission matrix, Store port, Authorizer
  platform/        # cross-cutting infrastructure
    config/        # environment configuration
    http/          # chi router, locale middleware, error responses
    i18n/          # multilanguage support (go-i18n/v2), catalogs
  tests/           # external test packages, one folder per package under test
```

User-facing text is never hardcoded: it lives in the i18n catalogs
(`src/platform/i18n/catalogs/`). Spanish is the only shipped locale; code and
internal error text stay in English. See `docs/architecture.md`.

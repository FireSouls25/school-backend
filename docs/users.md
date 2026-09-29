# Users (cuentas de acceso)

This document covers the `users` capability: the real login-account table
with username and bcrypt password hash, plus its PostgreSQL persistence.
Token/session endpoints (`POST /v1/login`, …) are the next phase; this
phase ships storage, password policy and server-side verification only,
so no HTTP contract changes yet.

## Overview

A user is identified by an immutable UUID generated at creation time,
exactly like a student or a teacher. The `User.ID` **is** the subject
identifier consumed by the role system: granting a role to a user means
assigning it to `User.ID`. Users never imports roles; the composition
root wires both.

The account keeps:

- **Identity**: unique `Username` (e.g. `admin`, `carlos.mendoza`),
  stored lowercase; optional unique `Email` (empty = none).
- **Credential**: `PasswordHash`, a bcrypt hash. Plaintext passwords only
  cross the `Service` boundary (`Create` / `Authenticate` / `SetPassword`)
  and are never persisted. `PasswordHash` is excluded from JSON
  (`json:"-"`) and every public `Service` method returns sanitized users
  (empty hash); only the `Store` sees hashes.
- **Lifecycle**: `Active` flag. Disabling flips it to false instead of
  deleting the account, so history keeps resolving. Disabled users fail
  authentication exactly like bad credentials (no enumeration).
- **Timestamps**: `CreatedAt` / `UpdatedAt`.

## Layout

```
src/core/users/          account: model, Store port, Service, MemoryStore
src/platform/postgres/   production adapter (UsersStore) + embedded schema.sql
src/tests/users/         service tests (policy, duplicates, authentication)
src/tests/postgres/      TestUsersStoreRoundTrip (round trip + duplicate mapping)
```

## Ports

The capability owns its persistence interface; the adapter is injected
from the composition root.

```go
// users.Store
Create(ctx, User) (User, error)              // enforces unique username / email
ByID(ctx, id string) (User, error)           // ErrNotFound when missing
ByUsername(ctx, username string) (User, error) // case-insensitive, ErrNotFound
ByEmail(ctx, email string) (User, error)       // case-insensitive, ErrNotFound
Update(ctx, User) (User, error)              // username/email/active only, never the hash
SetPasswordHash(ctx, id, hash string) error  // ErrNotFound when missing
Delete(ctx, id string) error                 // no-op when absent
```

`Service` wraps the store with validation, UUID assignment
(`google/uuid`) and bcrypt hashing (`golang.org/x/crypto/bcrypt`,
a well-maintained library instead of a custom implementation):

```go
// users.Service
Create(ctx, username, email, password string) (User, error)
Authenticate(ctx, identifier, password string) (User, error)
ByID(ctx, id string) (User, error)
ByUsername(ctx, username string) (User, error) // admin tooling; login uses Authenticate
SetPassword(ctx, id, newPassword string) error
SetActive(ctx, id string, active bool) (User, error)
Delete(ctx, id string) error
```

All public results are sanitized (no hash). Domain errors implement the
i18n `Coded` contract, e.g. `users.ErrNotFound` → catalog key
`users.err_not_found`.

### Policy

- **Username**: 3–32 chars, lowercase canonical, `^[a-z0-9][a-z0-9._-]*$`
  (matches the frontend `nombre.apellido` convention and the `admin`
  account). Collisions (e.g. two `ana.gomez`) are rejected with
  `ErrDuplicateUsername`; disambiguation (second-surname letters) stays a
  school-admin decision, enforced by uniqueness.
- **Email**: optional, validated with `net/mail` when present, unique
  when non-empty (case-insensitive).
- **Password**: 8–72 bytes (bcrypt limit), at least one letter and one
  number. The existing dev passwords (`admin123*`, `docente123*`) satisfy
  it so school accounts keep working.
- **Cost**: `bcrypt.DefaultCost` in production (`NewService`). Tests use
  `NewServiceWithCost(store, bcrypt.MinCost)` to stay fast; it clamps to
  `bcrypt.MinCost..bcrypt.MaxCost` and is documented as test-only.
- **Authentication**: `identifier` containing `@` looks up by email,
  otherwise by username. Unknown identifier, wrong password and inactive
  account all return `ErrInvalidCredentials` (HTTP 401) without revealing
  which field failed.

## Database schema

`src/platform/postgres/schema.sql` is embedded and applied idempotently
at connect time.

```
users(id uuid PK, username, email, password_hash, active,
      created_at, updated_at)
UNIQUE (lower(username))                  -- idx_users_username_unique
UNIQUE (lower(email)) WHERE email <> ''   -- idx_users_email_unique
```

The partial email index keeps "no email" (`''`, the codebase convention
for absent optional strings) from colliding. The adapter translates
unique violations (`23505`) on each index into `ErrDuplicateUsername` /
`ErrDuplicateEmail`, race-safe behind the service-level checks (which
keep memory stores consistent).

## Configuration

- `DATABASE_URL`: as usual, selects the pgx adapter; empty falls back to
  `MemoryStore` (development only).
- `BOOTSTRAP_ADMIN_USERNAME` + `BOOTSTRAP_ADMIN_PASSWORD` (required),
  `BOOTSTRAP_ADMIN_EMAIL` (optional): create the first login account at
  boot and grant it the admin role. Solves the chicken-and-egg problem
  (nobody can grant the first role through the API before an admin
  exists). Existing accounts are never modified: when the username
  already exists only the admin role is ensured. Rotate the password
  after the first login (`SetPassword`). Never ship a weak bootstrap
  password in production.

```sh
DATABASE_URL=postgres://grade:grade@localhost:5433/gradedb?sslmode=disable \
BOOTSTRAP_ADMIN_USERNAME=admin \
BOOTSTRAP_ADMIN_EMAIL=admin@observador.edu.co \
BOOTSTRAP_ADMIN_PASSWORD='admin123*' \
python3 scripts/dev.py run
```

`SEED_ADMINS` / `SEED_TEACHERS` keep working unchanged: they grant roles
to subject UUIDs, and a user id is a valid subject id.

## Error codes and HTTP mapping

| Code | HTTP |
|---|---|
| `users.err_not_found` | 404 |
| `users.err_invalid_id` / `err_invalid_username` / `err_invalid_email` / `err_weak_password` / `err_duplicate_username` / `err_duplicate_email` | 400 |
| `users.err_invalid_credentials` | 401 |

All messages are localized through `src/platform/i18n/catalogs/es.json`.
`PasswordHash` never leaves the server (`json:"-"` plus sanitized
service results), so no error path can leak it.

## Data protection notes (Ley 1581 de 2012)

- Credentials are sensitive personal data: only bcrypt hashes are
  stored, plaintext never touches the database, logs or error messages.
- Authentication failures are uniform (`ErrInvalidCredentials`) to block
  account enumeration.
- Deleting a user removes the login only; student/teacher profiles are
  separate aggregates with their own lifecycle.
- Audit logging of reads/writes is still pending (see roadmap).

## What's next (auth phase, not this change)

- `POST /v1/login {"identifier","password"}` → token + expiry + subject
  + roles + home; `POST /v1/logout` (revoke); `POST /v1/refresh`.
- The token replaces the `X-Subject-ID` header **without touching
  handlers**: the context-subject middleware stays the only source of
  the subject, as `docs/api.md` anticipates.
- Throttling on login, access audit, per-user language preference.
- Roles in Postgres so grants survive restarts (today `MemoryStore` +
  `SEED_*`; unchanged by this phase).

## Cambios pendientes en el frontend (documentados, NO implementados)

Por pedido explícito no se tocó `frontend/`. Cuando el backend exponga
el login, estos son los cambios correspondientes, todos del lado del
frontend:

1. **`src/domains/auth/accounts.ts`**: borrar el directorio de cuentas
   dev (`devAccounts`, `checkDevPassword`). La contraseña deja de
   compararse en el cliente.
2. **`src/domains/auth/Login.svelte`**: sustituir
   `resolveDevAccount`/`checkDevPassword` por `POST /v1/login
   {"identifier","password"}`. Mapear `401
   users.err_invalid_credentials` al mensaje de credenciales inválidas
   (sin distinguir usuario de contraseña); `429` futuro a "intente más
   tarde".
3. **`src/domains/auth/session.ts`**: guardar token + expiración en vez
   del id de sujeto (`localStorage`; la llave `grado.subject` se
   reemplaza por `grado.token` o equivalente).
4. **`src/lib/api/client.ts`**: enviar `Authorization: Bearer <token>`;
   mantener `X-Subject-ID` solo si `VITE_DEV_SUBJECT_ID` está puesto
   (modo dev). Refrescar el token antes de que expire (`POST
   /v1/refresh`) o redirigir a `/login` al primer `401`.
5. **`src/lib/nav.ts` (`guarded()`)**: sin cambios — los códigos
   `401`/`403` y el destino (`GET /v1/me` → `home`) ya están
   implementados.
6. **Borrador del login**: se mantiene (solo identificador, nunca la
   contraseña), igual que hoy.

# Users (cuentas de acceso)

This document covers the `users` capability: the real login-account table
with username and bcrypt password hash, plus its PostgreSQL persistence.
Token/session endpoints live in `src/core/auth` and `src/platform/http`
(login, refresh rotation, recovery, lockout); the full HTTP contract is
in `docs/api.md`.

## Overview

A user is identified by an immutable UUID generated at creation time,
exactly like a student or a teacher. The `User.ID` **is** the subject
identifier consumed by the role system: granting a role to a user means
assigning it to `User.ID`. Users never imports roles; the composition
root wires both.

The account keeps:

- **Identity**: unique `Username` (e.g. `admin`, `docente`),
  stored lowercase; optional unique `Email` (empty = none).
- **Credential**: `PasswordHash`, a bcrypt hash. Plaintext passwords only
  cross the `Service` boundary (`Create` / `Authenticate` / `SetPassword`)
  and are never persisted. `PasswordHash` is excluded from JSON
  (`json:"-"`) and every public `Service` method returns sanitized users
  (empty hash); only the `Store` sees hashes.
- **Lifecycle**: `Active` flag plus `DeletedAt` timestamp. Disabling
  flips `Active` to false instead of deleting the account, so history
  keeps resolving. Disabled users fail authentication exactly like bad
  credentials (no enumeration).
- **Soft delete with retention**: `DELETE /v1/users/{id}` (owner-only)
  stamps `DeletedAt` and disables the login instead of removing the row.
  The account stops working immediately, but its data survives
  `USER_RETENTION_DAYS` (default 7) for a possible restore
  (`POST /v1/users/{id}/restore`). A scheduled purge hard-deletes expired
  rows and cleans their role grants; the audit trail and snapshots keep
  resolving because they reference users by UUID.
- **Timestamps**: `CreatedAt` / `UpdatedAt`.
- **Photo**: own account picture (admins), capped at 2 MiB like every
  other photo store. It travels through `Photo` / `SetPhoto` and lives
  outside the `User` struct, so accounts never drag image bytes.
  Docentes vinculados keep theirs in `teachers.photo`: the staff profile
  stays the single source. Student logins cannot resolve anywhere yet —
  no `StudentID` link exists — so their photos stay pending.

## Layout

```
src/core/users/          account: model, Store port, Service, MemoryStore
src/devseed/             demo accounts seeded on boot (development only)
src/platform/postgres/   production adapter (UsersStore) + embedded schema.sql
src/tests/users/         service tests (policy, duplicates, authentication)
src/tests/devseed/       demo seeding: consistency and idempotency
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
Update(ctx, User) (User, error)              // username/email/active/teacher link, never the hash
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
ChangePassword(ctx, id, currentPassword, newPassword string) error // verifies current first (used by PUT /v1/me/password)
Photo(ctx, id string) (Photo, error)             // account picture; empty when none
SetPhoto(ctx, id string, photo Photo) error      // 2 MiB cap; empty data clears
SetActive(ctx, id string, active bool) (User, error)
LinkTeacher(ctx, userID, teacherID string) (User, error) // binds login to profile
UnlinkTeacher(ctx, userID string) (User, error)         // back to a pure account
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
- **Teacher link**: `TeacherID` binds a login to a teacher profile so
  `GET /v1/teachers/me/schedule` resolves the caller (dangling ids
  resolve to no schedule, never an error). Set at seed time for the
  demo teacher; link management over HTTP arrives with the login flow.

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

Prefer `BOOTSTRAP_ADMIN_PASSWORD_FILE` where the platform provides secrets
(docker secrets, Kubernetes): it takes precedence over the env var, so the
password never shows up in `docker inspect` or a process listing.
`APP_ENV=production` rejects the development passwords above, so a real
deployment must set its own; see `docs/deploy.md`.

`BOOTSTRAP_OWNER_*` mirrors `BOOTSTRAP_ADMIN_*` (username, email,
password or `_PASSWORD_FILE`; half-configured pairs fail boot, known
dev passwords are rejected in production) and grants the `owner` role:
the developers' account in production and the only role holding
`manage-owners`. `USER_RETENTION_DAYS` (default 7, minimum 1) sizes the
soft-delete window.

`SEED_ADMINS` / `SEED_TEACHERS` keep working unchanged: they grant roles
to subject UUIDs, and a user id is a valid subject id. They are a
development hook (roles live in memory until the auth phase).

### Demo data

`SEED_DEMO` (on by default in development, forced off in production) seeds
the demo school through `src/devseed`, including these two accounts, so a
developer can sign in without setting anything by hand:

| User | Password | Role |
|---|---|---|
| `admin` | `admin123*` | admin |
| `docente` | `docente123*` | teacher |
| `secretaria` | `secretaria123*` | secretary |
| `estudiante` | `estudiante123*` | student |

Both carry the fixed UUIDs the SPA dev login mock sends. Seeding is
idempotent, and an account that already exists (say a bootstrap admin named
`admin`) is reused rather than duplicated.

## Error codes and HTTP mapping

| Code | HTTP |
|---|---|
| `users.err_not_found` | 404 |
| `users.err_invalid_id` / `err_invalid_username` / `err_invalid_email` / `err_weak_password` / `err_duplicate_username` / `err_duplicate_email` / `err_not_deleted` | 400 |
| `users.err_invalid_credentials` | 401 |
| `users.err_self_delete` | 403 |
| `users.err_last_owner` / `err_already_deleted` | 409 |
| `users.err_retention_expired` | 410 |

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

## HTTP (implementado)

`POST /v1/users` (permiso `manage-users`) crea la cuenta de acceso de un
docente: sin registro público, el admin le pone la contraseña inicial.
Crear con `role: "owner"` exige además `manage-owners` (solo owners).

- `GET /v1/users` (`manage-users`): roster de cuentas con sus roles;
  filtros `?role=` e `?includeDeleted=` (el bin de retención solo lo ve
  un owner).
- `GET /v1/users/{id}` (`manage-users`): ficha con roles y `DeletedAt`;
  las eliminadas responden 404 salvo para owners.
- `DELETE /v1/users/{id}` (`manage-owners`): borrado suave con retención.
  Ni la propia cuenta (403) ni el último owner activo (409) pueden
  eliminarse.
- `POST /v1/users/{id}/restore` (`manage-owners`): restaura dentro de la
  ventana de retención.
- `DELETE /v1/roles` (`manage-roles`, o `manage-owners` para revocar
  `admin`/`owner`): revoca un rol; quitar el último owner da 409.
- `PUT /v1/users/{id}/active` (`block-access`, `manage-owners` para
  objetivos admin/owner): el secretario sigue sin poder tocar admins.

```json
// POST /v1/users {"username","email","password","role?"}
{
  "ID": "uuid de la cuenta (subject para roles)",
  "Username": "laura.torres",
  "Email": "laura.torres@observador.edu.co",
  "Active": true,
  "Roles": ["teacher"]
}
```

- El cuerpo es JSON estricto (desconocidos → `400`); la contraseña nunca
  vuelve en la respuesta.
- `"role"` es opcional (`teacher`, `admin`, `student`); otorgarlo exige
  además `manage-roles` (el admin tiene todos los permisos, así que en la
  práctica sigue siendo solo-admin).
- Errores con códigos estables: `users.err_duplicate_username` /
  `err_duplicate_email` / `err_weak_password` (`400`),
  `roles.err_unknown_role` (`400`).

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

Por pedido explícito no se tocó `frontend/`. Estado: `POST /v1/users`
ya existe (el admin crea la cuenta del docente con su contraseña
inicial); el login con token sigue pendiente. Cuando el backend exponga
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

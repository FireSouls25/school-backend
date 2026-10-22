# Auditoría (bitácora de acceso a datos)

Referencia de la capability `audit`: registra **quién tocó datos de
estudiantes, qué hizo y cómo terminó** (Ley 1581 de 2012, datos de
menores trazables). La consola admin (Bitácora) y la ficha por
estudiante la leen; los intentos denegados también quedan.

## Qué se registra

Los handlers llaman a `recordAudit` con la acción real (incluido el
`status` final, así que lo denegado queda visible). Cada entrada:

| Campo | Significado |
|---|---|
| `SubjectID` | quien actuó (siempre del contexto, nunca del body) |
| `Action` | `read`, `create`, `update`, `mark`, `delete` |
| `Resource` | `student`, `warning`, `session`, `report`, `schedule`, `subject`, `assignment`, `group`, `teacher-attendance` |
| `ResourceID` | registro afectado (vacío cuando no aplica) |
| `StudentID` | estudiante afectado (vacío en catálogos/horarios) |
| `Status` | código HTTP final |
| `HappenedAt` | sellado por el servicio |

## Garantías de diseño

- **No rompe la petición**: si la bitácora falla, se escribe `slog.Warn`
  y la respuesta original se entrega igual.
- **Append-only**: `audit_entries` no tiene llaves foráneas; sobrevive al
  borrado del registro o de la cuenta.
- **Solo lectura por admin**: `GET /v1/audit` exige `manage-system`,
  `?studentID=<uuid>` filtra la ficha y `?limit=N` topa (defecto 50,
  máximo 200).

## Intentos de login (auditoría auth, tabla aparte)

`login_attempts` (`bucket` = identificador|IP, `ip`, `success`, `at`)
guarda cada intento de login/refresh: es el presupuesto del throttle
**y** su auditoría. Sin endpoint de lectura (el admin consulta la DB);
ver `Threat model` en `docs/api.md`.

## Configuración y persistencia

- `DATABASE_URL` vacío → bitácora en memoria (solo desarrollo).
- Tablas `audit_entries` y `login_attempts` se crean con el resto del
  schema idempotente (`ADD COLUMN IF NOT EXISTS`, índices por fecha y
  estudiante); `login_attempts` se purga por ventana.

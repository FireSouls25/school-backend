# Auditoría (bitácora de acceso a datos de estudiantes)

Documento de referencia. La capability `audit` registra **quién leyó o
escribió datos de estudiantes, qué tocó y cómo terminó**. Es un requisito
de la Ley 1581 de 2012: los datos de menores deben ser trazables.

## Qué se registra

El middleware HTTP (`src/platform/http/audit.go`) clasifica la **ruta
coincidente** (nunca la URL cruda) y guarda una entrada con:

| Campo | Significado |
|---|---|
| `SubjectID` | el sujeto autenticado que actuó (siempre del contexto, nunca del body) |
| `Action` | `read`, `create`, `update`, `mark` |
| `Resource` | `student`, `warning`, `session`, `report` |
| `ResourceID` | registro afectado (vacío cuando no aplica) |
| `Status` | código HTTP final: los **denegados también quedan** |
| `HappenedAt` | sellado por el servicio, nunca por el llamador |

Rutas auditadas: `GET /v1/students/{id}`,
`GET /v1/students/{id}/warnings`, `POST /v1/students`,
`POST /v1/sessions/{sessionID}/marks`,
`GET /v1/statistics/student/{studentID}` y
`GET /v1/statistics/class/{groupID}`.

Rutas que **no** se registran (no tocan datos de estudiantes): `/v1/me`,
docentes, roles, catálogos, login.

## Garantías de diseño

- **No rompe la petición**: si la bitácora falla, se escribe un `slog.Warn`
  y la respuesta original se entrega igual. Una auditoría caída no puede
  dejar el colegio sin clase.
- **Sin autor no hay entrada**: un acceso sin sujeto no se registra, porque
  una entrada sin autor no prueba nada.
- **Append-only**: la tabla no tiene llaves foráneas a propósito; la
  bitácora sobrevive al borrado del registro o de la cuenta.
- **Solo lectura por admin**: `GET /v1/audit` exige `manage-system`.
  Parámetro opcional `?studentID=<uuid>` para la bitácora de un estudiante
  y `?limit=N` (por defecto 50, tope 200).

## Configuración y persistencia

- `DATABASE_URL` selecciona el adaptador `audit_log`; sin él, la bitácora
  vive en memoria (solo desarrollo).
- La tabla se crea sola con el resto del schema idempotente:

```
audit_log(id uuid PK, subject_id uuid, action text, resource text,
          resource_id text, status int, happened_at timestamptz)
```

## Qué falta

- **Lectura en el frontend**: no hay pantalla de auditoría; el endpoint
  ya existe (`GET /v1/audit`) para que la consola la muestre.
- **Actualizaciones**: `ActionUpdate` está en el catálogo pero no hay
  rutas de modificación todavía, así que aún no se registra ninguna.
- **Retención**: la bitácora crece sin límite; falta una política de
  purga acordada con el colegio.
- **Autenticación**: mientras el sujeto siga siendo un header confiable,
  la bitácora registra *lo que el servidor creyó*, no una identidad
  verificada. Se cierra con `POST /v1/login`.
# Generador de horarios: procedimientos necesarios

Documento de diseño (sin código): qué hay que construir para pasar del
horario manual actual a un generador automático con carga docente.

## 1. Punto de partida (lo que ya existe)

- `schedules.Entry`: `{ClassGroupID, TeacherID, SubjectID, Weekday (1-5),
  Start, End}` con `Clock` en minutos. La API acepta `"HH:MM"`.
- Validación vigente (`core/schedules/service.go`): solape de docente
  (`ErrTeacherConflict`), solape de salón (`ErrClassConflict`), jornada
  07:00–14:00 y duración mínima 60 min. El tablero (`ScheduleBoard`)
  crea/edita/borra franjas una por una contra estas reglas.
- `subjects.Subject` ya trae `Grades []int` (en qué grados se dicta) y
  la asignación docente↔materia (`Subjects.Assign`, open/closed por
  fecha) con guard `checkTeaches`: un docente solo dicta lo asignado.
- `teachers.Teacher` **no** tiene carga horaria ni materias por grado:
  eso hay que modelarlo (sección 2).
- El tablero de distribución de salones ya demuestra el patrón de
  interacción que queremos reutilizar: proponer, ajustar a mano y
  confirmar explícitamente. El generador **nunca auto-aplica**: propone
  una parrilla editable y el admin la guarda por bloques.

## 2. Modelo de datos nuevo (migraciones + core)

1. **Plan de estudios** (`study_plans`, nuevo core o dentro de
   `schedules`): por grado, lista de `{subjectID, weeklyHours}`.
   - Regla: toda materia del plan debe existir en el catálogo y tener
     al menos un docente asignado en ese grado (si no, el generador lo
     reporta como faltante en vez de inventar).
2. **Carga del docente**: campo nuevo en `teachers`
   (`weekly_hours INT, default 40`, editable en la ficha) = tope de
   horas/semana para el generador. No es control de nómina: es el
   presupuesto que el algoritmo no puede exceder.
3. **Bloque de clase**: `min_minutes` / `max_minutes` configurables
   por colegio (hoy fijos en 60 min e implícitos en el validador).
   Toda franja generada parte la carga semanal en bloques dentro de
   ese rango.
4. **Restricción diaria**: una misma materia no se repite el mismo día
   en el mismo salón (nueva regla en `validate`, con su código
   `schedules.err_duplicate_subject_day`).

## 3. Pantallas necesarias (frontend, rol admin + secretaría)

1. **Docentes: qué imparten y dónde.** Vista derivable de datos
   actuales (`GET /v1/schedule` + catálogo): tabla docente → materias
   → salones, editable (redirige al tablero existente para mover
   bloques). Nueva: editar `weekly_hours` en la ficha del docente.
2. **Plan por grado.** Por cada grado (1–11): materias del plan con
   sus horas semanales (number inputs, suma visible vs. jornada
   disponible Lun–Vie 07:00–14:00 = 35 h). Validar que la suma quepa;
   si no, bloquear la generación con mensaje claro.
3. **Parámetros de generación**: min/max por bloque, semana objetivo
   (año lectivo), botón **Generar propuesta**.
4. **Propuesta editable**: parrilla Lun–Vie por salón con los bloques
   propuestos, conflictos marcados (docente/salón ocupado, materia
   repetida, docente sobrecargado). Guardar por bloques (reusa
   `POST /v1/schedule`, que ya valida todo): lo guardado queda firme,
   lo no guardado se descarta.

## 4. Procedimiento del motor (backend, endpoint nuevo)

`POST /v1/schedule/suggest {year, minMinutes, maxMinutes}` responde la
propuesta sin persistir nada:

1. Cargar salones del año, plan por grado, asignaciones abiertas y
   cargas de docentes.
2. Descomponer cada materia en bloques: agotar `weeklyHours` en piezas
   de `maxMinutes` (última pieza ≥ `minMinutes`; si no cuadra, fallar
   con mensaje por grado/materia, no aproximar en silencio).
3. Ordenar bloques por restricción (materias con menos docentes
   disponibles primero, luego mayor carga).
4. Colocar en el primer hueco válido Lun–Vie que cumpla, en orden:
   salón libre, docente libre, docente bajo su tope, materia no usada
   ese día en ese salón, dentro de 07:00–14:00.
5. Si un bloque no cabe en ningún hueco, devolverlo en
   `unplaced[]` con el motivo (sin docente, sin hueco, sobrecarga) en
   vez de forzar. Propuesta parcial válida > todo o nada.
6. Responder `{placements[], unplaced[], warnings[]}`; el frontend
   pinta la parrilla y el admin guarda lo que acepte.

Complejidad: greedy con backtracking acotado basta para ~7 salones ×
35 h; si un colegio grande lo exige, se documenta como fase 2 (solver
externo). No prometer optimalidad: el contrato es "propuesta válida y
explicable".

## 5. Fases de entrega

- **Fase 1 (configuración)**: plan por grado + horas por docente +
  min/max por bloque + regla de no-repetición diaria. Todo manual,
  todo validado. Sin motor.
- **Fase 2 (propuesta)**: endpoint suggest + parrilla editable +
  guardado por bloques. La heurística de salones balanceados ya
  implementada en Cierre sirve de referencia de tono (proponer,
  explicar, dejar ajustar).
- **Fase 3 (calidad)**: detectar patrones (huecos, días pesados),
  informe de carga real vs. tope por docente, re-generación parcial
  por salón.

## 6. Riesgos y decisiones tomadas

- El generador no toca Auth ni permisos nuevos: usa `manage-schedules`
  (admin + secretaría), igual que el tablero manual.
- Cerrar el año congela lo generado como cualquier otro registro
  (bloqueo ya implementado): generar solo tiene sentido sobre el año
  lectivo abierto.
- La API de horarios acepta `?weekday=` suelto desde el cambio de la
  vista "Hoy": útil para previsualizar la parrilla generada por día.

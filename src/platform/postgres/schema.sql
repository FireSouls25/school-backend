-- Schema for the grade backend. Idempotent: safe to run on every start.
-- Fresh installs get the full tables; existing installs are migrated with
-- ADD COLUMN IF NOT EXISTS below.

CREATE TABLE IF NOT EXISTS students (
    id                  UUID PRIMARY KEY,
    names               TEXT        NOT NULL,
    surnames            TEXT        NOT NULL,
    class_id            TEXT        NOT NULL,
    document_id         TEXT        NOT NULL DEFAULT '',
    phone               TEXT        NOT NULL DEFAULT '',
    address             TEXT        NOT NULL DEFAULT '',
    birthplace          TEXT        NOT NULL DEFAULT '',
    birthdate           DATE,
    email               TEXT        NOT NULL DEFAULT '',
    vision_has          BOOLEAN     NOT NULL DEFAULT FALSE,
    vision_detail       TEXT        NOT NULL DEFAULT '',
    hearing_has         BOOLEAN     NOT NULL DEFAULT FALSE,
    hearing_detail      TEXT        NOT NULL DEFAULT '',
    blood_type          TEXT        NOT NULL DEFAULT '',
    is_new              BOOLEAN     NOT NULL DEFAULT FALSE,
    previous_school     TEXT        NOT NULL DEFAULT '',
    transfer_reason     TEXT        NOT NULL DEFAULT '',
    repeat_count        INTEGER     NOT NULL DEFAULT 0,
    mother_name         TEXT        NOT NULL DEFAULT '',
    mother_document     TEXT        NOT NULL DEFAULT '',
    mother_phone        TEXT        NOT NULL DEFAULT '',
    mother_occupation   TEXT        NOT NULL DEFAULT '',
    mother_address      TEXT        NOT NULL DEFAULT '',
    father_name         TEXT        NOT NULL DEFAULT '',
    father_document     TEXT        NOT NULL DEFAULT '',
    father_phone        TEXT        NOT NULL DEFAULT '',
    father_occupation   TEXT        NOT NULL DEFAULT '',
    father_address      TEXT        NOT NULL DEFAULT '',
    caregiver_name      TEXT        NOT NULL DEFAULT '',
    caregiver_document  TEXT        NOT NULL DEFAULT '',
    caregiver_phone     TEXT        NOT NULL DEFAULT '',
    caregiver_occupation TEXT       NOT NULL DEFAULT '',
    caregiver_address   TEXT        NOT NULL DEFAULT '',
    lives_with          TEXT        NOT NULL DEFAULT '',
    siblings            JSONB       NOT NULL DEFAULT '[]',
    medical_report      TEXT        NOT NULL DEFAULT '',
    diversity_condition TEXT        NOT NULL DEFAULT '',
    specialist_has      BOOLEAN     NOT NULL DEFAULT FALSE,
    specialist_detail   TEXT        NOT NULL DEFAULT '',
    status              TEXT        NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'graduated')),
    photo               BYTEA,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Migrate installs created before the extended profile existed.
ALTER TABLE students ADD COLUMN IF NOT EXISTS document_id TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS address TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS birthplace TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS birthdate DATE;
ALTER TABLE students ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS vision_has BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE students ADD COLUMN IF NOT EXISTS vision_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS hearing_has BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE students ADD COLUMN IF NOT EXISTS hearing_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS blood_type TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS is_new BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE students ADD COLUMN IF NOT EXISTS previous_school TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS transfer_reason TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS repeat_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE students ADD COLUMN IF NOT EXISTS mother_name TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS mother_document TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS mother_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS mother_occupation TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS mother_address TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS father_name TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS father_document TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS father_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS father_occupation TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS father_address TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS caregiver_name TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS caregiver_document TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS caregiver_phone TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS caregiver_occupation TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS caregiver_address TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS lives_with TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS siblings JSONB NOT NULL DEFAULT '[]';
ALTER TABLE students ADD COLUMN IF NOT EXISTS medical_report TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS diversity_condition TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS specialist_has BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE students ADD COLUMN IF NOT EXISTS specialist_detail TEXT NOT NULL DEFAULT '';
ALTER TABLE students ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'active';

CREATE INDEX IF NOT EXISTS idx_students_class_id ON students (class_id);

CREATE TABLE IF NOT EXISTS attendance_records (
    id         UUID PRIMARY KEY,
    student_id UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    class_id   TEXT        NOT NULL,
    date       DATE        NOT NULL,
    reason     TEXT        NOT NULL CHECK (reason IN ('absence', 'evasion'))
);

-- Admit the "late" (atraso) reason on installs created before it existed.
-- Drop + re-add keeps the statement idempotent on every start.
ALTER TABLE attendance_records DROP CONSTRAINT IF EXISTS attendance_records_reason_check;
ALTER TABLE attendance_records ADD CONSTRAINT attendance_records_reason_check
    CHECK (reason IN ('absence', 'evasion', 'late'));

CREATE INDEX IF NOT EXISTS idx_attendance_student_id ON attendance_records (student_id);

CREATE TABLE IF NOT EXISTS incident_faults (
    id          UUID PRIMARY KEY,
    student_id  UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    class_id    TEXT        NOT NULL,
    date        DATE        NOT NULL,
    severity    TEXT        NOT NULL CHECK (severity IN ('minor', 'ordinary', 'severe')),
    description TEXT        NOT NULL
);

-- Academic alerts: a teacher flags students for admin review. Open until
-- an admin reviews them; history stays per student.
CREATE TABLE IF NOT EXISTS academic_alerts (
    id          UUID PRIMARY KEY,
    student_id  UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    teacher_id  TEXT        NOT NULL,
    reason      TEXT        NOT NULL DEFAULT '',
    status      TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'reviewed')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    reviewed_at TIMESTAMPTZ,
    reviewed_by TEXT
);

CREATE INDEX IF NOT EXISTS idx_academic_alerts_open ON academic_alerts (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_academic_alerts_student ON academic_alerts (student_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_incident_faults_student_id ON incident_faults (student_id);

-- Convivencia observations: the teacher picks the situation, the
-- server owns the kind. Names are frozen snapshots.
CREATE TABLE IF NOT EXISTS observations (
    id                UUID PRIMARY KEY,
    student_id        UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    snap_student_name TEXT        NOT NULL DEFAULT '',
    teacher_id        TEXT        NOT NULL,
    class_group_id    TEXT        NOT NULL DEFAULT '',
    snap_class_label  TEXT        NOT NULL DEFAULT '',
    situation_id      TEXT        NOT NULL,
    kind              SMALLINT    NOT NULL CHECK (kind IN (1, 2, 3)),
    note              TEXT        NOT NULL DEFAULT '',
    happened_on       DATE        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_observations_teacher_id ON observations (teacher_id);

-- WhatsApp outbox: queued template sends with dedup collapsing retries
-- and double taps into a single delivery.
CREATE TABLE IF NOT EXISTS notification_outbox (
    id         UUID PRIMARY KEY,
    to_phone   TEXT        NOT NULL,
    kind       TEXT        NOT NULL,
    template   TEXT        NOT NULL,
    language   TEXT        NOT NULL DEFAULT 'es',
    params     JSONB       NOT NULL DEFAULT '[]',
    dedup_key  TEXT        NOT NULL,
    status     TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    attempts   INTEGER     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_notify_dedup_key ON notification_outbox (dedup_key);
CREATE INDEX IF NOT EXISTS idx_notify_pending ON notification_outbox (status, created_at);

CREATE TABLE IF NOT EXISTS warnings (
    id                   UUID PRIMARY KEY,
    student_id           UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    class_id             TEXT        NOT NULL,
    teacher_id           TEXT        NOT NULL,
    happened_at          TIMESTAMPTZ NOT NULL,
    gravity              TEXT        NOT NULL CHECK (gravity IN ('mild', 'moderate', 'severe')),
    title                TEXT        NOT NULL,
    description          TEXT        NOT NULL,
    snap_names           TEXT        NOT NULL DEFAULT '',
    snap_surnames        TEXT        NOT NULL DEFAULT '',
    snap_document_id     TEXT        NOT NULL DEFAULT '',
    snap_class_id        TEXT        NOT NULL DEFAULT '',
    snap_birthdate       DATE,
    snap_age             INTEGER     NOT NULL DEFAULT -1,
    snap_caregiver_name  TEXT        NOT NULL DEFAULT '',
    snap_caregiver_phone TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_warnings_student_id ON warnings (student_id);

-- Append-only trail of llamado corrections: editing never rewrites.
CREATE TABLE IF NOT EXISTS warning_revisions (
    id          UUID PRIMARY KEY,
    warning_id  UUID NOT NULL REFERENCES warnings (id) ON DELETE CASCADE,
    student_id  TEXT NOT NULL DEFAULT '',
    old_gravity TEXT NOT NULL DEFAULT '',
    new_gravity TEXT NOT NULL,
    old_title   TEXT NOT NULL DEFAULT '',
    new_title   TEXT NOT NULL DEFAULT '',
    old_detail  TEXT NOT NULL DEFAULT '',
    new_detail  TEXT NOT NULL DEFAULT '',
    changed_by  TEXT NOT NULL DEFAULT '',
    changed_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_warning_revisions_warning_id ON warning_revisions (warning_id);

CREATE TABLE IF NOT EXISTS teachers (
    id                  UUID PRIMARY KEY,
    names               TEXT        NOT NULL,
    surnames            TEXT        NOT NULL,
    document_id         TEXT        NOT NULL DEFAULT '',
    phone               TEXT        NOT NULL DEFAULT '',
    address             TEXT        NOT NULL DEFAULT '',
    birthplace          TEXT        NOT NULL DEFAULT '',
    birthdate           DATE,
    email               TEXT        NOT NULL DEFAULT '',
    sex                 TEXT        NOT NULL DEFAULT '',
    medical_conditions  TEXT        NOT NULL DEFAULT '',
    homeroom_class_id   TEXT        NOT NULL DEFAULT '',
    area_head_subject_id TEXT       NOT NULL DEFAULT '',
    active              BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Retiring a teacher flips active instead of deleting the profile.
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT TRUE;
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS photo BYTEA;
-- Sex as recorded by the school: 'female', 'male' or empty when not declared.
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS sex TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_teachers_homeroom_class_id ON teachers (homeroom_class_id);
-- Jefe de área: subject UUID the teacher heads; empty means none.
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS area_head_subject_id TEXT NOT NULL DEFAULT '';

-- Daily roll of teachers: one explicit mark per teacher per day.
CREATE TABLE IF NOT EXISTS teacher_attendance (
    teacher_id  UUID NOT NULL REFERENCES teachers (id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    status      TEXT NOT NULL CHECK (status IN ('present', 'absent', 'late', 'permission')),
    note        TEXT NOT NULL DEFAULT '',
    recorded_by TEXT NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (teacher_id, day)
);

CREATE INDEX IF NOT EXISTS idx_teacher_attendance_day ON teacher_attendance (day);

-- Append-only trail of teacher mark changes: correcting a day never
-- rewrites, it appends a revision with the status before and after.
CREATE TABLE IF NOT EXISTS teacher_attendance_revisions (
    id          UUID PRIMARY KEY,
    teacher_id  UUID NOT NULL REFERENCES teachers (id) ON DELETE CASCADE,
    day         DATE NOT NULL,
    old_status  TEXT NOT NULL DEFAULT '',
    new_status  TEXT NOT NULL CHECK (new_status IN ('present', 'absent', 'late', 'permission')),
    note        TEXT NOT NULL DEFAULT '',
    recorded_by TEXT NOT NULL DEFAULT '',
    recorded_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_teacher_attendance_revisions_teacher_day
    ON teacher_attendance_revisions (teacher_id, day);

-- Correlate warnings issued in one batch against several students.
ALTER TABLE warnings ADD COLUMN IF NOT EXISTS group_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_warnings_group_id ON warnings (group_id);


CREATE TABLE IF NOT EXISTS subjects (
    id          UUID PRIMARY KEY,
    name        TEXT        NOT NULL,
    code        TEXT        NOT NULL DEFAULT '',
    description TEXT        NOT NULL DEFAULT '',
    active      BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Subject names are unique, case-insensitively.
CREATE UNIQUE INDEX IF NOT EXISTS idx_subjects_lower_name ON subjects (lower(name));

-- Catalog phase: which years a subject is taught in, and its console color.
ALTER TABLE subjects ADD COLUMN IF NOT EXISTS grades INTEGER[] NOT NULL DEFAULT '{}';
ALTER TABLE subjects ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT '';
-- Numbered children ("1", "2") selectable per schedule entry.
ALTER TABLE subjects ADD COLUMN IF NOT EXISTS variants TEXT[] NOT NULL DEFAULT '{}';

CREATE TABLE IF NOT EXISTS subject_assignments (
    id          UUID PRIMARY KEY,
    teacher_id  UUID        NOT NULL REFERENCES teachers (id) ON DELETE CASCADE,
    subject_id  UUID        NOT NULL REFERENCES subjects (id) ON DELETE CASCADE,
    started_at  DATE        NOT NULL,
    ended_at    DATE,
    CHECK (ended_at IS NULL OR ended_at >= started_at)
);

-- A teacher cannot teach the same subject in two open assignments.
CREATE UNIQUE INDEX IF NOT EXISTS idx_assignments_open_pair
    ON subject_assignments (teacher_id, subject_id) WHERE ended_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_assignments_teacher_id ON subject_assignments (teacher_id);
CREATE INDEX IF NOT EXISTS idx_assignments_subject_id ON subject_assignments (subject_id);

CREATE TABLE IF NOT EXISTS school_years (
    id          UUID PRIMARY KEY,
    year        INTEGER     NOT NULL,
    periods     INTEGER     NOT NULL DEFAULT 3,
    start_date  DATE        NOT NULL,
    end_date    DATE        NOT NULL,
    holidays    DATE[]      NOT NULL DEFAULT '{}',
    no_class_days DATE[]  NOT NULL DEFAULT '{}',
    closed      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (periods >= 1 AND periods <= 6),
    CHECK (start_date < end_date)
);

-- Dated periods of a school year: one row per period (1-based n).
-- Periods move with the year: deleting it cascades.
CREATE TABLE IF NOT EXISTS school_year_periods (
    year_id    UUID NOT NULL REFERENCES school_years (id) ON DELETE CASCADE,
    n          INTEGER NOT NULL,
    start_date DATE NOT NULL,
    end_date   DATE NOT NULL,
    PRIMARY KEY (year_id, n)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_school_years_year ON school_years (year);

CREATE TABLE IF NOT EXISTS class_groups (
    id             UUID PRIMARY KEY,
    school_year_id UUID        NOT NULL REFERENCES school_years (id) ON DELETE RESTRICT,
    grade          INTEGER     NOT NULL CHECK (grade >= 1 AND grade <= 11),
    group_no       INTEGER     NOT NULL CHECK (group_no >= 1),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_class_groups_unique
    ON class_groups (school_year_id, grade, group_no);

-- Grade 0 is preescolar and group 0 exists (00--0 Jardín): widen the
-- original 1-based checks. DROP+ADD keeps existing databases working.
ALTER TABLE class_groups DROP CONSTRAINT IF EXISTS class_groups_grade_check;
ALTER TABLE class_groups ADD CONSTRAINT class_groups_grade_check
    CHECK (grade >= 0 AND grade <= 11);
ALTER TABLE class_groups DROP CONSTRAINT IF EXISTS class_groups_group_no_check;
ALTER TABLE class_groups ADD CONSTRAINT class_groups_group_no_check
    CHECK (group_no >= 0);

CREATE TABLE IF NOT EXISTS enrollments (
    id             UUID PRIMARY KEY,
    student_id     UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    class_group_id UUID        NOT NULL REFERENCES class_groups (id) ON DELETE RESTRICT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_enrollments_pair
    ON enrollments (student_id, class_group_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_group_id ON enrollments (class_group_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_student_id ON enrollments (student_id);

CREATE TABLE IF NOT EXISTS promotions (
    id            UUID PRIMARY KEY,
    student_id    UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    from_group_id UUID        NOT NULL REFERENCES class_groups (id) ON DELETE RESTRICT,
    to_group_id   UUID        REFERENCES class_groups (id) ON DELETE RESTRICT,
    decision      TEXT        NOT NULL CHECK (decision IN ('promote', 'repeat', 'graduate')),
    decided_by    TEXT        NOT NULL,
    decided_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_promotions_student_id ON promotions (student_id);

CREATE TABLE IF NOT EXISTS schedule_entries (
    id             UUID PRIMARY KEY,
    class_group_id UUID        NOT NULL REFERENCES class_groups (id) ON DELETE CASCADE,
    teacher_id     UUID        NOT NULL REFERENCES teachers (id) ON DELETE CASCADE,
    subject_id     UUID        NOT NULL REFERENCES subjects (id) ON DELETE CASCADE,
    weekday        SMALLINT    NOT NULL CHECK (weekday >= 0 AND weekday <= 6),
    start_min      INTEGER     NOT NULL CHECK (start_min >= 0 AND start_min < 1440),
    end_min        INTEGER     NOT NULL CHECK (end_min > 0 AND end_min < 1440),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (start_min < end_min)
);

CREATE INDEX IF NOT EXISTS idx_schedule_teacher_id ON schedule_entries (teacher_id);
CREATE INDEX IF NOT EXISTS idx_schedule_group_id ON schedule_entries (class_group_id);
CREATE INDEX IF NOT EXISTS idx_schedule_subject_id ON schedule_entries (subject_id);

-- Joint workshops: entries sharing joint_id are one lesson across salons.
ALTER TABLE schedule_entries ADD COLUMN IF NOT EXISTS joint_id UUID;
-- Numbered subject child ("1", "2") chosen for the slot, if any.
ALTER TABLE schedule_entries ADD COLUMN IF NOT EXISTS variant TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS sessions (
    id             UUID PRIMARY KEY,
    class_group_id UUID        NOT NULL REFERENCES class_groups (id) ON DELETE RESTRICT,
    class_label    TEXT        NOT NULL DEFAULT '',
    school_year    INTEGER     NOT NULL DEFAULT 0,
    teacher_id     TEXT        NOT NULL,
    subject_id     TEXT        NOT NULL DEFAULT '',
    date           DATE        NOT NULL,
    period         INTEGER     NOT NULL DEFAULT 1 CHECK (period >= 1),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_sessions_group_id ON sessions (class_group_id);
CREATE INDEX IF NOT EXISTS idx_sessions_teacher_id ON sessions (teacher_id);
CREATE INDEX IF NOT EXISTS idx_sessions_date ON sessions (date);

CREATE TABLE IF NOT EXISTS session_rosters (
    session_id  UUID NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    student_id  UUID NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    names       TEXT NOT NULL DEFAULT '',
    surnames    TEXT NOT NULL DEFAULT '',
    document_id TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (session_id, student_id)
);

CREATE INDEX IF NOT EXISTS idx_session_rosters_student_id ON session_rosters (student_id);

CREATE TABLE IF NOT EXISTS session_revisions (
    id         UUID PRIMARY KEY,
    session_id UUID        NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    rev_no     INTEGER     NOT NULL,
    student_id UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    from_mark  TEXT        NOT NULL DEFAULT ''
        CHECK (from_mark IN ('', 'absence', 'absence-permission', 'evasion', 'late')),
    to_mark    TEXT        NOT NULL DEFAULT ''
        CHECK (to_mark IN ('', 'absence', 'absence-permission', 'evasion', 'late')),
    changed_by TEXT        NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL,
    note       TEXT        NOT NULL DEFAULT '',
    UNIQUE (session_id, rev_no)
);

CREATE INDEX IF NOT EXISTS idx_session_revisions_session_id ON session_revisions (session_id);

-- Login accounts. The id doubles as the subject identifier for role
-- assignments. Passwords are stored as bcrypt hashes, never plaintext.
CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    username      TEXT        NOT NULL DEFAULT '',
    email         TEXT        NOT NULL DEFAULT '',
    password_hash TEXT        NOT NULL DEFAULT '',
    active        BOOLEAN     NOT NULL DEFAULT TRUE,
    token_version BIGINT      NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Usernames are unique, case-insensitively.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_username_unique
    ON users (lower(username));
-- Emails are unique when present, case-insensitively.
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_email_unique
    ON users (lower(email)) WHERE email <> '';

-- Account phase: link a login to its teacher profile, so "my schedule"
-- resolves without a second login. Nullable; deleting the profile
-- unlinks instead of deleting the account. Placed last so the teachers
-- table always exists for the reference.
ALTER TABLE users ADD COLUMN IF NOT EXISTS teacher_id UUID NULL REFERENCES teachers (id) ON DELETE SET NULL;

-- Account phase: own picture for logins without a linked profile
-- (admins). Linked docentes keep theirs inside teachers.photo so every
-- listing stays the single source of truth.
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo BYTEA;

-- Auth revocation: bumping invalidates previously minted bearers.
ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version BIGINT NOT NULL DEFAULT 0;

-- Soft delete with retention: deleted_at stamps when the login was
-- removed by an owner instead of wiped. The row (and its history)
-- survives until the retention window lapses, during which it can be
-- restored; a scheduled purge hard-deletes expired rows.
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;
-- Tracks whether the owner already replaced the initial password: the
-- console nudges every fresh login until the first change.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password_changed BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at) WHERE deleted_at IS NOT NULL;

-- Manual no-class days (locally moved holidays, emergencies).
ALTER TABLE school_years ADD COLUMN IF NOT EXISTS no_class_days DATE[] NOT NULL DEFAULT '{}';

-- Refresh rotation: only hashes persist, never usable bearers. Revoked
-- rows stay until expiry for reuse detection, then DeleteExpired purges.
CREATE TABLE IF NOT EXISTS refresh_tokens (
    jti         UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);

-- Auth throttle + audit: every login/refresh guess lands here (keyed
-- identifier|ip), so restarts and replicas share one budget. Denied
-- attempts are not recorded (they must not extend the window).
CREATE TABLE IF NOT EXISTS login_attempts (
    id      BIGSERIAL PRIMARY KEY,
    bucket  TEXT        NOT NULL,
    ip      TEXT        NOT NULL DEFAULT '',
    success BOOLEAN     NOT NULL DEFAULT FALSE,
    at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_login_attempts_bucket_at ON login_attempts (bucket, at);

-- Password recovery: single-use reset tokens. Only hashes persist;
-- used and expired rows are purged. Delivery (mail/WhatsApp) is pending.
CREATE TABLE IF NOT EXISTS password_resets (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_password_resets_token_hash ON password_resets (token_hash);
CREATE INDEX IF NOT EXISTS idx_password_resets_user_id ON password_resets (user_id);

-- Role grants survive restarts (the memory store they replace does not).
CREATE TABLE IF NOT EXISTS role_grants (
    subject_id UUID NOT NULL,
    role       TEXT NOT NULL,
    PRIMARY KEY (subject_id, role)
);

-- Audit trail: who did what to which record, when, with what outcome.
-- Append-only by convention (no updates or deletes anywhere in the
-- codebase); student_id is null for non-student actions such as schedule
-- and catalog writes.
CREATE TABLE IF NOT EXISTS audit_entries (
    id          UUID PRIMARY KEY,
    subject_id  UUID        NOT NULL,
    action      TEXT        NOT NULL,
    resource    TEXT        NOT NULL,
    resource_id UUID        NULL,
    student_id  UUID        NULL,
    status      INTEGER     NOT NULL,
    happened_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_audit_happened_at ON audit_entries (happened_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_student_id ON audit_entries (student_id) WHERE student_id IS NOT NULL;
-- Failed writes have no resource to point at: their id travels empty.
ALTER TABLE audit_entries ALTER COLUMN resource_id DROP NOT NULL;

-- Public URL slugs: short opaque aliases so links never expose UUIDs.
-- Partial unique indexes let legacy '' rows coexist until backfilled.
ALTER TABLE students ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_students_slug ON students (slug) WHERE slug <> '';
ALTER TABLE teachers ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_teachers_slug ON teachers (slug) WHERE slug <> '';
ALTER TABLE class_groups ADD COLUMN IF NOT EXISTS slug TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_class_groups_slug ON class_groups (slug) WHERE slug <> '';

-- Uniforme por estudiante y llamado: solo se registran cambios (el
-- checkbox arranca marcado). Historial completo para la trazabilidad.
CREATE TABLE IF NOT EXISTS session_uniform_history (
    id         UUID PRIMARY KEY,
    session_id UUID        NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    student_id UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    ok         BOOLEAN     NOT NULL,
    changed_by TEXT        NOT NULL,
    changed_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_session_uniform_history_session_id ON session_uniform_history (session_id);

-- Descargos: la defensa escrita ante una falta, una por estudiante y
-- llamado (upsert). No cambia la marca; viaja junto a ella.
CREATE TABLE IF NOT EXISTS session_defenses (
    session_id UUID NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    student_id UUID NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    text       TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (session_id, student_id)
);

CREATE INDEX IF NOT EXISTS idx_session_defenses_session_id ON session_defenses (session_id);

-- Permiso docente: ampliar los CHECK de estado existentes.
ALTER TABLE teacher_attendance DROP CONSTRAINT IF EXISTS teacher_attendance_status_check;
ALTER TABLE teacher_attendance ADD CONSTRAINT teacher_attendance_status_check
    CHECK (status IN ('present', 'absent', 'late', 'permission'));
ALTER TABLE teacher_attendance_revisions DROP CONSTRAINT IF EXISTS teacher_attendance_revisions_new_status_check;
ALTER TABLE teacher_attendance_revisions ADD CONSTRAINT teacher_attendance_revisions_new_status_check
    CHECK (new_status IN ('present', 'absent', 'late', 'permission'));

-- Reemplazo docente: la falta cubierta por un sustituto es un estado propio.
ALTER TABLE teacher_attendance DROP CONSTRAINT IF EXISTS teacher_attendance_status_check;
ALTER TABLE teacher_attendance ADD CONSTRAINT teacher_attendance_status_check
    CHECK (status IN ('present', 'absent', 'late', 'permission', 'replacement'));
ALTER TABLE teacher_attendance_revisions DROP CONSTRAINT IF EXISTS teacher_attendance_revisions_new_status_check;
ALTER TABLE teacher_attendance_revisions ADD CONSTRAINT teacher_attendance_revisions_new_status_check
    CHECK (new_status IN ('present', 'absent', 'late', 'permission', 'replacement'));

-- Llamados unificados: los formularios guardaron entre el 3 y el 16 de
-- noviembre de 2026 en observations, que solo leía la página del
-- docente. Se copian a warnings (mismo id, idempotente) para que la
-- lista del admin, la ficha, las estadísticas y los avisos los vean.
-- El título es el texto de la situación del manual; la nota pasa a la
-- descripción. Tipo I, II y III quedan como mild, moderate y severe.
INSERT INTO warnings (id, student_id, class_id, teacher_id, happened_at,
    gravity, title, description, snap_names, snap_surnames,
    snap_document_id, snap_class_id, snap_birthdate, snap_age,
    snap_caregiver_name, snap_caregiver_phone)
SELECT o.id, o.student_id, o.class_group_id, o.teacher_id,
    (o.happened_on + TIME '12:00') AT TIME ZONE 'America/Bogota',
    CASE o.kind WHEN 3 THEN 'severe' WHEN 2 THEN 'moderate' ELSE 'mild' END,
    COALESCE(c.text, o.situation_id), o.note, s.names, s.surnames,
    s.document_id, s.class_id, s.birthdate, -1,
    s.caregiver_name, s.caregiver_phone
FROM observations o
JOIN students s ON s.id = o.student_id
LEFT JOIN (VALUES
    ('s1-01', 'Ser descortés con cualquiera de los miembros de la comunidad educativa'),
    ('s1-02', 'Incumplir con tareas, trabajos, lecciones, talleres o actividades programadas por el profesor'),
    ('s1-03', 'Impuntualidad a clase o demás actividades programadas por la institución'),
    ('s1-04', 'Quedarse en el aula de clase en horas de descanso o en otras actividades programadas'),
    ('s1-05', 'No entregar en el tiempo estipulado al padre de familia o acudiente las citaciones que la institución envía'),
    ('s1-06', 'Presentarse a la institución sin los uniformes respectivos o incompletos'),
    ('s1-07', 'Promover el desorden y el desaseo en corredores, aulas, restaurante, cancha, patio, durante los cambios de hora o durante las diferentes actividades programadas por la institución'),
    ('s1-08', 'Interrumpir las clases con intervenciones como chistes, gestos o actitudes inoportunas que promuevan la indisciplina y el desorden de otros estudiantes'),
    ('s1-09', 'Realizar excesivas demostraciones de afecto con compañeros de igual o diferente sexo'),
    ('s1-10', 'Consumir alimentos en horas de clase'),
    ('s1-11', 'Esconder las pertenencias de los miembros de la comunidad educativa'),
    ('s1-12', 'Ingresar radios, reproductores de sonido, juegos electrónicos, gorras, pañoletas, piercing y demás objetos que no correspondan a las necesidades del quehacer escolar'),
    ('s1-13', 'Realizar apuestas y juegos de azar o comercializar cualquier clase de productos sin autorización de la institución'),
    ('s1-14', 'Obstaculizar premeditadamente el ingreso o la salida de las instalaciones de los estudiantes u otro miembro de la comunidad'),
    ('s2-01', 'Reincidir sistemáticamente en la comisión de una o varias situaciones del tipo I'),
    ('s2-02', 'Amenazar y/o agredir de palabra o de hecho, a cualquier integrante de la comunidad educativa'),
    ('s2-03', 'Injuria, difamación u ofensas graves contra los miembros de la comunidad educativa'),
    ('s2-04', 'Agresión grave física o moral contra los demás miembros de la comunidad educativa'),
    ('s2-05', 'Realizar actos de discriminación de raza, etnia, religión, sexo, creencias culturales, orientación sexual, condición económica, pensamiento político, diversidad funcional, entre otros'),
    ('s2-06', 'Promover o participar en desórdenes que deriven en atropello y atentados contra la integridad de las personas y los bienes privados o públicos'),
    ('s2-07', 'Daños graves causados por uso indebido o intencional en las instalaciones, materiales o documentos que pertenezcan a la comunidad educativa o en los bienes de otros miembros de la institución'),
    ('s2-08', 'Burlarse de los miembros de la comunidad educativa ya sea con palabras, gestos o acciones irrespetuosas'),
    ('s2-09', 'Cometer fraude en las evaluaciones y/o trabajos académicos'),
    ('s2-10', 'Falsificar o sustraer documentos públicos (certificados, constancias, boletines, documentos reglamentarios u oficiales)'),
    ('s2-11', 'Negarse a vestir correctamente los uniformes de la institución'),
    ('s2-12', 'La práctica o incitación de actos que pongan en riesgo o peligro la integridad física o vida de los miembros de la comunidad educativa'),
    ('s2-13', 'Conformar y/o pertenecer a pandillas dentro y fuera de la institución'),
    ('s2-14', 'Participar en actos de delincuencia o vandalismo'),
    ('s2-15', 'Evadirse de clase o de la institución'),
    ('s2-16', 'El incumplimiento de las sanciones impuestas'),
    ('s2-17', 'Las amenazas directas o indirectas contra cualquier miembro de la comunidad educativa'),
    ('s2-18', 'Faltar a clases y a actividades propias de la institución sin causa justificada'),
    ('s2-19', 'Comportamiento inmoral o práctica de escenas morbosas y vulgares'),
    ('s2-20', 'Evidenciar marcado desinterés en su rendimiento académico e incumplimiento reiterado de las actas de compromiso'),
    ('s2-21', 'No presentar actividades de apoyo o nivelación'),
    ('s2-22', 'Entorpecer el buen desarrollo de clase'),
    ('s2-23', 'Porte de cualquier tipo de armas dentro de la institución'),
    ('s2-24', 'Porte, distribución o consumo de drogas alucinógenas'),
    ('s2-25', 'Hurto dentro de las instalaciones de la institución'),
    ('s2-26', 'Destrucción premeditada de los implementos escolares, cuadernos, libros o trabajos de sus compañeros'),
    ('s2-27', 'El constreñimiento hacia miembros de la comunidad educativa'),
    ('s2-28', 'Ingresar y/o salir de la institución por lugares no autorizados y diferentes a la puerta principal de acceso'),
    ('s2-29', 'Presentarse a la institución en estado de embriaguez o bajo el efecto de sustancias psicotrópicas'),
    ('s2-30', 'Pegar chicles o elementos adhesivos en las paredes, mobiliario, elementos de la sede respectiva, al igual que adherirlos sobre el cuerpo de algún miembro de la comunidad educativa'),
    ('s2-31', 'Realizar grafitis obscenos, degradantes, calumniosos, burlescos o insultantes contra cualquier miembro de la comunidad educativa'),
    ('s2-32', 'Activar el timbre o cualquier otro dispositivo que indica el cambio de clases, comienzo o conclusión de la jornada escolar sin ser autorizado previamente'),
    ('s2-33', 'Desobedecer o asumir de manera agresiva, desafiante o altanera los llamados de atención que le haga un directivo o docente de la institución'),
    ('s2-34', 'Incumplir con la presentación de los materiales necesarios para el trabajo en clase'),
    ('s2-35', 'Omitir, tergiversar o falsear la información que la institución envía o recibe'),
    ('s2-36', 'Desprestigiar el buen nombre de la institución con expresiones y actuaciones indebidas dentro y fuera de la misma'),
    ('s2-37', 'Sobornar y/o chantajear'),
    ('s2-38', 'Introducir a la institución folletos, revistas y otros medios de divulgación pornográfica'),
    ('s2-39', 'Agredir de palabra o hecho a un directivo, docente, administrativo o estudiante de la institución, al igual que a cualquier persona ajena a esta que se encuentre en el interior de una sede'),
    ('s2-40', 'Jugar dinero o realizar rifas o ventas sin autorización'),
    ('s2-41', 'Las demás actitudes y comportamientos que atenten contra la sana convivencia dentro y fuera de la institución'),
    ('s2-42', 'Negarse a realizar actividades programadas en clase por parte del docente o de quien haga sus veces en ausencia de éste'),
    ('s2-43', 'Hacer uso inadecuado de las redes sociales: Facebook, Twitter, Instagram, WhatsApp, Tiktok, correos electrónicos o mensajes que atenten contra la dignidad de otras personas'),
    ('s3-01', 'Reincidir sistemáticamente en la comisión de una o varias situaciones tipo II'),
    ('s3-02', 'Causar lesiones personales a cualquier miembro de la comunidad educativa (amenazar y/o agredir de palabra o de hecho)'),
    ('s3-03', 'La participación en actos de matoneo escolar, bullying o ciberacoso'),
    ('s3-04', 'La ejecución de actos tendientes a generar anarquía general'),
    ('s3-05', 'Causar actos de vandalismo en la institución, en la calle o en su comunidad'),
    ('s3-06', 'Ejecutar de manera individual o colectiva actos o conductas tipificadas como delitos en el ordenamiento penal'),
    ('s3-07', 'Ser declarado responsable de hurto o abuso de confianza por autoridad competente en el colegio o fuera de él'),
    ('s3-08', 'Acosar, abusar o realizar actos sexuales, constreñir o inducir a actos sexuales abusivos')
) AS c(id, text) ON c.id = o.situation_id
ON CONFLICT (id) DO NOTHING;

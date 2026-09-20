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

CREATE INDEX IF NOT EXISTS idx_incident_faults_student_id ON incident_faults (student_id);

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
    medical_conditions  TEXT        NOT NULL DEFAULT '',
    homeroom_class_id   TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_teachers_homeroom_class_id ON teachers (homeroom_class_id);

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
    closed      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (periods >= 1 AND periods <= 6),
    CHECK (start_date < end_date)
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

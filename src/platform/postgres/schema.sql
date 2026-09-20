-- Schema for the grade backend. Idempotent: safe to run on every start.

CREATE TABLE IF NOT EXISTS students (
    id         UUID PRIMARY KEY,
    names      TEXT        NOT NULL,
    surnames   TEXT        NOT NULL,
    class_id   TEXT        NOT NULL,
    photo      BYTEA,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_students_class_id ON students (class_id);

CREATE TABLE IF NOT EXISTS attendance_records (
    id         UUID PRIMARY KEY,
    student_id UUID        NOT NULL REFERENCES students (id) ON DELETE CASCADE,
    class_id   TEXT        NOT NULL,
    date       DATE        NOT NULL,
    reason     TEXT        NOT NULL CHECK (reason IN ('absence', 'evasion'))
);

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

CREATE TABLE IF NOT EXISTS student_attendance (
    attendance_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    school_id text NOT NULL,
    student_id text NOT NULL,
    record_date date NOT NULL,
    status_code text NOT NULL,
    submitted_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO student_attendance (school_id, student_id, record_date, status_code)
SELECT
    CASE WHEN value <= 6 THEN 'SCH-001' ELSE 'SCH-002' END,
    'STU-' || lpad(value::text, 3, '0'),
    DATE '2026-07-01',
    CASE WHEN value = 12 THEN 'X' WHEN value IN (5, 9) THEN 'A' ELSE 'P' END
FROM generate_series(1, 12) AS value;

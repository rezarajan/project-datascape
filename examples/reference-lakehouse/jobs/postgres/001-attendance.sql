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

CREATE TABLE IF NOT EXISTS payments (
    payment_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    invoice_id text NOT NULL,
    amount_cents integer NOT NULL,
    paid_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS invoices (
    invoice_id text PRIMARY KEY,
    account_id text NOT NULL,
    amount_cents integer NOT NULL,
    issued_at date NOT NULL
);

INSERT INTO invoices (invoice_id, account_id, amount_cents, issued_at)
VALUES
    ('INV-001', 'acct-education-north', 125000, DATE '2026-07-01'),
    ('INV-002', 'acct-education-south', 98000, DATE '2026-07-02')
ON CONFLICT (invoice_id) DO NOTHING;

INSERT INTO payments (invoice_id, amount_cents)
SELECT invoice_id, amount_cents
FROM invoices
ON CONFLICT DO NOTHING;

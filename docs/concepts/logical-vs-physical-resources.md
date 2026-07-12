# Logical vs Physical Resources

Logical resources are stable compiler identities. Physical resources are target artifacts owned by adapters.

Diff, inspect, recovery, and rollout isolation use logical identities.

Example:

- logical stream: `EventStream/education/student-attendance-cdc`
- physical Redpanda topic: `cdc.local.education.sms.public.student_attendance.v1`

Changing adapters may alter physical projections without changing logical source declarations.

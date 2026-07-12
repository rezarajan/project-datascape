# Audit and Evidence

Audit and execution evidence are required only when selected by `AuditBinding`, declared through audit resources, or required by policy.

When selected, Datascape generates:

- a separate execution-evidence object-storage bucket declaration;
- `schemas/evidence.schema.json`;
- verification results with stable check IDs;
- recovery artifacts for audit integrity validation.

Evidence storage is distinct from raw data, broker state, derived data, and lineage admission journal state.

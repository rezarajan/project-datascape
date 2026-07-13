# CDC Operations

CDC steady state is declared with `CDCClass`, `CDCInstance`, and `CDCBinding`. One-shot work is declared with `CDCOperation` and rendered as operation plans.

Implemented for the first Compose release:

- deterministic CDC worker, connector, verification, and recovery artifacts;
- read-only CLI views with `platformctl status`, `platformctl cdc connectors`, `platformctl cdc describe`, and `platformctl operations plan`;
- generated operation bundles with `platformctl operations generate`;
- validation for ObserveOnly mutation rejection, external worker-management rejection, destructive offset prerequisites, shared CDC instance deletion, and idempotency-key conflicts.

Generated as provider operation plans:

- pause, resume, restart, delete, move, table filter change, snapshot, resnapshot, offset import/export/reset, credential rotation, provider upgrade, backup, restore, adoption, detachment, and deletion workflows.

Future control-plane work:

- continuous reconciliation, live drift repair, rolling worker upgrades, zero-downtime credential rotation, and provider-specific live execution beyond generated Compose operation bundles.

Plans identify affected sources, connectors, streams, CDC instances, provider instances, preconditions, approval requirements, verification, and recovery steps. Secret values are never copied into plans, diffs, logs, checksums, or generated documentation.

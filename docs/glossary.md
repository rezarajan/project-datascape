# Glossary

- Compiler: The `platformctl` binary that turns declared input into deterministic artifacts.
- IR: Normalized intermediate representation.
- CDC event: A committed database state-change event.
- Domain event: A business fact or decision.
- Lineage event: An execution and dataset relationship event.
- Audit event: Security, governance, operator, or evidence record.
- Control event: Replay, recovery, repair, or administrative intent.
- Bundle digest: Deterministic digest over bundle payload artifacts.
- Rollout-sensitive digest: Digest used only where a runtime workload should roll.

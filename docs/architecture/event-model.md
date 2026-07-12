# Event Model

Event classes remain separate:

- CDC events: committed database state changes.
- Domain events: business facts or decisions.
- Lineage events: execution and dataset relationships.
- Audit events: security, governance, operator, or execution evidence.
- Control events: replay, recovery, repair, or administrative intent.

Native domain events use CloudEvents in later runtime adapters.

Current release generates both CDC and domain-event streams:

- CDC events are produced by change-stream provider from PostgreSQL logical replication.
- Domain events are published through application runtime Pub/Sub as CloudEvents.
- Lineage events are admitted through lineage admission-compatible configuration.
- Audit/evidence records are stored separately from raw data.

Physical Redpanda topics are generated adapter projections, not portable contracts.

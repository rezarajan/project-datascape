# Reliability

The platform does not claim universal exactly-once execution. It pursues verifiable, idempotent outcomes using durable logs, raw archives, idempotency ledgers, checkpoints, atomic table commits, replay metadata, and evidence.

The compiler rejects data pipelines with external effects and no idempotency strategy.

Current release applies the same reliability model to CDC and native events:

- raw archive object keys are deterministic;
- change-stream provider offsets are configured as durable Kafka Connect topics;
- Redpanda topics are declared deterministically;
- application runtime events use CloudEvents and DLQ subscription metadata;
- lineage admission admission has journal and quarantine paths;
- verification and replay artifacts describe duplicate-safe recovery behavior.

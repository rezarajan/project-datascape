# Idempotency

Supported strategies include event-id ledger, natural-key upsert, compare-and-set, transactional outbox, idempotency header, deterministic object key, Iceberg atomic overwrite or merge, and checkpointed sink.

Current release validates that external-effect pipelines declare a strategy.

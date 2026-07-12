# ADR-0033 - Raw event archive object identity and idempotency

Status: Accepted with scheduled runtime validation

## Context
CDC and domain events must be durably archived and replayable without duplicate effects.

## Decision
Use deterministic object keys of the form `<event-class>/<logical-stream>/<partition>/<offset>-<event-id>.json`.

## Alternatives considered
Timestamp-based object names; random UUID object names; archive after downstream processing only.

## Consequences
Retries can overwrite or detect identical objects safely.

## Security implications
Raw data and execution evidence are stored separately.

## Operational implications
Archive consumers acknowledge broker records only after durable object creation in the intended runtime model.

## Reversibility
Object key patterns can be versioned by archive adapter.

## Validation
Current release generates raw archive config and recovery inventory scaffolding; live idempotency is covered by integration tests.

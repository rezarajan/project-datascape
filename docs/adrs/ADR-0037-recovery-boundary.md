# ADR-0037 - Current release recovery boundary

Status: Accepted

## Context
Current release does not implement Bronze, Silver, Gold, or Kubernetes recovery, but must establish executable recovery contracts.

## Decision
Generate schema-registry rehydration, topic recreation, connector recreation, raw archive inventory, raw replay, lineage admission replay, audit integrity, and recovery dependency graph artifacts.

## Alternatives considered
Defer recovery entirely; document manual recovery only.

## Consequences
Later planned releases extend the same recovery graph to lakehouse, metadata, lineage views, dashboards, and validation reports.

## Security implications
Recovery artifacts do not contain secret values.

## Operational implications
Local runtime state can be recreated from generated artifacts and surviving durable volumes.

## Reversibility
Recovery step implementations can mature without changing the declared boundary.

## Validation
Recovery artifacts are generated under `recovery/` and included in bundle checksums.

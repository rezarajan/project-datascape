# ADR-0030 - Logical versus physical event streams

Status: Accepted

## Context
Portable contracts must not depend on broker-specific topic names.

## Decision
Specifications use logical stream identities; adapters generate physical topic projections recorded in `resources.json` and `plan.json`.

## Alternatives considered
Expose change-stream provider topic names as source contracts; make applications publish to physical topics.

## Consequences
Broker substitution does not require changing application contracts or source specifications.

## Security implications
Access policies can be generated from logical identities and projected to physical topics.

## Operational implications
Operators can inspect logical-to-physical mappings for troubleshooting and recovery.

## Reversibility
Projection rules can evolve by adapter version.

## Validation
Tests assert CDC logical stream mapping to `cdc.local...` physical topics.

# ADR-0034 - lineage admission lineage admission for local runtime

Status: Accepted

## Context
Lineage failure must not be silently ignored.

## Decision
Generate lineage admission configuration and a local lineage admission-compatible admission/quarantine contract for Compose.

## Alternatives considered
Send OpenLineage directly to Marquez; omit local lineage proof; let producers ignore failures.

## Consequences
Valid lineage is admitted through a durable boundary and invalid lineage is quarantined with stable reasons.

## Security implications
Producer identity rules are generated.

## Operational implications
Replay artifacts exist even when downstream lineage delivery is unavailable.

## Reversibility
An upstream lineage admission image can replace the local-compatible service through the catalogue.

## Validation
Configuration, policies, verification checks, quarantine, and replay artifacts are generated.

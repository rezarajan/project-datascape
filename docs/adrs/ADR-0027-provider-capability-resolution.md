# ADR-0027 - Component catalogue and capability resolution

Status: Accepted

## Context
Runtime profiles must select implementations without hard-coding vendors into source specifications.

## Decision
Resolve selected implementations from `RuntimeProfile` against `Provider` entries and validate target compatibility, image digests, and required capabilities.

## Alternatives considered
Hard-code Redpanda and change-stream provider; let renderers silently choose defaults; defer validation to Compose.

## Consequences
`plan.json` and `provenance.json` expose resolved adapter versions and component image references.

## Security implications
Floating tags are rejected. Digest-pinned images are required unless an explicit development exception is declared.

## Operational implications
Profiles can swap implementations without changing logical source, stream, contract, archive, lineage, or recovery resources.

## Reversibility
Capability names can evolve through API-versioned compatibility rules.

## Validation
Tests reject missing adapters and incompatible targets.

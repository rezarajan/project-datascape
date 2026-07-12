# ADR-0031 - Git-authoritative schemas and runtime registry projection

Status: Accepted

## Context
Runtime Schema Registry IDs are not durable domain identifiers.

## Decision
`EventContract` resources generate canonical schema files, content digests, subject mappings, bootstrap artifacts, verification artifacts, and recovery manifests.

## Alternatives considered
Treat runtime registry as authoritative; hand-register schemas; store registry IDs in contracts.

## Consequences
Registry state can be rehydrated from Git-generated artifacts.

## Security implications
Schema content is hashable and reviewable before runtime registration.

## Operational implications
Registry replacement does not change logical schema identity.

## Reversibility
Additional schema formats can be added behind contract normalization.

## Validation
Schema-only changes update schema projection artifacts without changing Compose service definitions.

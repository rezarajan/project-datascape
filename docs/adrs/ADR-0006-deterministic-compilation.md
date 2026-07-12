# ADR-0006 - Deterministic compilation definition and limits

Status: Accepted

## Context
Identical compiler inputs must produce byte-identical deterministic artifacts and stable bundle digests.

## Decision
Artifacts exclude current timestamps, random IDs, unordered map iteration, live-cluster discovery, floating component versions, and nondeterministic archive ordering.

## Alternatives considered
Best-effort deterministic output; embedding generated-at metadata in bundles.

## Consequences
Nondeterministic CI metadata is outside the deterministic bundle unless supplied explicitly.

## Security implications
Stable hashes support integrity verification and tamper detection.

## Operational implications
`SOURCE_DATE_EPOCH` is captured only when explicitly supplied.

## Reversibility
Additional deterministic fields can be added compatibly; nondeterministic bundle fields would violate this ADR.

## Validation
Current release implements canonical JSON, normalized newlines, stable file ordering, stable modes, and bundle checksums.

# ADR-0007 - Stable logical resource identity

Status: Accepted

## Context
Diffing, provenance, inspection, recovery, and rollout isolation require stable resource identity.

## Decision
Resource identity is composed from API version, kind, logical namespace, logical name, target, and adapter.

## Alternatives considered
Use filenames, output order, runtime-generated IDs, or physical topic/resource names.

## Consequences
Generated resources can be traced and compared independent of file order.

## Security implications
Stable identities make authorization, audit, and evidence records correlatable without exposing secrets.

## Operational implications
Changing unrelated resources does not rename or invalidate stable logical identities.

## Reversibility
Identity changes require a migration policy because they affect digests and diffs.

## Validation
Current release implements `ResourceIdentity` and uses it in IR, plan, diff, inspect, and resource inventory.

# ADR-0066: Steady-State Resources and CDC Operations

## Status

Accepted.

## Context

Production CDC needs routine desired-state compilation and exceptional day-2 actions such as pausing connectors, resetting offsets, resnapshotting, moving connectors, rotating credentials, and adopting external resources. These actions should not be hidden in YAML templates or executed implicitly by generation.

## Decision

Declarative resources describe steady state. `CDCOperation` describes one-shot or exceptional work through deterministic operation plans. Provider contracts declare supported actions, parameters, mutability, destructiveness, idempotency, preconditions, verification, and recovery expectations.

`platformctl generate` and `docker compose up` do not execute operation plans. Compose bundles include operation artifacts, and executable mutation remains behind explicit operation entry points or provider-owned adapters.

Destructive operations require approval and recovery prerequisites. Offset reset, connector deletion, CDC instance deletion, and detach-and-delete actions are classified as destructive.

## Consequences

Operators can review plans and diffs in CI before mutation. `ObserveOnly` resources reject mutation operations. External `ManagedConnectors` instances allow connector-management plans but reject worker-management plans unless an explicit future provider contract permits them.

The first production Compose release generates operation plans and verification artifacts. A future continuously reconciling control plane may execute the same provider contracts, but planned-only operations are not reported as automatically applied.

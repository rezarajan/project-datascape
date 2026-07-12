# ADR-0001 - Product boundary: compiler, not custom runtime control plane

Status: Accepted

## Context
The product compiles declarative data-platform specifications into deployment, verification, recovery, and documentation artifacts.

## Decision
`platformctl` is a single-binary compiler. It does not become a continuously running data-platform control plane.

## Alternatives considered
Build a bespoke runtime control plane; use only static YAML templates.

## Consequences
Runtime behavior is delegated to established systems. Compiler correctness depends on deterministic IR, validation, rendering, and conformance artifacts.

## Security implications
The compiler must not handle live secret values. Generated artifacts use secret references.

## Operational implications
Operators review and apply generated artifacts through their target deployment workflow.

## Reversibility
This can be revisited only with a separate runtime-control-plane product decision.

## Validation
Current release implements compiler-only commands and emits no daemon or controller.

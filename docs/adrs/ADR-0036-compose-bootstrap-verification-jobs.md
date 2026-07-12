# ADR-0036 - Compose bootstrap and verification jobs

Status: Accepted

## Context
The local bundle must start without undocumented manual setup.

## Decision
Generate one-shot bootstrap and verification service definitions plus deterministic configuration files consumed by those services.

## Alternatives considered
Manual curl commands; shell-heavy inline Compose commands; documentation-only setup.

## Consequences
Initialization intent is versioned, reviewable, and included in bundle checksums.

## Security implications
Bootstrap artifacts use secret references and do not write secret values into provenance.

## Operational implications
`platformctl verify --bundle ... --runtime` reports stable check IDs from generated verification artifacts.

## Reversibility
Bootstrap implementation can be replaced by richer helper containers while preserving config contracts.

## Validation
Current release emitted `CHECK-001` through `CHECK-019` verification entries. Current release emits graph-derived `CHECK-*` checks and the CLI verifier accepts both ID families for compatibility.

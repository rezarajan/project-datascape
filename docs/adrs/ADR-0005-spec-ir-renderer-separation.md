# ADR-0005 - User specification, normalized IR, and renderer separation

Status: Accepted

## Context
Rendering target files directly from arbitrary user YAML would make validation, determinism, and substitution weak.

## Decision
Compilation flows from user resources to validated normalized IR to target artifacts.

## Alternatives considered
Direct text templating; target-specific source specs; runtime discovery during compilation.

## Consequences
Compiler passes are explicit and unit-testable.

## Security implications
Validation rejects inline secret-like values before artifact generation.

## Operational implications
`inspect`, `plan`, `diff`, and change isolation operate on logical resources, not arbitrary text order.

## Reversibility
The IR can evolve under specification-version compatibility rules.

## Validation
Current release implements parsing, validation, IR derivation, planning, and foundation rendering as separate packages.

# ADR-0004 - Standard-library-first dependency policy

Status: Accepted

## Context
Dependencies increase supply-chain, maintenance, and reproducibility risk.

## Decision
Use the Go standard library unless a dependency provides substantial value that is difficult to reproduce. Every accepted dependency requires an ADR entry.

## Alternatives considered
Adopt common CLI, YAML, templating, and assertion libraries immediately.

## Consequences
Current release used only the standard library. Current release accepted `gopkg.in/yaml.v3` through ADR-0026 because correct YAML support was disproportionate to implement safely in-house.

## Security implications
The initial supply chain is limited to the Go toolchain.

## Operational implications
Fewer dependencies simplify static builds, audits, and release management.

## Reversibility
Dependencies may be added through future ADRs with explicit justification.

## Validation
All external dependencies must have an ADR. ADR-0026 records the YAML parser dependency.

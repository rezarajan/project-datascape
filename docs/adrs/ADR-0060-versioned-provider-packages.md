# ADR-0060: Versioned Provider Packages and Conformance

Status: Accepted

## Context

Custom data-platform components must not require core compiler switch changes,
but unbounded templates or in-process Go plugins would weaken determinism,
portability and trust.

## Decision

Providers use the versioned `datascape.dev/provider-plan/v1alpha1` contract.
Descriptors record package and contract versions, digest/provenance, portable
capabilities, supported binding kinds, target compatibility, constrained service
plans, artifacts and conformance identifiers. Embedded and user-declared
providers pass through the same registry and normalized IR.

Initial distribution is embedded or local declarative packages. OCI transport
may be added later without changing the descriptor. Go's in-process plugin ABI
is not used.

## Alternatives considered

Hard-code each product; execute arbitrary templates; dynamically load Go shared
objects.

## Consequences

Providers are inspectable and testable. The constrained Compose service contract
must evolve when a genuinely new target feature is needed.

## Security and operations

Production profiles require immutable images and may reject elevated service
requirements. Package checksums and provenance are present in plans.

## Reversibility and validation

The provider contract is versioned independently. Custom-resource and reference
provider fixtures prove registration and target rendering without core kind edits.

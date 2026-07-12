# ADR-0003 - Ports and Adapters architecture

Status: Accepted

## Context
Core compiler logic must not couple directly to Kubernetes, Docker, broker clients, cloud SDKs, or template engines.

## Decision
Use Ports and Adapters. Core packages produce technology-neutral plans; target and component support lives behind adapter packages.

## Alternatives considered
Direct renderer calls from parsing; vendor-specific domain models; global template context.

## Consequences
Adapters can be added without changing the core IR or validation model.

## Security implications
Vendor credentials and secret values remain outside core compiler data structures.

## Operational implications
Component substitution is validated through capability declarations and conformance suites.

## Reversibility
Package boundaries can be refactored, but core/vendor separation is a product invariant.

## Validation
Current release creates the required package boundaries and keeps implementation standard-library-only.

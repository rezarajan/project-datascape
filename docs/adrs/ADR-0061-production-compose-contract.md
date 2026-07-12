# ADR-0061: Production Docker Compose Contract

Status: Accepted

## Context

Compose is valuable for a hardened single host but cannot provide distributed
high availability. Development bundles previously used tags, broad port
exposure and minimal service policy.

## Decision

`single-host-production` is the only production availability class for Compose.
Its compiler policy requires digest-pinned images and localhost-bound published
ports. The provider service plan supports restart policy, graceful shutdown,
non-root user, read-only filesystem, init, capability drops, security options,
tmpfs, secrets/configs, profiles and resource limits. Dependencies use
`service_healthy` when the prerequisite has a health check.

## Alternatives considered

Describe Compose as highly available; leave hardening to manual edits; make
Kubernetes the only useful target.

## Consequences

Generated production bundles are strict and may reject third-party images until
their provider declares a safe deployment. Local profiles can explicitly permit
unpinned images for development.

## Security and operations

Secrets are references, public exposure is opt-in, and recovery/storage plans
ship beside the bundle. Compose does not replace host hardening, backups or TLS.

## Reversibility and validation

Policies can be expanded by profile without changing the IR. Unit tests and
Compose configuration validation enforce generation behavior.

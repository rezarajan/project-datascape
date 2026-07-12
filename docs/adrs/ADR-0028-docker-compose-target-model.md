# ADR-0028 - Docker Compose target model

Status: Accepted

## Context
Current release needs a runnable local target without claiming production HA.

## Decision
Generate Docker Compose as the local/integration target with stable service names, named volumes, health checks, dependency conditions, and adapter-owned configuration mounts.

## Alternatives considered
Start with Kubernetes; generate only static configuration; use shell scripts as the primary runtime.

## Consequences
Compose output proves the compiler pipeline end to end while production HA remains a Kubernetes release.

## Security implications
Generated bundles use environment and secret references rather than embedded secret values.

## Operational implications
Users start the local stack with Docker Compose and supplied environment values.

## Reversibility
Compose renderer can be replaced without changing core IR.

## Validation
Generated `compose.yaml` passes `docker compose config` with supplied environment values.

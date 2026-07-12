# ADR-0032 - application runtime native-event integration

Status: Accepted

## Context
Applications that intentionally publish domain events should not use broker-specific clients.

## Decision
Generate application runtime Pub/Sub components, subscriptions, resiliency policy, scopes, and native-event example configuration.

## Alternatives considered
Use Kafka client libraries in example applications; expose Redpanda topic names directly.

## Consequences
Native events use CloudEvents and stable logical contracts while broker projection remains adapter-owned.

## Security implications
application runtime access scopes and secret-store references are generated for runtime enforcement.

## Operational implications
Applications publish through application runtime HTTP/gRPC rather than Kafka APIs.

## Reversibility
Application runtime adapters can change without changing event contracts.

## Validation
Generated application runtime artifacts are included in the Compose bundle and change only for dependent event-source changes.

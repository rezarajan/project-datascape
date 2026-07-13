# ADR-0065: First-Class CDC Runtime Resources

## Status

Accepted.

## Context

CDC bindings previously selected the change-stream capability directly. That made multiple connector requests collapse into one provider plan, hid worker sharing decisions, and forced the reference Debezium registration path to know about one database and one connector.

## Decision

Datascape separates CDC into:

- `CDCClass`: cluster-scoped runtime/provider compatibility, supported connector classes, class defaults, and worker configuration.
- `CDCInstance`: namespaced managed, imported, or external runtime with provider selection, resources, endpoint, credentials reference, management policy, and verification requirements.
- `CDCBinding`: one source-to-stream connector request that references a source, stream, CDC instance, and connector class.

The compiler emits target-neutral CDC IR containing CDC classes, instances, and connector plans. Connector plans own deterministic connector names, source resolution, secret environment references, provider-native configuration, lifecycle state, dependencies, and verification.

## Consequences

Worker sharing and isolation are explicit. Multiple connectors may share one CDC instance or be split across instances with independent resources and provider instances. External CDC instances may be managed through `ManagedConnectors` or observed through `ObserveOnly`.

Provider-native configuration is generated from constrained class mappings and defaults plus binding overrides. The compiler does not execute templates or plugins, and generated files keep secrets as references.

Legacy CDC bindings without `cdcRef` remain accepted only for deterministic non-production defaults. Production profiles require explicit CDC runtime references.

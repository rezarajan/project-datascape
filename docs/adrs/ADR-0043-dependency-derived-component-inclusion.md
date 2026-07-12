# ADR-0043: Dependency-Derived Component Inclusion

Status: Accepted

Compose and other targets include components required by the resolved graph, not by a fixed catalogue list. Runtime profiles provide implementation choices and may explicitly force capabilities, but there is no broad default component set.

This prevents source-only, external-only, CDC-only, and events-only bundles from carrying unrelated services.

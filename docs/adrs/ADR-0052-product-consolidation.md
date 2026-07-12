# ADR-0052: Product Consolidation

Status: Accepted

`platformctl` is a declarative data-platform compiler with one current `platform.datascape.dev/v1alpha1` core API. Removed pre-consolidation kinds are not compatibility APIs for current compilation.

The compiler parses resources, registers definitions and providers, resolves bindings, evaluates policy, builds a dependency graph, derives provider-owned target resources, and renders deterministic artifacts.

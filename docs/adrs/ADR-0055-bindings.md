# ADR-0055: Bindings

Status: Accepted

Typed binding resources are the current public edge model. A typed binding declares readable source and target references, optional provider instance, lifecycle state, and ownership. The compiler normalizes typed bindings and generic extension `Binding` resources into capability edges. `BindingDefinition` declares compatibility and dependency closure for a capability.

The resolver validates references, provider compatibility, cycles, deterministic ordering, and per-binding digests.

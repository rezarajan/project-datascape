# ADR-0038: Resource And Binding Model

Status: Accepted

Datascape models platform intent as typed resources plus typed binding resources. Resources describe sources, streams, contracts, stores, pipelines, policies, connections, and external capabilities. Binding resources describe relationships such as source-to-stream, producer-to-stream, stream-to-archive, source-to-lineage, and audit boundaries.

The compiler validates bindings independently from component selection. This allows producer-only streams, consumers of existing streams, and externally managed infrastructure without forcing one reference architecture.

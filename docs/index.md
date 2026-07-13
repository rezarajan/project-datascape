# Datascape Documentation

Datascape is a deterministic data-platform compiler. Current release models platforms as typed resources, connections, providers, bindings, and targets, validates the graph, and derives only the capabilities required by that graph.

- [Complete reference-lakehouse quickstart](usage/quickstart-local.md)
- [Architecture overview](architecture/overview.md)
- [Kubernetes-familiar data-platform model](concepts/kubernetes-familiar-data-platform.md)
- [Specification reference](specification/reference.md)
- [Storage classes, volumes and claims](specification/storage.md)
- [Database and connector classes](specification/database-connectivity.md)
- [Provider package authoring](development/provider-packages.md)
- [Production Compose](usage/production-compose.md)
- [CDC operations](operations/cdc-operations.md)
- [Reference compatibility matrix](operations/compatibility-matrix.md)
- [Recovery](usage/recovery.md)
- [CLI reference](cli/reference.md)
- [Migration](migration/resource-model-v1alpha1.md)
- [Glossary](glossary.md)

Every ADR under `docs/adrs/` is included automatically in the generated site
navigation. The generated API reference is built from the compiler's registered
resource definitions and is available at `reference/api.html` in the output.

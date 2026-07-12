# ADR-0054: Providers And Capabilities

Status: Accepted

Provider descriptors advertise open namespaced capabilities, target compatibility, runtime dependencies, renderer contracts, services, artifacts, and conformance metadata. `ProviderInstance` selects a configured provider for a target.

Built-in providers use the same registry path as user providers.

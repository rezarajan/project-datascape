# ADR-0056: Compatibility And Migration

Status: Accepted

The current API is a hard break from pre-consolidation manifests. `platformctl migrate` provides best-effort conversion to current resources and bindings, emits diagnostics, and does not preserve removed kinds in current compilation.

External provider executable protocols are reserved for future design. Current extension paths are declarative resources and compile-time provider registration.

# ADR-0050: Deterministic Graph Output

Status: Accepted

Graph nodes, bindings, external resources, policies, overrides, diagnostics, generated files, and digests must be deterministic across input ordering and `GOMAXPROCS` variation.

Resource hashes include ownership and graph state in `resources.json` while preserving per-resource rollout isolation.

# Compiler Pipeline

The compiler defines explicit passes:

1. parse
2. schema validation
3. semantic validation
4. reference resolution
5. capability resolution
6. policy enforcement
7. normalization
8. dependency graph construction
9. execution-plan derivation
10. target planning
11. artifact rendering
12. canonicalization
13. per-resource hashing
14. bundle hashing
15. provenance generation
16. documentation generation
17. conformance-test generation
18. recovery-artifact generation

Later planned releases deepen target and adapter behavior without bypassing these passes.

Current release implements real work in the early and middle passes:

- YAML and JSON parsing normalize into resources.
- Schema and semantic validation reject unsupported API versions, duplicate keys, unknown fields, invalid HA claims, inline secret-like values, missing refs, and missing idempotency declarations.
- Capability resolution validates `RuntimeProfile` selections against `Provider`.
- Normalization derives typed IR plans for streams, contracts, CDC, native events, archives, lineage, audit, verification, and recovery.
- Compose rendering is adapter-owned and deterministic.

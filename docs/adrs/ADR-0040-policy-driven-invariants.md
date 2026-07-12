# ADR-0040: Policy-Driven Invariants

Status: Accepted

Operational invariants such as lineage, audit, idempotency, verification, recovery, deferrals, and external trust are controlled by `PlatformPolicy`.

Security invariants that prevent secret leakage remain non-negotiable and cannot be bypassed by policy overrides.

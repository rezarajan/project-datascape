# ADR-0049: Pipeline Binding Rules

Status: Accepted

Pipelines declare inputs and outputs unless marked planned or disabled. Pipelines with external effects require an idempotency strategy.

Lineage, audit, and recovery for pipelines are policy-driven or selected by explicit bindings.

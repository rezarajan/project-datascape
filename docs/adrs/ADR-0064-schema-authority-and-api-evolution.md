# ADR-0064: Schema Authority and API Evolution

Status: Accepted

## Context

Validation, CLI explanation and documentation must not define separate versions
of the API. Extension resources also need strict unknown-field behavior.

## Decision

Resource definitions are the authoritative registry for scope, category,
provider type, capabilities, binding roles and spec schema. Their schemas are
compiled and evaluated as JSON Schema 2020-12. Built-in class and storage
resources reject unknown fields and report field-addressed diagnostics.
Generated API documentation reads this registry. Public resources remain
v1alpha1 while the new model is exercised; incompatible changes require a new
version and deterministic migration.

## Alternatives considered

Independent Go validation structs and documentation tables; permissive maps for
all built-ins; immediate promotion to v1beta1.

## Consequences

Schema changes are visible in plans, docs and tests. Precise built-in checks
remain for high-value remediations, while JSON Schema covers nested and extension
constraints without creating a parallel schema system.

## Security, reversibility and validation

Strict schemas reduce accidental secret and target-specific field leakage.
Migration fixtures and deterministic schema generation protect API evolution.

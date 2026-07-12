# ADR-0029 - PostgreSQL CDC through change-stream provider

Status: Accepted

## Context
The default zero-burden integration path is normal PostgreSQL writes captured through CDC.

## Decision
Use change-stream provider PostgreSQL connector configuration generated from `RelationalSource` resources.

## Alternatives considered
Application polling; trigger-based capture; direct application event publication only.

## Consequences
The compiler generates source prerequisites, connector config, stream mappings, bootstrap artifacts, and verification SQL.

## Security implications
Database credentials are references, not inline generated secrets.

## Operational implications
Local sample sources are initialized automatically; external sources receive checks rather than unexpected mutation.

## Reversibility
CDC adapters remain behind adapter boundaries.

## Validation
Connector configuration and prerequisite artifacts are generated deterministically.

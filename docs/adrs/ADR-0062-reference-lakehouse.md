# ADR-0062: Complete Reference Lakehouse

Status: Accepted

## Context

A narrow CDC example does not teach users how sources, storage, medallion tables,
compute, governance and analytics form a data platform.

## Decision

The primary quickstart is `examples/reference-lakehouse`. It uses synthetic
attendance data and declares PostgreSQL 18, mounted SQLite, Debezium Kafka
Connect, Redpanda,
S3-compatible storage, an Iceberg/Nessie catalog, Spark transformations, Trino,
OpenLineage/Marquez, data-quality quarantine and an OpenMetadata governance
profile. The default `reference-up` workflow enables governance; operators can
omit that Compose profile for a smaller custom launch.

The expected flow is PostgreSQL CDC to Redpanda and bronze storage, SQLite batch
ingestion to bronze, bronze-to-silver validation, silver-to-gold aggregation,
SQL query, metadata discovery and lineage emission. `just reference-up` is the
single entry point; focused examples remain concept examples.

## Alternatives considered

Keep the incomplete CDC quickstart; use mocks for governance services; require
manual Compose editing.

## Consequences

The complete example requires substantial memory and Docker Compose v2. Smaller
examples remain available for learning individual resources.

## Security, reversibility and validation

Only synthetic data is used. Development secrets are generated locally with
0600 permissions. Compilation, storage resolution and service presence are
covered in tests; runtime verification is an integration target.

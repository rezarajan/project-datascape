# Quickstart: Reference Lakehouse

The primary quickstart creates a complete synthetic education lakehouse rather
than an isolated component demonstration.

## Prerequisites

- Go 1.26 or later
- Docker Engine with Docker Compose v2
- `just`
- approximately 16 GB of free memory for the optional governance profile

From a clean checkout, run:

```sh
just reference-up
```

The command builds `platformctl`, validates and generates the bundle, copies the
versioned reference jobs, creates local development secrets with mode `0600`,
starts Compose, runs the Spark medallion job and verifies the result.

The platform contains:

- PostgreSQL 18 reached through a TCP/JDBC connection and logical replication;
- SQLite reached through a mounted file and ODBC connector class;
- one managed Debezium Kafka Connect `CDCInstance` shared by two CDC bindings;
- Redpanda for the logical change streams;
- S3-compatible object storage and an Iceberg/Nessie catalog;
- bronze, silver, quarantine and gold Iceberg datasets;
- Spark processing, Trino queries and OpenLineage emission to Marquez;
- an optional OpenMetadata governance profile started with
  `just reference-governance-up`.

Useful commands:

```sh
just reference-verify
./bin/platformctl cdc connectors --platform examples/reference-lakehouse/platform.yaml --profile profiles/reference.yaml
./bin/platformctl operations plan --platform examples/reference-lakehouse/platform.yaml --profile profiles/reference.yaml
just reference-governance-up
just reference-logs
just reference-down
just reference-reset
```

`reference-down` retains volumes. `reference-reset` explicitly removes only the
generated reference bundle and its Compose volumes.

Focused examples remain under `examples/`, but they are concept examples rather
than the primary quickstart.

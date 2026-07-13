# platformctl

`platformctl` is a deterministic data-platform compiler. It reads portable YAML or JSON resources and generates target artifacts for deployment, verification, recovery, provenance, and documentation.

The public API is Kubernetes-familiar: bounded resource kinds with `apiVersion`,
`kind`, `metadata`, and `spec`. It is specialized for integrated data platforms,
not general-purpose container orchestration. Most users author:

- components such as `RelationalSource`, `EventStream`, `ObjectStore`, `LineageSink`, `AuditStore`, `Pipeline`, `Table`, and `Warehouse`;
- classes, instances and connections such as `DatabaseClass`, `ConnectorClass`,
  `CDCClass`, `DatabaseInstance`, `CDCInstance`, `DatabaseConnection` and
  `SecretReference`;
- `StorageClass`, `PersistentVolume`, `PersistentVolumeClaim` and
  `VolumeMountBinding` for durable state;
- typed bindings for CDC, batch/stream ingestion, transformation, lineage,
  access, audit and storage attachment.

`ResourceDefinition`, `Provider`, `ProviderInstance`, `BindingDefinition`, and generic `Binding` remain supported for provider extensions and custom capabilities.

## Core Concepts

Resources describe durable platform intent. Classes describe reusable policy,
connections describe access, providers advertise portable capabilities, and
bindings declare relationships and data movement. CDC workers are modeled as
first-class `CDCInstance` resources so connector sharing, isolation, external
ownership and day-2 operation plans remain reviewable. Targets choose how the
normalized graph is rendered.

Secrets are always references. `SecretReference.spec.backend` is one of `env`, `file`, `kubernetes`, or `vault`, and `spec.keys` lists logical keys such as `username`, `password`, `token`, `accessKey`, and `secretKey`.

## Quickstart

```sh
just reference-up
```

This builds and launches the synthetic reference lakehouse: PostgreSQL 18 and
SQLite sources, CDC and event streaming, S3-compatible object storage, Iceberg
medallion tables, Spark, Trino, data quality, metadata and OpenLineage. See the
[complete quickstart](docs/usage/quickstart-local.md).

Generate the navigable static HTML documentation with:

```sh
just docs
just docs-serve
```

The underlying commands are `platformctl docs build` and
`platformctl docs serve`.

## Build

```sh
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" ./cmd/platformctl
```

## Test

```sh
GOCACHE=/tmp/project-datascape-go-cache go test ./...
```

Docker runtime integration tests are available through `just test-integration`.

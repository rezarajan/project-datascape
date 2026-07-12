# Specification Reference

Resources use Kubernetes-style `apiVersion`, `kind`, `metadata`, `spec`, and optional `status`. YAML, JSON, and multi-document YAML are supported. Duplicate keys and unknown top-level fields are rejected.

## Primary Kinds

Components:

- `sources.datascape.dev/v1alpha1`: `RelationalSource`, `EventProducer`
- `streams.datascape.dev/v1alpha1`: `EventStream`
- `contracts.datascape.dev/v1alpha1`: `EventContract`
- `stores.datascape.dev/v1alpha1`: `ObjectStore`, `Warehouse`
- `lineage.datascape.dev/v1alpha1`: `LineageSink`
- `audit.datascape.dev/v1alpha1`: `AuditStore`
- `pipelines.datascape.dev/v1alpha1`: `Pipeline`
- `tables.datascape.dev/v1alpha1`: `Table`
- `databases.datascape.dev/v1alpha1`: `DatabaseInstance`
- `catalogs.datascape.dev/v1alpha1`: `TableCatalog`, `MetadataCatalog`
- `compute.datascape.dev/v1alpha1`: `QueryEngine`
- `quality.datascape.dev/v1alpha1`: `DataQualityRule`

Classes and storage:

- `databases.datascape.dev/v1alpha1`: `DatabaseClass`
- `connections.datascape.dev/v1alpha1`: `ConnectorClass`
- `storage.datascape.dev/v1alpha1`: `StorageClass`, `PersistentVolume`, `PersistentVolumeClaim`

Connections:

- `platform.datascape.dev/v1alpha1`: `SecretReference`
- `connections.datascape.dev/v1alpha1`: `DatabaseConnection`, `ObjectStoreConnection`, `EventStreamConnection`

Typed bindings:

- `bindings.datascape.dev/v1alpha1`: `CDCBinding`, `StreamPublishBinding`, `StreamArchiveBinding`, `LineageBinding`, `AuditBinding`, `PipelineBinding`, `AccessBinding`
- `bindings.datascape.dev/v1alpha1`: `BatchIngestBinding`, `StreamIngestBinding`, `TransformBinding`, `VolumeMountBinding`

Targets and policy:

- `platform.datascape.dev/v1alpha1`: `Target`, `RuntimeProfile`, `PlatformPolicy`

Extension authoring:

- `ResourceDefinition`, `Provider`, `ProviderInstance`, `BindingDefinition`, and generic `Binding`

Generic `Binding` is the extension escape hatch for custom capabilities. Built-in workflows should use typed binding kinds.

## Connections

`SecretReference.spec.backend` must be `env`, `file`, `kubernetes`, or `vault`. `SecretReference.spec.keys` contains logical keys such as `username`, `password`, `token`, `accessKey`, and `secretKey`.

New database connections bind `instanceRef` to `connectorClassRef`. TCP connector
classes require `endpoint.host` and `endpoint.port`; file connector classes
require `claimRef` and `file.path`. Legacy network connections using `engine`,
`host`, `port`, `database`, and `credentialsRef` remain accepted during v1alpha1.

`RelationalSource.spec.connectionRef` points to a `DatabaseConnection`. Credentials belong behind the connection and secret reference, not inline in source specs.

## Typed Binding Specs

`CDCBinding.spec`:

- `sourceRef`: `RelationalSource`
- `streamRef`: `EventStream`
- optional `providerInstanceRef`, `mode`, `snapshot`, `tables`

`StreamPublishBinding.spec`:

- `sourceRef`: `EventProducer`
- `streamRef`: `EventStream`
- optional `providerInstanceRef`, `mode`

`StreamArchiveBinding.spec`:

- `streamRef`: `EventStream`
- `objectStoreRef`: `ObjectStore`
- optional `providerInstanceRef`, `format`, `retention`

`LineageBinding.spec`:

- `sourceRef`: source resource
- `sinkRef`: `LineageSink`

`AuditBinding.spec`:

- `sourceRef`: source resource
- `auditStoreRef`: `AuditStore`

References use `Kind/name`, `Kind/namespace/name`, or `group/version/Kind/namespace/name`.

`VolumeMountBinding` connects a `PersistentVolumeClaim` to a data workload using
`claimRef`, `workloadRef`, `mountPath`, and optional `readOnly`.

## Graph Rules

Provider instances are selected by capability, target compatibility, and typed binding kind. Resources can be managed, imported, external, planned, deferred, or disabled. Object storage, audit, lineage, and pipeline bindings are optional unless required by `PlatformPolicy`. External resources must declare verification checks unless policy explicitly allows an external-trust override.

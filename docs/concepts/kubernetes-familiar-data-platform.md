# Kubernetes-Familiar Data Platform Model

Datascape borrows Kubernetes' declarative resource vocabulary without becoming
a general-purpose scheduler or control plane. `platformctl` compiles desired
data-platform intent into deterministic target artifacts; it does not reconcile
running resources continuously.

| Kubernetes idea | Datascape equivalent | Purpose |
|---|---|---|
| API object | `apiVersion`, `kind`, `metadata`, `spec` | Versioned portable intent |
| CRD | `ResourceDefinition` | Add a data-platform resource kind |
| Operator/driver | `Provider` and `ProviderInstance` | Satisfy portable capabilities on a target |
| StorageClass | `StorageClass` | Reusable storage/provisioning policy |
| PV/PVC | `PersistentVolume` / `PersistentVolumeClaim` | Supply and request durable storage |
| Volume mount | `VolumeMountBinding` | Attach a claim to a data workload |
| Admission | Schema and semantic validation | Reject invalid graphs before rendering |

Datascape adds concepts specific to integrated data platforms: database and
connector classes, relational sources, event streams, object stores, Iceberg
catalogs, medallion tables, pipelines, lineage, quality rules and typed data-flow
bindings.

Compose remains a single-host projection. A class such as Longhorn can be
represented for a future Kubernetes target, but it is rejected when selected by
a Compose profile because Docker Compose cannot invoke a CSI provisioner.

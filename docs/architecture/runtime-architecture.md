# Runtime Architecture

The local runtime is derived from the graph. A governed reference architecture can include CDC, native events, raw archive, and lineage admission:

```mermaid
flowchart LR
  PG[(PostgreSQL)] --> DBZ[change-stream provider Connect]
  DBZ --> RP[(Redpanda)]
  APP[Native Event Producer] --> DAPR[application runtime Pub/Sub]
  DAPR --> RP
  RP --> RAW[Raw Archive Config]
  RAW --> S3[(S3-Compatible Storage)]
  DBZ --> EF[lineage admission Admission]
  RAW --> EF
  EF --> J[(Lineage Journal)]
  EF --> Q[(Quarantine)]
```

Each edge in that diagram is optional unless declared by a binding or required by policy. `RelationalSource` alone does not render change-stream provider or a broker, external streams do not render managed brokers, and lineage admission appears only when lineage selects it.

Compose supports local development and hardened single-host production. It does not claim distributed production HA. Kubernetes, Flink, Iceberg, Dagster, Trino, and OpenMetadata are scheduled for later planned releases.

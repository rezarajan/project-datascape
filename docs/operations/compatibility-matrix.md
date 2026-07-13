# Reference Compatibility Matrix

The reference profile pins readable release tags for local evaluation. A
single-host production profile additionally requires immutable image digests.

| Component | Reference version | Compatibility role |
|---|---:|---|
| Go | 1.26 | Compiler and CLI |
| PostgreSQL | 18 | Operational relational source and logical replication |
| Redpanda | 26.1.12 | Kafka-compatible event stream |
| Debezium Kafka Connect | 3.6.0.Final | PostgreSQL CDC source and Kafka-compatible output |
| Project Nessie | 0.108.1 | Iceberg-compatible catalog API |
| Spark/Iceberg reference image | Spark 3.5.5 / Iceberg 1.8.1 | Medallion writes |
| Trino | 482 | Iceberg query engine |
| Marquez | 0.51.1 | OpenLineage API and lineage graph |
| OpenMetadata | 1.13.1 | Optional governance profile |

These versions are an integrated reference set, not a universal compatibility
claim. Before promoting a generated bundle, resolve every image to its tested
multi-architecture digest and record the results in provider package metadata.

CDC compatibility is validated across database engine, `ConnectorClass`, `CDCClass`, `CDCInstance`, provider instance and deployment target. The Compose reference supports PostgreSQL logical replication through Debezium Kafka Connect and treats SQLite as a file/batch source.

Version selection is checked against upstream release and compatibility pages:

- [Go releases](https://go.dev/dl/)
- [Redpanda releases and compatibility](https://docs.redpanda.com/streaming/current/reference/releases/)
- [Debezium releases](https://debezium.io/releases/)
- [Project Nessie releases](https://projectnessie.org/releases/)
- [Trino container documentation](https://trino.io/docs/current/installation/containers.html)
- [OpenMetadata requirements](https://docs.open-metadata.org/v1.13.x/deployment/minimum-requirements)

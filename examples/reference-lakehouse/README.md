# Reference Lakehouse

This example compiles the operational-source, integration, lakehouse,
governance and analytics layers into one single-host Docker Compose bundle.

The optional governance profile bootstraps OpenMetadata catalog entities from
`jobs/openmetadata/bootstrap.json`, then runs the OpenMetadata OpenLineage
ingestion workflow against the `openlineage-events` Kafka topic produced by the
Spark medallion jobs. After `just reference-up` and `just reference-governance-up`,
the default OpenMetadata login can see the reference PostgreSQL, SQLite, Iceberg
lakehouse tables, the three OpenLineage pipeline jobs, and the bronze-to-silver
to-gold lineage without UI setup.
It uses synthetic attendance data and demonstrates different access semantics
for a networked PostgreSQL database and a mounted SQLite database file.

Run `just reference-up` from the repository root. See the generated HTML
quickstart for prerequisites, endpoints, verification and reset behavior.

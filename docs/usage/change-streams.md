# PostgreSQL CDC

Datascape generates PostgreSQL CDC when a `CDCBinding` connects a `RelationalSource` to an `EventStream` through a first-class `CDCInstance`.

Selected CDC artifacts include:

- PostgreSQL local initialization SQL;
- logical replication prerequisites;
- one CDC worker per managed `CDCInstance`;
- one connector plan and provider-native connector configuration per `CDCBinding`;
- schema history topic configuration;
- logical-to-physical stream mapping;
- object archive routing when `StreamArchiveBinding` is declared;
- verification SQL;
- connector recreation recovery artifact;
- operation plans for pause, resume, restart, resnapshot, offset safety, migration and deletion workflows.

`RelationalSource` without a CDC binding is valid and does not render a broker or connector. SQLite remains a file/batch source unless a compatible CDC connector class is explicitly declared.

Managed CDC instances render Kafka Connect workers in Compose. External CDC instances render no worker. With `managementPolicy: ManagedConnectors`, Datascape generates idempotent connector registration against the external endpoint. With `managementPolicy: ObserveOnly`, it emits connector configuration and verification artifacts only.

Generated connector JSON uses environment-variable references such as `${EDUCATION_POSTGRES_CREDENTIALS_PASSWORD}`. Secret values are not placed in plans, generated configuration, Compose YAML, checksums or logs.

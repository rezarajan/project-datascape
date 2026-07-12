# PostgreSQL CDC

Datascape generates PostgreSQL CDC through the change-stream provider when a `CDCBinding` connects a `RelationalSource` to an `EventStream`.

Selected CDC artifacts include:

- PostgreSQL local initialization SQL;
- logical replication prerequisites;
- change-stream provider connector configuration;
- schema history topic configuration;
- logical-to-physical stream mapping;
- object archive routing when `StreamArchiveBinding` is declared;
- verification SQL;
- connector recreation recovery artifact.

`RelationalSource` without a CDC binding is valid and does not render a broker or connector. External sources are checked rather than silently modified.

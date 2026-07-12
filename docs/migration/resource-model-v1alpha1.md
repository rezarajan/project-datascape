# Resource Model Migration

`platformctl migrate` converts older manifests into the typed resource and binding model on a best-effort basis. Current compilation does not accept removed manifest shapes directly.

Common mappings:

- Platform roots become `Target` and `RuntimeProfile` declarations.
- Database-like sources become `RelationalSource` plus an external `DatabaseConnection` when the older manifest did not separate connection details.
- Application event producers become `EventProducer`.
- Streams become `EventStream`.
- Durable object storage becomes `ObjectStore`.
- Lineage sinks become `LineageSink`.
- Built-in binding capabilities become typed bindings: `CDCBinding`, `StreamPublishBinding`, `StreamArchiveBinding`, `LineageBinding`, and `AuditBinding`.

Generic `Binding` is emitted only when the migration cannot map a custom capability to a built-in typed binding. Review provider selections, generated connection ownership, and secret references before using migrated manifests for production generation.

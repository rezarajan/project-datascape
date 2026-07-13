# Recovery

Use:

```sh
platformctl recover plan
platformctl recover generate --output dist/recovery
```

Recovery artifacts are derived from the graph:

- schema registry rehydration when contracts are projected;
- topic recreation when managed streams exist;
- connector recreation, CDC worker configuration, connector offsets and schema-history state when CDC is selected;
- object archive inventory and replay when `StreamArchiveBinding` exists;
- lineage admission replay when lineage admission lineage is selected;
- audit integrity validation when audit storage is selected;
- recovery dependency graph.

CDC recovery plans identify which state is authoritative, derived, external or unavailable to Datascape. Secret references are listed, but secret values are not copied.

Medallion reconstruction is scheduled for later planned releases.

# CDC Only

CDC is selected by a `CDCBinding` from a `RelationalSource` to an `EventStream`.

```yaml
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: appdb-cdc
spec:
  sourceRef: RelationalSource/appdb
  streamRef: EventStream/order-changes
  mode: cdc
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: order-changes-archive
spec:
  streamRef: EventStream/order-changes
  objectStoreRef: ObjectStore/raw
```

This renders PostgreSQL, change-stream provider, the event bus, the archive writer, and object storage. It does not render application runtime or lineage admission unless additional bindings require them.

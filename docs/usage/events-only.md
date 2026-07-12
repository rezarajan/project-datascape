# Events Only

Native events use an `EventProducer` and `StreamPublishBinding` with a stream. Object archive is explicit.

```yaml
apiVersion: sources.datascape.dev/v1alpha1
kind: EventProducer
metadata:
  name: order-api
spec:
  contractRef: EventContract/order-created
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamPublishBinding
metadata:
  name: order-api-orders
spec:
  sourceRef: EventProducer/order-api
  streamRef: EventStream/orders
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: orders-archive
spec:
  streamRef: EventStream/orders
  objectStoreRef: ObjectStore/raw
```

This renders the application runtime, event bus, archive writer, and object storage. It does not render PostgreSQL or change-stream provider.

# External Kafka Producer

Use an external producer when another team or platform already writes to the stream.

```yaml
apiVersion: platform.datascape.dev/v1alpha1
kind: EventStream
metadata:
  name: orders
---
apiVersion: platform.datascape.dev/v1alpha1
kind: ExternalProducer
metadata:
  name: upstream-orders
spec:
  streamRef: EventStream/orders
  interface: kafka
  verification:
    checks:
      - id: EXT-001
        description: upstream producer can publish to the orders topic
```

Datascape records the external producer, renders verification metadata, and does not generate a producer service.

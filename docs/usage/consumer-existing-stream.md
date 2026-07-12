# Consumer Of Existing Stream

Declare an external stream and bind the consumer to it.

```yaml
apiVersion: platform.datascape.dev/v1alpha1
kind: ExternalEventStream
metadata:
  name: existing-orders
spec:
  interface: kafka
  verification:
    checks:
      - id: EXT-STREAM-001
        description: existing topic is reachable
---
apiVersion: platform.datascape.dev/v1alpha1
kind: ConsumerBinding
metadata:
  name: orders-consumer
spec:
  sourceRef: ExternalEventStream/existing-orders
```

The stream is `externallySatisfied`. The compiler renders connection and verification artifacts, not a broker or producer.

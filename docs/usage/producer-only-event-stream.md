# Producer-Only Event Stream

A stream can have producers before consumers exist. Declare the stream and a producer binding; do not add a placeholder consumer.

```yaml
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata:
  name: orders
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamPublishBinding
metadata:
  name: order-api-produces-orders
spec:
  sourceRef: EventProducer/order-api
  streamRef: EventStream/orders
  mode: application-runtime
```

The compiler validates the producer binding and renders the event-bus/runtime capabilities required by the binding. It does not require a consumer declaration.

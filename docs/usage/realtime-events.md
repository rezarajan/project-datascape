# Realtime Events

Datascape generates application runtime Pub/Sub artifacts when a native event source or producer binding selects the application runtime/native event path:

- Pub/Sub component;
- subscriptions;
- resiliency policy;
- example CloudEvents payload configuration;
- DLQ metadata.

The example publishes `attendance.corrected.v1` through application runtime contracts. PostgreSQL and change-stream provider are omitted unless CDC bindings are also present.

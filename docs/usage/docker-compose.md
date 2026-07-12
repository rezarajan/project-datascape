# Docker Compose

Docker Compose is the selective single-host target.

Generated services are derived from the resolved graph:

- `RelationalSource` alone renders a PostgreSQL source service, not a broker or change-stream provider.
- `CDCBinding` renders PostgreSQL, change-stream provider, event bus, and optional archive services.
- `StreamPublishBinding` renders application-runtime and event-bus services.
- External streams and producers render verification/configuration, not managed producer or broker services.
- lineage admission renders only for `LineageBinding` or policy-required lineage that selects lineage admission.

Compose supports local development and hardened single-host production. It is not distributed high availability.

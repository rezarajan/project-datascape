# Production Compose

The Compose target supports local development and hardened single-host production. Production Compose means one hardened host, not distributed high availability.

```yaml
apiVersion: platform.datascape.dev/v1alpha1
kind: RuntimeProfile
metadata:
  name: single-host-production
spec:
  target: compose
  availability:
    class: single-host-production
  implementations:
    eventBus: redpanda
    cdc: change-stream-provider-postgres
    objectStorage: minio
    postgresSource: postgres
    utility: busybox
```

Production rendering keeps image digests, avoids inline secrets, adds no-new-privileges, drops Linux capabilities where configured, and emits verification and recovery artifacts for the selected graph.

CDC is explicitly single-host in the Compose target. A managed `CDCInstance` renders one Kafka Connect worker service and per-binding registration jobs. Separate CDC instances can receive independent CPU, memory and PID limits. Horizontal CDC replicas are rejected in the first production Compose release because safe task distribution, published ports and offset ownership need a provider contract beyond single-host Compose.

External CDC instances render no worker. `ManagedConnectors` generates connector registration against the declared control endpoint; `ObserveOnly` generates connector configuration and verification artifacts without registration.

The production profile rejects provider services that use tag-only images or
publish a port without a `127.0.0.1` host binding. Providers may additionally
set non-root users, read-only filesystems, `init`, temporary filesystems,
capability drops, security options, graceful shutdown, restart policy, resource
limits, secrets and configs.

Generate a production bundle with a platform whose providers use immutable
image references:

```sh
platformctl generate \
  --platform platform.yaml \
  --profile profiles/single-host-production.yaml \
  --output dist/production
```

Production Compose remains one failure domain. Host TLS, operating-system
hardening, off-host backups and recovery exercises remain operator
responsibilities and must be documented for the deployed platform.

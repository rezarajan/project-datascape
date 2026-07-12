# Resource Hashing

Each logical resource has a canonical digest. Runtime adapters must use rollout-sensitive digests only for configuration that should trigger that specific workload.

Bundle-wide digests must not be placed in pod-template metadata.

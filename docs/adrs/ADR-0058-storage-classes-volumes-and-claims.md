# ADR-0058: Storage Classes, Volumes, Claims and Mount Bindings

Status: Accepted

## Context

Stateful data services need storage policy independent of a particular workload
or deployment target. Raw Compose volume strings cannot express ownership,
capacity, access intent, reclaim behavior or future provisioners.

## Decision

Introduce cluster-scoped `StorageClass` and `PersistentVolume`, namespaced
`PersistentVolumeClaim`, and typed `VolumeMountBinding`. The compiler resolves a
claim to an explicit/static volume or dynamically plans a unique volume through
a compatible class. The normalized storage plan is target-neutral.

The Compose adapter supports named volumes, imported bind paths and external
volumes. Only `compose.named` dynamically provisions Compose volumes. Classes
declare target compatibility; a Longhorn class is representable but is rejected
for Compose because its provisioner requires a Kubernetes target.

## Alternatives considered

Keep volumes inside provider services; copy Kubernetes PVC objects verbatim;
allow implicit arbitrary host mounts.

## Consequences

Storage attachment is visible in plans, provenance and recovery output. Compose
cannot enforce topology, replicated durability or Kubernetes RWX semantics, so
the documentation states those limitations explicitly.

## Security and operations

Bind sources are explicit, path traversal is rejected, and imported storage must
declare verification. `Retain` is the default reclaim policy.

## Reversibility and validation

New target adapters can map the same plan to CSI or cloud volumes. Tests prove
unique dynamic RWO claims, static file claims, mount rendering and target errors.

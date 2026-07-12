# ADR-0008 - Per-resource hashing and rollout isolation

Status: Accepted with scheduled runtime validation

## Context
Deploying a new compiler or platform version must not restart unrelated resources.

## Decision
Each logical resource receives its own canonical digest and rollout-sensitive digest. Bundle-wide digests must not be placed in rollout-triggering fields.

## Alternatives considered
Global platform revision labels; content hashes in primary names; no per-resource hashing.

## Consequences
Adapters must compute workload template digests only from consumed configuration and immutable references.

## Security implications
Resource digests provide targeted integrity evidence without leaking secret values.

## Operational implications
Kubernetes rollout-isolation validation is scheduled for the Kubernetes release.

## Reversibility
Digest inputs can be tightened, but broad global rollout triggers remain prohibited.

## Validation
Current release implements per-resource canonical digests and change-isolation primitives. Kubernetes pod-template validation is scheduled for Current release.

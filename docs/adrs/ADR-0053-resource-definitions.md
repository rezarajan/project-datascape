# ADR-0053: Resource Definitions

Status: Accepted

Current resources use Kubernetes-style `apiVersion`, `kind`, `metadata`, `spec`, and optional `status`. `ResourceDefinition` objects register extension kinds with scope, provider type, binding roles, capabilities, and validation metadata.

Labels, annotations, and status are not semantic inputs for provider selection or rollout-sensitive digests.

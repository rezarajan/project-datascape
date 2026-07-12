# ADR-0057: Class, Instance and Connection Model

Status: Accepted

## Context

A product name, a provisioned database and a consumer access path are different
concerns. Treating all three as fields on `DatabaseConnection` made file-backed
databases look like network servers and prevented reusable infrastructure policy.

## Decision

Use cluster-scoped `DatabaseClass` and `ConnectorClass` resources, namespaced
`DatabaseInstance` resources, and namespaced `DatabaseConnection` resources.
A class describes reusable policy and compatibility, an instance describes a
managed/imported database, and a connection binds the instance to one access
interface. Sources reference connections. Bindings select ingestion behavior.

## Alternatives considered

One polymorphic connection object; engine-specific source kinds; embedding
connector configuration in every pipeline.

## Consequences

One database may expose JDBC query, ODBC query and change-capture connections
without duplicating its lifecycle. More references are required, but each has a
single responsibility. Legacy network connections remain valid during v1alpha1.

## Security and operations

Credentials remain behind `SecretReference`. Connection plans may expose
endpoints, driver names and mounted paths but never resolved secret values.

## Reversibility and validation

The compiler can collapse the resources into a future API version if field use
shows the separation is excessive. Compatibility and file/TCP requirements are
covered by validation and reference-lakehouse tests.

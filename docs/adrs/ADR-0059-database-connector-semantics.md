# ADR-0059: Database Connector Semantics

Status: Accepted

## Context

ODBC and JDBC are client interfaces; TCP and file mounting are transports; CDC
is an operation. Conflating these concepts creates false portability claims.

## Decision

`ConnectorClass` declares `interface`, `transport`, compatible engines, driver,
supported operations and target compatibility. `DatabaseConnection` binds that
class to a `DatabaseInstance`. TCP connectors require an endpoint. File
connectors require a claim and a path visible at the mount point. CDC remains a
typed binding and is not a property of `RelationalSource`.

The reference platform uses PostgreSQL over JDBC/TCP and logical replication,
while SQLite uses ODBC/file semantics with a mounted database file.

## Alternatives considered

Engine-specific connection kinds; a generic URL with no semantic validation;
representing SQLite as a network server.

## Consequences

Provider authors must declare compatibility accurately. Users can see why a
connector is rejected before deployment.

## Security, reversibility and validation

File paths and credential references remain explicit. The model is extensible
to Unix sockets and remote gateways. Validation tests cover incompatible engines
and missing file/TCP requirements.

# Database and Connector Classes

Database lifecycle and database access are modeled separately.

- `DatabaseClass` selects an engine-compatible provider and storage policy.
- `DatabaseInstance` represents one managed, imported or external database.
- `ConnectorClass` describes interface, transport, driver and operations.
- `DatabaseConnection` binds an instance to one connector.
- `RelationalSource` describes the data available through the connection.
- An ingestion binding chooses CDC, batch or another operation.

## PostgreSQL

The reference PostgreSQL class provisions a persistent network service. Its
JDBC connector uses `transport: tcp`, so the connection supplies a service
endpoint. The `CDCBinding` separately references a native logical-replication
connector class backed by Debezium Kafka Connect; query access and change-data
capture are therefore not conflated on the source.

## SQLite

SQLite is a file-backed database. Its ODBC connector uses `transport: file`.
The connection references a claim and the path visible after mounting. It has no
host or port and does not claim CDC support.

An organization may add a separate remote SQLite gateway provider, but that is
a different connector with a network transport; it does not change SQLite file
access into a server implicitly.

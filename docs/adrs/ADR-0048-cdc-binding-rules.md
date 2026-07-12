# ADR-0048: CDC Binding Rules

Status: Accepted

`RelationalSource` alone is a valid source declaration. CDC begins only when a `CDCBinding` binds it to an event stream.

CDC archive and lineage behavior are explicit through `StreamArchiveBinding` and `LineageBinding`.

# ADR-0026 - YAML parser dependency

Status: Accepted

## Context
Current release requires real YAML support, multi-document input, duplicate-key rejection, source locations, and YAML/JSON semantic equivalence.

## Decision
Use `gopkg.in/yaml.v3` for YAML parsing and inspect `yaml.Node` directly before normalizing to JSON.

## Alternatives considered
Keep JSON-only parsing; implement YAML parsing in-house; use a larger configuration framework.

## Consequences
The compiler now has one external Go dependency. Parser logic remains deterministic and rejects duplicate keys before resource normalization.

## Security implications
The dependency expands the supply chain and must remain pinned in `go.sum`. YAML aliases are resolved only into normalized values; input ordering is not semantic.

## Operational implications
Users can provide native YAML, JSON, or multi-document YAML with line and column diagnostics where practical.

## Reversibility
The parser can be replaced behind `internal/spec` if another maintained YAML parser becomes preferable.

## Validation
Parser tests cover native YAML, JSON, multi-document YAML, duplicate keys, unknown fields, and YAML/JSON equivalence.

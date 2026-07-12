# ADR-0002 - Go-only single-binary implementation

Status: Accepted

## Context
The compiler must run without Python, Node.js, Java, shell scripts, or another runtime.

## Decision
Implement `platformctl` entirely in Go and build it as one static CLI binary.

## Alternatives considered
Python CLI, Node.js CLI, mixed-language generator, shell-based task runner.

## Consequences
All compiler functionality must be implemented in Go packages with standard-library-first design.

## Security implications
The runtime attack surface is reduced by avoiding interpreter and package-manager dependencies.

## Operational implications
The documented reproducible build uses `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false`, and an empty build id.

## Reversibility
Reversible only through a major packaging decision.

## Validation
Current release creates a Go module and `cmd/platformctl` binary.

# ADR-0035 - Local audit and execution-evidence storage

Status: Accepted

## Context
Audit and execution evidence must not depend solely on broker, lakehouse, metadata, or orchestration state.

## Decision
Use a separately named local object-storage bucket for execution evidence and generate a stable evidence JSON Schema.

## Alternatives considered
Store evidence only in logs; store evidence in broker topics only; combine raw data and evidence.

## Consequences
Verification, bootstrap, replay, and recovery can reference independent evidence objects.

## Security implications
Evidence paths can be separately credentialed in later targets.

## Operational implications
Evidence survives runtime component recreation when object storage volumes survive.

## Reversibility
Evidence storage can move to an external object store by profile selection.

## Validation
`schemas/evidence.schema.json` and evidence bucket declarations are generated.

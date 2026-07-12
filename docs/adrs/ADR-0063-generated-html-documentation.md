# ADR-0063: Generated Static HTML Documentation

Status: Accepted

## Context

Generating two Markdown files did not create an onboarding or API reference
experience and allowed schema documentation to drift.

## Decision

`platformctl docs build --source docs --output dist/docs` converts the maintained
documentation tree into a responsive, searchable static HTML site. It also
generates the API reference from the same built-in resource definitions used by
validation. `platformctl docs serve` serves an already-built site locally.
`just docs` and `just docs-serve` are the repository shortcuts.

The site has no runtime server dependency and contains local CSS and JavaScript.
Documentation links are rewritten from Markdown to HTML during generation.

## Alternatives considered

Commit generated HTML; require a global Python/Node documentation tool; maintain
the API reference manually.

## Consequences

The Go binary includes one Markdown-rendering dependency. The generated site is
not committed and can be hosted on any static server.

## Security, reversibility and validation

The builder reads only the selected source tree and writes through the artifact
boundary. Tests verify navigation, assets and generated API content.

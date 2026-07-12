# Provider Package Authoring

A provider package connects portable capabilities to a target implementation.
It declares package and contract versions, capabilities, binding kinds, target
compatibility, constrained services/artifacts and conformance identifiers.

Capability names describe behavior, not products. Use
`datascape.dev/source.change-stream`, not `datascape.dev/postgres`.

Provider services are normalized into the target-neutral plan before a target
adapter sees them. Production Compose profiles reject unpinned images and
non-localhost published ports. Elevated requirements must therefore be explicit
and reviewed rather than hidden in target YAML.

Custom resource kinds use `ResourceDefinition`; custom relationships use
`BindingDefinition` and generic `Binding`. The compiler must not require a new
kind switch for either extension.

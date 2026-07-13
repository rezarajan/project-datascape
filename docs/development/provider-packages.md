# Provider Package Authoring

A provider package connects portable capabilities to a target implementation.
It declares package and contract versions, capabilities, binding kinds, target
compatibility, constrained services/artifacts and conformance identifiers.

Capability names describe behavior, not products. Use
`datascape.dev/source.change-stream`, not `datascape.dev/postgres`.

CDC providers should declare `datascape.dev/source.change-stream` and `CDCBinding` support, then expose runtime behavior through `CDCClass` and `CDCInstance` resources. Connector-native configuration must be expressed with constrained mappings from normalized fields to provider keys. Do not add executable templates, arbitrary scripts or in-process plugins to provider declarations.

Provider operation definitions declare action name, applicable kinds, JSON Schema parameters, mutability, destructiveness, idempotency, required capabilities, execution contract, verification contract and rollback or recovery contract. Core compiler logic should not grow product-specific switches for every operation.

Provider services are normalized into the target-neutral plan before a target
adapter sees them. Production Compose profiles reject unpinned images and
non-localhost published ports. Elevated requirements must therefore be explicit
and reviewed rather than hidden in target YAML.

Custom resource kinds use `ResourceDefinition`; custom relationships use
`BindingDefinition` and generic `Binding`. The compiler must not require a new
kind switch for either extension.

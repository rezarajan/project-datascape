# Adding a Component Adapter

Adapters live outside core compiler logic.

Implementation rules:

- add capability metadata to the provider registry;
- keep vendor-specific rendering under `internal/adapters`;
- consume typed IR rather than raw user resources;
- emit deterministic files with stable paths and modes;
- expose generated files through `resources.json`;
- add conformance checks for the shared capability;
- add change-isolation tests proving unrelated artifacts remain byte-identical.

Do not import adapter packages into `internal/domain`, `internal/spec`, or `internal/validation`.

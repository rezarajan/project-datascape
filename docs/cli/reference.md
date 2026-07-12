# CLI Reference

Commands:

- `platformctl init`
- `platformctl validate`
- `platformctl plan`
- `platformctl generate`
- `platformctl diff`
- `platformctl inspect`
- `platformctl api-resources`
- `platformctl api-definitions`
- `platformctl providers`
- `platformctl bindings`
- `platformctl explain <kind>`
- `platformctl migrate`
- `platformctl docs build --source docs --output dist/docs`
- `platformctl docs serve --directory dist/docs --listen 127.0.0.1:8000`
- `platformctl secrets init --bundle dist/reference --development`
- `platformctl verify`
- `platformctl conformance`
- `platformctl recover plan`
- `platformctl recover generate`
- `platformctl version`

Commands operate on deterministic compiler artifacts. Compose generation derives services from resolved provider instances and binding capabilities.

Common generation flow:

```sh
platformctl generate --platform examples/change-stream/platform.yaml --profile profiles/local.yaml --output dist/local
platformctl verify --bundle dist/local --runtime
```

Use `platformctl migrate <old-manifest.yaml>` to produce best-effort current resources from older manifest shapes.

`secrets init` is deliberately restricted to development bundles. It writes
`.env` with mode `0600`, does not print secret values, and refuses to overwrite
an existing file unless `--force` is provided.

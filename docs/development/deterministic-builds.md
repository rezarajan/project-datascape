# Deterministic Builds

Use:

```sh
CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" ./cmd/platformctl
```

Do not embed build time. Version and source commit are explicit release inputs.

Bundle determinism checks should generate the same input twice and compare directories:

```sh
platformctl generate --platform examples/postgres-cdc/platform.yaml --profile profiles/local.yaml --output /tmp/a
platformctl generate --platform examples/postgres-cdc/platform.yaml --profile profiles/local.yaml --output /tmp/b
diff -qr /tmp/a /tmp/b
```

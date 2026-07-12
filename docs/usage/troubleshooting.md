# Troubleshooting

Diagnostics include severity, code, resource, field path, message, remediation, and source file.

Common failures:

- `DSPEC018`: duplicate YAML key; remove the duplicate.
- `DSPEC021`: unknown spec field; use the current v1alpha1 schema.
- `DREF002`: reference points to a missing logical resource.
- `DGRAPH002`: consumer binding has no producer-backed, imported, or external stream.
- `DGRAPH003`: external resource lacks verification and no allowed trust override exists.
- `DOVR003`: strict policy rejects a production override.
- `DCAP002`: selected implementation is absent from the provider registry.
- `DCAP005`: image digest is missing and no development exception is enabled.

For Compose startup, provide environment values with `--env-file .env`.

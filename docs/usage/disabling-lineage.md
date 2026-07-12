# Disabling Lineage

Lineage is optional unless required by policy. Omit `LineageBinding` and lineage policy requirements to disable lineage artifacts.

```yaml
apiVersion: platform.datascape.dev/v1alpha1
kind: PlatformPolicy
metadata:
  name: development
spec:
  validationMode: permissive
  requirements:
    lineage: false
```

With no lineage binding, Datascape does not render lineage admission configuration, lineage admission services, or lineage recovery files.

# Enabling lineage admission

lineage admission is selected by a `LineageBinding`.

```yaml
apiVersion: bindings.datascape.dev/v1alpha1
kind: LineageBinding
metadata:
  name: appdb-lineage
spec:
  sourceRef: RelationalSource/appdb
  sinkRef: LineageSink/lineage-admission
  mode: lineage-admission
```

The compiler renders lineage admission configuration, service scaffolding, verification checks, and recovery replay artifacts for that binding.

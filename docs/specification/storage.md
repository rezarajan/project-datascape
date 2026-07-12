# Storage Classes, Volumes and Claims

`StorageClass` is cluster-scoped policy. A class declares a portable provisioner
name, target compatibility, reclaim policy, binding mode and provider-specific
parameters. Compose currently supports dynamic `compose.named` volumes and
static bind/external volumes.

```yaml
apiVersion: storage.datascape.dev/v1alpha1
kind: StorageClass
metadata:
  name: local-durable
spec:
  provisioner: compose.named
  targetCompatibility: [compose]
  reclaimPolicy: Retain
  volumeBindingMode: Immediate
  default: true
```

A claim requests capacity and access intent:

```yaml
apiVersion: storage.datascape.dev/v1alpha1
kind: PersistentVolumeClaim
metadata:
  name: database-data
  namespace: education
spec:
  capacity: 10Gi
  accessModes: [ReadWriteOnce]
```

The compiler binds the claim to a compatible static volume or plans a unique
named volume. Static bindings require the same class, sufficient capacity and
all requested access modes; one volume cannot be bound to multiple claims. A
`VolumeMountBinding` attaches the claim to a data service:

```yaml
apiVersion: bindings.datascape.dev/v1alpha1
kind: VolumeMountBinding
metadata:
  name: database-storage
  namespace: education
spec:
  claimRef: PersistentVolumeClaim/database-data
  workloadRef: DatabaseInstance/operational-db
  mountPath: /var/lib/postgresql/data
```

Compose does not enforce multi-node topology or replicated durability. Access
modes are validated as intent and must also be supported by the chosen target.

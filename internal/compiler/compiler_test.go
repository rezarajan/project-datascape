package compiler

import (
	"context"
	"os"
	"runtime"
	"strings"
	"testing"

	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/spec"
)

func TestReferenceLakehouseCompilesWithDistinctClaimsAndOrderedCommands(t *testing.T) {
	platform, err := os.ReadFile("../../examples/reference-lakehouse/platform.yaml")
	if err != nil {
		t.Fatal(err)
	}
	profile, err := os.ReadFile("../../profiles/reference.yaml")
	if err != nil {
		t.Fatal(err)
	}
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "platform.yaml", Content: platform}, {Name: "profile.yaml", Content: profile}}, Options{CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("reference diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Plan.Storage.Claims) != 5 || len(result.Plan.Storage.Mounts) != 6 {
		t.Fatalf("unexpected storage plan: %#v", result.Plan.Storage)
	}
	bound := map[string]bool{}
	for _, claim := range result.Plan.Storage.Claims {
		key := claim.BoundVolume.CanonicalString()
		if bound[key] && claim.Identity.Name != "sqlite-data" {
			t.Fatalf("managed ReadWriteOnce volume reused: %s", key)
		}
		bound[key] = true
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	if !strings.Contains(compose, `command: ["redpanda","start","--overprovisioned"`) {
		t.Fatalf("provider command ordering was not preserved:\n%s", compose)
	}
	for _, expected := range []string{"postgres-source:", "sqlite-source:", "attendance-changes:", "cdc-connector:", "cdc-register:", "lakehouse-store:", "iceberg-catalog:", "lakehouse-pipeline:", "query-engine:", "lineage-backend:"} {
		if !strings.Contains(compose, expected) {
			t.Fatalf("reference compose missing %s", expected)
		}
	}
}

func TestLonghornStorageClassRejectedForCompose(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "longhorn.yaml", Content: []byte(`apiVersion: storage.datascape.dev/v1alpha1
kind: StorageClass
metadata:
  name: longhorn
spec:
  provisioner: driver.longhorn.io
  targetCompatibility: [kubernetes]
  reclaimPolicy: Retain
  volumeBindingMode: WaitForFirstConsumer
`)}}, Options{Target: "compose"})
	if !hasDiagnostic(result.Diagnostics, "DSTOR007") {
		t.Fatalf("expected DSTOR007, got %#v", result.Diagnostics)
	}
}

func TestExplicitVolumeBindingEnforcesStorageClassCompatibility(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "storage.yaml", Content: []byte(`apiVersion: storage.datascape.dev/v1alpha1
kind: StorageClass
metadata: {name: fast}
spec: {provisioner: compose.named}
---
apiVersion: storage.datascape.dev/v1alpha1
kind: StorageClass
metadata: {name: archive}
spec: {provisioner: compose.named}
---
apiVersion: storage.datascape.dev/v1alpha1
kind: PersistentVolume
metadata: {name: imported}
spec:
  storageClassRef: StorageClass/archive
  capacity: 1Gi
  accessModes: [ReadWriteOnce]
---
apiVersion: storage.datascape.dev/v1alpha1
kind: PersistentVolumeClaim
metadata: {name: application-data}
spec:
  storageClassRef: StorageClass/fast
  volumeRef: PersistentVolume/imported
  capacity: 10Gi
  accessModes: [ReadWriteOnce]
`)}}, Options{Target: "compose"})
	if !hasDiagnostic(result.Diagnostics, "DSTOR011") {
		t.Fatalf("expected storage class mismatch, got %#v", result.Diagnostics)
	}
}

func TestCompileDocumentsDeterministicAcrossInputOrderAndGOMAXPROCS(t *testing.T) {
	ctx := context.Background()
	docs := sampleDocs()
	beforeProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(beforeProcs)
	first := CompileDocuments(ctx, docs, Options{CompilerVersion: "test"})
	runtime.GOMAXPROCS(4)
	second := CompileDocuments(ctx, []spec.NamedDocument{docs[1], docs[0]}, Options{CompilerVersion: "test"})

	if domain.HasErrors(first.Diagnostics) {
		t.Fatalf("first compile diagnostics: %#v", first.Diagnostics)
	}
	if domain.HasErrors(second.Diagnostics) {
		t.Fatalf("second compile diagnostics: %#v", second.Diagnostics)
	}
	if first.Provenance.BundleDigest != second.Provenance.BundleDigest {
		t.Fatalf("bundle digests differ:\n%s\n%s", first.Provenance.BundleDigest, second.Provenance.BundleDigest)
	}
	assertFilesEqual(t, first.Files, second.Files)
}

func TestConsolidatedComposeBundleFromBindings(t *testing.T) {
	result := CompileDocuments(context.Background(), sampleDocs(), Options{CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Plan.ProviderInstances) == 0 {
		t.Fatal("expected provider instances")
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	for _, service := range []string{"relational-source:", "event-stream:", "object-store:"} {
		if !strings.Contains(compose, service) {
			t.Fatalf("compose file missing %s:\n%s", service, compose)
		}
	}
	checks := fileContent(t, result.Files, "verification/checks.json")
	for _, id := range []string{"BINDING-001", "CDC-001", "EVENTSTREAM-001", "ARCHIVE-001", "COMPOSE-001"} {
		if !strings.Contains(checks, id) {
			t.Fatalf("verification checks should include %s: %s", id, checks)
		}
	}
	env := fileContent(t, result.Files, ".env.example")
	if !strings.Contains(env, "EDUCATION_STUDENT_RECORDS_DB_CREDENTIALS_PASSWORD=change-me-local") {
		t.Fatalf("env-backed SecretReference should render logical key placeholders: %s", env)
	}
}

func TestLegacyCurrentKindRejected(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: PostgresSource
metadata:
  name: appdb
spec: {}
`),
	}}, Options{})
	if !hasDiagnostic(result.Diagnostics, "DLEGACY001") {
		t.Fatalf("expected DLEGACY001, got %#v", result.Diagnostics)
	}
}

func TestCustomResourceDefinitionCompilesWithoutCoreEdits(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: ResourceDefinition
metadata:
  name: custom-source
spec:
  apiVersion: custom.datascape.dev/v1alpha1
  kind: CustomSource
  scope: Namespaced
  capabilities: [datascape.dev/source.relational]
  schema:
    required: [endpoint]
    properties:
      endpoint:
        type: string
---
apiVersion: custom.datascape.dev/v1alpha1
kind: CustomSource
metadata:
  name: upstream
spec:
  endpoint: ref:upstream
`),
	}}, Options{Target: "compose"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	if digestByKind(result, "CustomSource") == "" {
		t.Fatal("expected custom resource in resource inventory")
	}
	if !strings.Contains(fileContent(t, result.Files, "compose.yaml"), "relational-source:") {
		t.Fatal("custom source capability should select a provider service")
	}
}

func TestCustomResourceDefinitionSchemaValidation(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: ResourceDefinition
metadata:
  name: custom-source
spec:
  apiVersion: custom.datascape.dev/v1alpha1
  kind: CustomSource
  schema:
    required: [endpoint]
---
apiVersion: custom.datascape.dev/v1alpha1
kind: CustomSource
metadata:
  name: upstream
spec: {}
`),
	}}, Options{Target: "compose"})
	if !hasDiagnostic(result.Diagnostics, "DRES003") {
		t.Fatalf("expected DRES003, got %#v", result.Diagnostics)
	}
}

func TestSourceOnlyCompilationDoesNotRenderStreamServices(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata:
  name: appdb-credentials
spec:
  backend: env
  keys: [username, password]
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata:
  name: appdb
spec:
  engine: postgres
  host: relational-source
  port: 5432
  database: app
  credentialsRef: SecretReference/appdb-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata:
  name: appdb
spec:
  connectionRef: DatabaseConnection/appdb
`),
	}}, Options{Target: "compose"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	if !strings.Contains(compose, "relational-source:") {
		t.Fatal("expected source service")
	}
	if strings.Contains(compose, "event-stream:") || strings.Contains(compose, "object-store:") {
		t.Fatalf("source-only bundle should not render unrelated services:\n%s", compose)
	}
}

func TestOneSourceCanBindToMultipleTargets(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata:
  name: appdb-credentials
spec:
  backend: env
  keys: [username, password]
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata:
  name: appdb
spec:
  engine: postgres
  host: relational-source
  port: 5432
  database: app
  credentialsRef: SecretReference/appdb-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata:
  name: appdb
spec:
  connectionRef: DatabaseConnection/appdb
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata:
  name: app-changes
---
apiVersion: stores.datascape.dev/v1alpha1
kind: ObjectStore
metadata:
  name: archive-a
---
apiVersion: stores.datascape.dev/v1alpha1
kind: ObjectStore
metadata:
  name: archive-b
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: appdb-changes
spec:
  sourceRef: RelationalSource/appdb
  streamRef: EventStream/app-changes
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: archive-a
spec:
  streamRef: EventStream/app-changes
  objectStoreRef: ObjectStore/archive-a
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: archive-b
spec:
  streamRef: EventStream/app-changes
  objectStoreRef: ObjectStore/archive-b
`),
	}}, Options{Target: "compose"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Plan.Bindings) != 3 {
		t.Fatalf("expected 3 bindings, got %d", len(result.Plan.Bindings))
	}
}

func TestBindingOnlyDigestChangesDoNotChangeSourceDigest(t *testing.T) {
	first := CompileDocuments(context.Background(), sampleDocs(), Options{CompilerVersion: "test"})
	modified := sampleDocs()
	modified[1].Content = []byte(strings.Replace(string(modified[1].Content), "student-records-change-stream", "student-records-change-stream-renamed", 1))
	second := CompileDocuments(context.Background(), modified, Options{CompilerVersion: "test"})
	if domain.HasErrors(first.Diagnostics) || domain.HasErrors(second.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v %#v", first.Diagnostics, second.Diagnostics)
	}
	if digestByKind(first, "RelationalSource") != digestByKind(second, "RelationalSource") {
		t.Fatal("binding-only change should not alter source resource digest")
	}
	if digestByKindAndName(first, "CDCBinding", "student-records-change-stream") == "" {
		t.Fatal("original binding digest missing")
	}
	if digestByKindAndName(second, "CDCBinding", "student-records-change-stream-renamed") == "" {
		t.Fatal("renamed binding digest missing")
	}
}

func TestTypedAndGenericBindingsNormalizeToSameGraph(t *testing.T) {
	typed := CompileDocuments(context.Background(), sampleDocs(), Options{CompilerVersion: "test"})
	genericDocs := sampleDocs()
	genericDocs[1].Content = []byte(strings.ReplaceAll(string(genericDocs[1].Content), `apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: student-records-change-stream
  namespace: education
spec:
  sourceRef: RelationalSource/student-records
  streamRef: EventStream/attendance-changes`, `apiVersion: platform.datascape.dev/v1alpha1
kind: Binding
metadata:
  name: student-records-change-stream
  namespace: education
spec:
  capability: datascape.dev/source.change-stream
  sourceRef: RelationalSource/student-records
  targetRef: EventStream/attendance-changes`))
	genericDocs[1].Content = []byte(strings.ReplaceAll(string(genericDocs[1].Content), `apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: attendance-changes-object-store
  namespace: education
spec:
  streamRef: EventStream/attendance-changes
  objectStoreRef: ObjectStore/evidence-store`, `apiVersion: platform.datascape.dev/v1alpha1
kind: Binding
metadata:
  name: attendance-changes-object-store
  namespace: education
spec:
  capability: datascape.dev/store.object
  sourceRef: EventStream/attendance-changes
  targetRef: ObjectStore/evidence-store`))
	generic := CompileDocuments(context.Background(), genericDocs, Options{CompilerVersion: "test"})
	if domain.HasErrors(typed.Diagnostics) || domain.HasErrors(generic.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v %#v", typed.Diagnostics, generic.Diagnostics)
	}
	if len(typed.Plan.Bindings) != len(generic.Plan.Bindings) {
		t.Fatalf("binding counts differ: %d != %d", len(typed.Plan.Bindings), len(generic.Plan.Bindings))
	}
	typedEdges := bindingEdges(typed)
	genericEdges := bindingEdges(generic)
	if len(typedEdges) != len(genericEdges) {
		t.Fatalf("edge counts differ: %#v %#v", typedEdges, genericEdges)
	}
	for key, left := range typedEdges {
		right, ok := genericEdges[key]
		if !ok {
			t.Fatalf("generic binding edge missing %s", key)
		}
		if left.Capability != right.Capability || left.Source.CanonicalString() != right.Source.CanonicalString() || left.Target.CanonicalString() != right.Target.CanonicalString() || left.ProviderInstance.CanonicalString() != right.ProviderInstance.CanonicalString() {
			t.Fatalf("binding edge %s graph differs:\n%#v\n%#v", key, left, right)
		}
	}
}

func TestTypedBindingRejectsWrongTargetKind(t *testing.T) {
	docs := sampleDocs()
	docs[1].Content = []byte(strings.Replace(string(docs[1].Content), "streamRef: EventStream/attendance-changes", "streamRef: ObjectStore/evidence-store", 1))
	result := CompileDocuments(context.Background(), docs, Options{CompilerVersion: "test"})
	if !hasDiagnostic(result.Diagnostics, "DBIND011") {
		t.Fatalf("expected DBIND011, got %#v", result.Diagnostics)
	}
}

func TestTypedBindingRejectsProviderMismatch(t *testing.T) {
	docs := sampleDocs()
	docs[1].Content = []byte(strings.Replace(string(docs[1].Content), "streamRef: EventStream/attendance-changes", "streamRef: EventStream/attendance-changes\n  providerInstanceRef: ProviderInstance/default/local-object-store", 1))
	result := CompileDocuments(context.Background(), docs, Options{CompilerVersion: "test"})
	if !hasDiagnostic(result.Diagnostics, "DBIND015") {
		t.Fatalf("expected DBIND015, got %#v", result.Diagnostics)
	}
}

func TestSchemaFileIsDeterministic(t *testing.T) {
	first, err := SchemaFile()
	if err != nil {
		t.Fatal(err)
	}
	second, err := SchemaFile()
	if err != nil {
		t.Fatal(err)
	}
	if string(first.Content) != string(second.Content) {
		t.Fatal("schema content is not deterministic")
	}
}

func sampleDocs() []spec.NamedDocument {
	return []spec.NamedDocument{
		{
			Name: "platform.yaml",
			Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: Target
metadata:
  name: local
spec:
  type: compose
---
apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata:
  name: student-records-db-credentials
  namespace: education
spec:
  backend: env
  keys: [username, password]
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata:
  name: student-records-db
  namespace: education
spec:
  engine: postgres
  host: relational-source
  port: 5432
  database: attendance
  credentialsRef: SecretReference/student-records-db-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata:
  name: student-records
  namespace: education
spec:
  connectionRef: DatabaseConnection/student-records-db
  tables: [student_attendance]
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata:
  name: attendance-changes
  namespace: education
spec:
  eventClass: change
`),
		},
		{
			Name: "bindings.yaml",
			Content: []byte(`apiVersion: stores.datascape.dev/v1alpha1
kind: ObjectStore
metadata:
  name: evidence-store
  namespace: education
spec:
  bucket: attendance-evidence
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: student-records-change-stream
  namespace: education
spec:
  sourceRef: RelationalSource/student-records
  streamRef: EventStream/attendance-changes
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: StreamArchiveBinding
metadata:
  name: attendance-changes-object-store
  namespace: education
spec:
  streamRef: EventStream/attendance-changes
  objectStoreRef: ObjectStore/evidence-store
`),
		},
	}
}

func assertFilesEqual(t *testing.T, a, b []artifact.File) {
	t.Helper()
	if len(a) != len(b) {
		t.Fatalf("file counts differ: %d != %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path {
			t.Fatalf("file path %d differs: %s != %s", i, a[i].Path, b[i].Path)
		}
		if a[i].Mode != b[i].Mode {
			t.Fatalf("file mode %s differs: %o != %o", a[i].Path, a[i].Mode, b[i].Mode)
		}
		if string(a[i].Content) != string(b[i].Content) {
			t.Fatalf("file content %s differs", a[i].Path)
		}
	}
}

func digestByKind(result Result, kind string) string {
	for _, resource := range result.Resources {
		if resource.Kind == kind {
			return resource.CanonicalDigest
		}
	}
	return ""
}

func digestByKindAndName(result Result, kind, name string) string {
	for _, resource := range result.Resources {
		if resource.Kind == kind && resource.Identity.Name == name {
			return resource.CanonicalDigest
		}
	}
	return ""
}

func fileContent(t *testing.T, files []artifact.File, path string) string {
	t.Helper()
	for _, file := range files {
		if file.Path == path {
			return string(file.Content)
		}
	}
	t.Fatalf("file %s not found", path)
	return ""
}

func bindingEdges(result Result) map[string]ir.BindingPlan {
	out := map[string]ir.BindingPlan{}
	for _, binding := range result.Plan.Bindings {
		key := binding.Capability + "|" + binding.Source.CanonicalString() + "|" + binding.Target.CanonicalString()
		out[key] = binding
	}
	return out
}

func hasDiagnostic(diags []domain.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func resourceHasOverride(resources []ir.ResourcePlan, kind, name string) bool {
	for _, resource := range resources {
		if resource.Kind == kind && resource.Identity.Name == name && len(resource.Overrides) > 0 {
			return true
		}
	}
	return false
}

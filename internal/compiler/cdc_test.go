package compiler

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/spec"
)

func TestCDCSharedInstanceProducesOneWorkerAndTwoConnectorConfigs(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "cdc.yaml", Content: []byte(twoSourceSharedCDCManifest())}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Plan.CDC.Connectors) != 2 {
		t.Fatalf("expected 2 connector plans, got %d", len(result.Plan.CDC.Connectors))
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	if strings.Count(compose, "  cdc-education-shared-cdc:\n    image:") != 1 {
		t.Fatalf("expected one shared CDC worker:\n%s", compose)
	}
	for _, path := range []string{
		"configuration/cdc/education/shared-cdc/finance-cdc.json",
		"configuration/cdc/education/shared-cdc/operations-cdc.json",
	} {
		content := fileContent(t, result.Files, path)
		if content == "" || !strings.Contains(content, "database.hostname") {
			t.Fatalf("missing connector config %s: %s", path, content)
		}
	}
	if fileContent(t, result.Files, "configuration/cdc/education/shared-cdc/finance-cdc.json") == fileContent(t, result.Files, "configuration/cdc/education/shared-cdc/operations-cdc.json") {
		t.Fatal("connector configs should be independent and not overwritten")
	}
}

func TestCDCSplitInstancesApplyWorkerResourcesAndRouting(t *testing.T) {
	manifest := strings.Replace(twoSourceSharedCDCManifest(), "cdcRef: CDCInstance/shared-cdc", "cdcRef: CDCInstance/fast-cdc", 1)
	manifest = strings.Replace(manifest, "name: shared-cdc\n  namespace: education\nspec:\n  classRef: CDCClass/debezium-kafka-connect\n  providerInstanceRef: ProviderInstance/default/local-cdc\n  replicas: 1\n  resources: {cpus: \"1\", memory: 1g, pidsLimit: 256}", "name: shared-cdc\n  namespace: education\nspec:\n  classRef: CDCClass/debezium-kafka-connect\n  providerInstanceRef: ProviderInstance/default/local-cdc\n  replicas: 1\n  resources: {cpus: \"1\", memory: 1g, pidsLimit: 256}\n---\napiVersion: cdc.datascape.dev/v1alpha1\nkind: CDCInstance\nmetadata:\n  name: fast-cdc\n  namespace: education\nspec:\n  classRef: CDCClass/debezium-kafka-connect\n  providerInstanceRef: ProviderInstance/default/local-cdc\n  replicas: 1\n  resources: {cpus: \"2\", memory: 2g, pidsLimit: 512}", 1)
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "cdc.yaml", Content: []byte(manifest)}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	for _, service := range []string{"cdc-education-fast-cdc:", "cdc-education-shared-cdc:", `cpus: "2"`, `mem_limit: "2g"`, "pids_limit: 512"} {
		if !strings.Contains(compose, service) {
			t.Fatalf("compose missing %s:\n%s", service, compose)
		}
	}
	if result.Plan.CDC.Connectors[0].CDCInstance.CanonicalString() == result.Plan.CDC.Connectors[1].CDCInstance.CanonicalString() {
		t.Fatal("expected connectors routed to different CDC instances")
	}
}

func TestExternalDatabaseInternalCDCUsesEndpointAndSecretRefs(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "external-db.yaml", Content: []byte(externalDatabaseManifest())}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	compose := fileContent(t, result.Files, "compose.yaml")
	if strings.Contains(compose, "external-finance-db:") || strings.Contains(compose, "relational-source:") {
		t.Fatalf("external database should not render a database/source service:\n%s", compose)
	}
	if !strings.Contains(compose, "cdc-education-shared-cdc:") {
		t.Fatalf("managed CDC worker missing:\n%s", compose)
	}
	config := fileContent(t, result.Files, "configuration/cdc/education/shared-cdc/finance-cdc.json")
	for _, expected := range []string{`"database.hostname":"finance.db.example"`, `"database.user":"${EDUCATION_EXTERNAL_FINANCE_CREDENTIALS_USERNAME}"`, `"database.password":"${EDUCATION_EXTERNAL_FINANCE_CREDENTIALS_PASSWORD}"`} {
		if !strings.Contains(config, expected) {
			t.Fatalf("connector config missing %s: %s", expected, config)
		}
	}
	if strings.Contains(config, "supersecret") || strings.Contains(fileContent(t, result.Files, "compose.yaml"), "supersecret") {
		t.Fatal("secret value leaked into generated artifacts")
	}
}

func TestExternalCDCManagedAndObserveOnlyModes(t *testing.T) {
	managed := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "external-cdc.yaml", Content: []byte(externalCDCManifest("ManagedConnectors"))}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(managed.Diagnostics) {
		t.Fatalf("managed diagnostics: %#v", managed.Diagnostics)
	}
	managedCompose := fileContent(t, managed.Files, "compose.yaml")
	if strings.Contains(managedCompose, "cdc-education-external-cdc:") {
		t.Fatalf("external CDC must not render a worker:\n%s", managedCompose)
	}
	if !strings.Contains(managedCompose, "cdc-register-education-external-cdc-finance-cdc:") || !strings.Contains(managedCompose, "connect.example") {
		t.Fatalf("external ManagedConnectors should render endpoint-targeted registration:\n%s", managedCompose)
	}

	observe := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "external-cdc.yaml", Content: []byte(externalCDCManifest("ObserveOnly"))}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(observe.Diagnostics) {
		t.Fatalf("observe diagnostics: %#v", observe.Diagnostics)
	}
	observeCompose := fileContent(t, observe.Files, "compose.yaml")
	if strings.Contains(observeCompose, "cdc-education-external-cdc:") || strings.Contains(observeCompose, "cdc-register-education-external-cdc-finance-cdc:") {
		t.Fatalf("ObserveOnly should render no worker or registration:\n%s", observeCompose)
	}
	if fileContent(t, observe.Files, "configuration/cdc/education/external-cdc/finance-cdc.json") == "" {
		t.Fatal("ObserveOnly should still emit connector configuration")
	}
	if observe.Plan.CDC.Connectors[0].State != "externallyAppliedOrPending" {
		t.Fatalf("unexpected ObserveOnly state: %s", observe.Plan.CDC.Connectors[0].State)
	}
}

func TestProviderInstancesForSameCapabilityDoNotCollapse(t *testing.T) {
	result := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "providers.yaml", Content: []byte(twoProviderCDCManifest())}}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(result.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
	seen := map[string]bool{}
	for _, planned := range result.Plan.PlannedResources {
		if planned.Capability == "datascape.dev/source.change-stream" {
			seen[planned.ProviderInstance.Namespace+"/"+planned.ProviderInstance.Name] = true
		}
	}
	if !seen["education/cdc-a"] || !seen["education/cdc-b"] {
		t.Fatalf("expected both CDC provider instances in planned resources, got %#v", seen)
	}
}

func TestCDCConnectorOutputDeterministicAcrossInputOrderAndGOMAXPROCS(t *testing.T) {
	docs := []spec.NamedDocument{{Name: "a.yaml", Content: []byte(twoSourceSharedCDCManifest())}, {Name: "b.yaml", Content: []byte("")}}
	before := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(before)
	first := CompileDocuments(context.Background(), docs[:1], Options{Target: "compose", CompilerVersion: "test"})
	runtime.GOMAXPROCS(4)
	second := CompileDocuments(context.Background(), []spec.NamedDocument{docs[0]}, Options{Target: "compose", CompilerVersion: "test"})
	if domain.HasErrors(first.Diagnostics) || domain.HasErrors(second.Diagnostics) {
		t.Fatalf("unexpected diagnostics: %#v %#v", first.Diagnostics, second.Diagnostics)
	}
	if fileContent(t, first.Files, "configuration/cdc/education/shared-cdc/finance-cdc.json") != fileContent(t, second.Files, "configuration/cdc/education/shared-cdc/finance-cdc.json") {
		t.Fatal("connector configuration changed across GOMAXPROCS")
	}
	assertFilesEqual(t, first.Files, second.Files)
}

func TestCDCIncompatibleEngineConnectorAndProductionImplicitRuntimeFail(t *testing.T) {
	badConnector := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "bad.yaml", Content: []byte(strings.Replace(externalDatabaseManifest(), "compatibleEngines: [postgresql]", "compatibleEngines: [sqlite]", 1))}}, Options{Target: "compose", CompilerVersion: "test"})
	if !hasDiagnostic(badConnector.Diagnostics, "DCONN010") {
		t.Fatalf("expected DCONN010, got %#v", badConnector.Diagnostics)
	}
	productionImplicit := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "prod.yaml", Content: []byte(productionImplicitCDCManifest())}}, Options{Target: "compose", CompilerVersion: "test"})
	if !hasDiagnostic(productionImplicit.Diagnostics, "DCDC008") {
		t.Fatalf("expected DCDC008, got %#v", productionImplicit.Diagnostics)
	}
}

func TestCDCOperationSafetyValidation(t *testing.T) {
	observeMutation := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "ops.yaml", Content: []byte(externalCDCManifest("ObserveOnly") + `
---
apiVersion: operations.datascape.dev/v1alpha1
kind: CDCOperation
metadata: {name: pause-finance, namespace: education}
spec:
  targetRef: CDCBinding/finance-cdc
  action: PauseConnector
  idempotencyKey: pause-finance
`)}}, Options{Target: "compose", CompilerVersion: "test"})
	if !hasDiagnostic(observeMutation.Diagnostics, "DOPS005") {
		t.Fatalf("expected ObserveOnly mutation rejection, got %#v", observeMutation.Diagnostics)
	}
	reset := CompileDocuments(context.Background(), []spec.NamedDocument{{Name: "ops.yaml", Content: []byte(twoSourceSharedCDCManifest() + `
---
apiVersion: operations.datascape.dev/v1alpha1
kind: CDCOperation
metadata: {name: reset-finance, namespace: education}
spec:
  targetRef: CDCBinding/finance-cdc
  action: ResetOffsets
  idempotencyKey: reset-finance
`)}}, Options{Target: "compose", CompilerVersion: "test"})
	for _, code := range []string{"DOPS007", "DOPS008", "DOPS009"} {
		if !hasDiagnostic(reset.Diagnostics, code) {
			t.Fatalf("expected %s, got %#v", code, reset.Diagnostics)
		}
	}
}

func twoSourceSharedCDCManifest() string {
	return commonCDCPrelude() + `
---
apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata: {name: finance-credentials, namespace: education}
spec: {backend: env, keys: [username, password]}
---
apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata: {name: operations-credentials, namespace: education}
spec: {backend: env, keys: [username, password]}
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata: {name: finance-db, namespace: education}
spec:
  engine: postgresql
  host: finance-db
  port: 5432
  database: finance
  credentialsRef: SecretReference/finance-credentials
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata: {name: operations-db, namespace: education}
spec:
  engine: postgresql
  host: operations-db
  port: 5432
  database: operations
  credentialsRef: SecretReference/operations-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata: {name: finance, namespace: education}
spec: {connectionRef: DatabaseConnection/finance-db, tables: [public.payments]}
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata: {name: operations, namespace: education}
spec: {connectionRef: DatabaseConnection/operations-db, tables: [public.orders]}
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata: {name: finance-events, namespace: education}
spec: {}
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata: {name: operations-events, namespace: education}
spec: {}
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata: {name: finance-cdc, namespace: education}
spec:
  sourceRef: RelationalSource/finance
  streamRef: EventStream/finance-events
  cdcRef: CDCInstance/shared-cdc
  connectorClassRef: ConnectorClass/postgres-debezium
  tables: [public.payments, public.invoices]
  snapshot: initial
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata: {name: operations-cdc, namespace: education}
spec:
  sourceRef: RelationalSource/operations
  streamRef: EventStream/operations-events
  cdcRef: CDCInstance/shared-cdc
  connectorClassRef: ConnectorClass/postgres-debezium
  tables: [public.orders]
  snapshot: never
`
}

func externalDatabaseManifest() string {
	return commonCDCPrelude() + `
---
apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata: {name: external-finance-credentials, namespace: education}
spec: {backend: env, keys: [username, password]}
---
apiVersion: databases.datascape.dev/v1alpha1
kind: DatabaseInstance
metadata: {name: external-finance-db, namespace: education}
spec:
  classRef: DatabaseClass/postgres
  ownership: external
  database: finance
  credentialsRef: SecretReference/external-finance-credentials
  verification:
    checks:
      - id: EXTERNAL-DB-ENDPOINT
        description: external database endpoint is reachable
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata: {name: external-finance, namespace: education}
spec:
  instanceRef: DatabaseInstance/external-finance-db
  connectorClassRef: ConnectorClass/postgres-debezium
  endpoint: {host: finance.db.example, port: 5432}
  database: finance
  credentialsRef: SecretReference/external-finance-credentials
---
apiVersion: sources.datascape.dev/v1alpha1
kind: RelationalSource
metadata: {name: finance, namespace: education}
spec: {connectionRef: DatabaseConnection/external-finance, tables: [public.payments]}
---
apiVersion: streams.datascape.dev/v1alpha1
kind: EventStream
metadata: {name: finance-events, namespace: education}
spec: {}
---
apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata: {name: finance-cdc, namespace: education}
spec:
  sourceRef: RelationalSource/finance
  streamRef: EventStream/finance-events
  cdcRef: CDCInstance/shared-cdc
  connectorClassRef: ConnectorClass/postgres-debezium
`
}

func externalCDCManifest(policy string) string {
	manifest := strings.Replace(externalDatabaseManifest(), `apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: shared-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  providerInstanceRef: ProviderInstance/default/local-cdc
  replicas: 1
  resources: {cpus: "1", memory: 1g, pidsLimit: 256}`, `apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: external-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  ownership: external
  providerInstanceRef: ProviderInstance/default/local-cdc
  managementPolicy: `+policy+`
  endpoint: {url: "https://connect.example"}
  verification:
    checks:
      - id: EXTERNAL-CDC-ENDPOINT
        description: external CDC endpoint is reachable`, 1)
	return strings.Replace(manifest, "cdcRef: CDCInstance/shared-cdc", "cdcRef: CDCInstance/external-cdc", 1)
}

func twoProviderCDCManifest() string {
	return strings.Replace(twoSourceSharedCDCManifest(), `apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: shared-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  providerInstanceRef: ProviderInstance/default/local-cdc
  replicas: 1
  resources: {cpus: "1", memory: 1g, pidsLimit: 256}`, `apiVersion: platform.datascape.dev/v1alpha1
kind: Provider
metadata: {name: cdc-provider-a, namespace: education}
spec: {type: datascape.dev/cdc, capabilities: [datascape.dev/source.change-stream], bindingKinds: [CDCBinding], targetCompatibility: [compose]}
---
apiVersion: platform.datascape.dev/v1alpha1
kind: Provider
metadata: {name: cdc-provider-b, namespace: education}
spec: {type: datascape.dev/cdc, capabilities: [datascape.dev/source.change-stream], bindingKinds: [CDCBinding], targetCompatibility: [compose]}
---
apiVersion: platform.datascape.dev/v1alpha1
kind: ProviderInstance
metadata: {name: cdc-a, namespace: education}
spec: {providerRef: Provider/cdc-provider-a, target: compose}
---
apiVersion: platform.datascape.dev/v1alpha1
kind: ProviderInstance
metadata: {name: cdc-b, namespace: education}
spec: {providerRef: Provider/cdc-provider-b, target: compose}
---
apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: shared-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  providerInstanceRef: ProviderInstance/cdc-a
  replicas: 1
---
apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: other-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  providerInstanceRef: ProviderInstance/cdc-b
  replicas: 1`, 1)
}

func productionImplicitCDCManifest() string {
	return strings.Replace(twoSourceSharedCDCManifest(), "  cdcRef: CDCInstance/shared-cdc\n", "", 1) + `
---
apiVersion: platform.datascape.dev/v1alpha1
kind: RuntimeProfile
metadata: {name: production}
spec:
  target: compose
  availability: {class: single-host-production}
  development: {enabled: false, allowUnpinnedImages: false}
`
}

func commonCDCPrelude() string {
	return `apiVersion: platform.datascape.dev/v1alpha1
kind: Target
metadata: {name: local}
spec: {type: compose}
---
apiVersion: databases.datascape.dev/v1alpha1
kind: DatabaseClass
metadata: {name: postgres}
spec:
  engine: postgresql
  providerInstanceRef: ProviderInstance/default/local-source
  supportedConnectorClasses: [postgres-debezium]
---
apiVersion: connections.datascape.dev/v1alpha1
kind: ConnectorClass
metadata: {name: postgres-debezium}
spec:
  interface: native
  transport: tcp
  driver: io.debezium.connector.postgresql.PostgresConnector
  operations: [snapshot, change-stream]
  compatibleEngines: [postgresql]
  targetCompatibility: [compose]
  configuration:
    defaults:
      plugin.name: pgoutput
---
apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCClass
metadata: {name: debezium-kafka-connect}
spec:
  engine: kafka-connect
  providerInstanceRef: ProviderInstance/default/local-cdc
  supportedConnectorClasses: [postgres-debezium]
  targetCompatibility: [compose]
  parameters:
    image: quay.io/debezium/connect:3.6.0.Final
---
apiVersion: cdc.datascape.dev/v1alpha1
kind: CDCInstance
metadata:
  name: shared-cdc
  namespace: education
spec:
  classRef: CDCClass/debezium-kafka-connect
  providerInstanceRef: ProviderInstance/default/local-cdc
  replicas: 1
  resources: {cpus: "1", memory: 1g, pidsLimit: 256}`
}

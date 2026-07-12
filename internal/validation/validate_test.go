package validation

import (
	"context"
	"testing"

	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/spec"
)

func TestValidateRejectsComposeProductionHA(t *testing.T) {
	resource := testResource("RuntimeProfile", "local", `{"target":"compose","availability":{"class":"production-ha"}}`)
	diags := ValidateResources(context.Background(), []spec.Resource{resource})
	if !hasCode(diags, "DVAL006") {
		t.Fatalf("expected DVAL006, got %#v", diags)
	}
}

func TestValidateRejectsInlineSecret(t *testing.T) {
	resource := testResource("Target", "demo", `{"databasePassword":"not-a-reference"}`)
	diags := ValidateResources(context.Background(), []spec.Resource{resource})
	if !hasCode(diags, "DVAL005") {
		t.Fatalf("expected DVAL005, got %#v", diags)
	}
}

func TestValidateDuplicateIdentity(t *testing.T) {
	resource := testResource("Target", "demo", `{}`)
	diags := ValidateResources(context.Background(), []spec.Resource{resource, resource})
	if !hasCode(diags, "DVAL001") {
		t.Fatalf("expected DVAL001, got %#v", diags)
	}
}

func TestValidateRejectsLegacyKind(t *testing.T) {
	resource := testResource("PostgresSource", "appdb", `{}`)
	diags := ValidateResources(context.Background(), []spec.Resource{resource})
	if !hasCode(diags, "DLEGACY001") {
		t.Fatalf("expected DLEGACY001, got %#v", diags)
	}
}

func TestValidateTypedBindingRequiresRefs(t *testing.T) {
	resources := parseTestResources(t, `apiVersion: bindings.datascape.dev/v1alpha1
kind: CDCBinding
metadata:
  name: appdb-cdc
spec:
  sourceRef: RelationalSource/appdb
`)
	diags := ValidateResources(context.Background(), resources)
	if !hasCode(diags, "DBIND014") {
		t.Fatalf("expected DBIND014, got %#v", diags)
	}
}

func TestValidateDatabaseConnectionRequiresCredentialsRef(t *testing.T) {
	resource := spec.Resource{APIVersion: "connections.datascape.dev/v1alpha1", Kind: "DatabaseConnection", Metadata: spec.Metadata{Name: "appdb"}, Spec: []byte(`{"engine":"postgres","host":"localhost","port":5432,"database":"app"}`)}
	diags := ValidateResources(context.Background(), []spec.Resource{resource})
	if !hasCode(diags, "DCONN003") {
		t.Fatalf("expected DCONN003, got %#v", diags)
	}
}

func TestValidateSecretReferenceRequiredKeys(t *testing.T) {
	resources := parseTestResources(t, `apiVersion: platform.datascape.dev/v1alpha1
kind: SecretReference
metadata:
  name: appdb-credentials
spec:
  backend: env
  keys: [username]
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata:
  name: appdb
spec:
  engine: postgres
  host: localhost
  port: 5432
  database: app
  credentialsRef: SecretReference/appdb-credentials
`)
	diags := ValidateResources(context.Background(), resources)
	if !hasCode(diags, "DCONN004") {
		t.Fatalf("expected DCONN004, got %#v", diags)
	}
}

func TestValidateExternalConnectionCanOmitCredentials(t *testing.T) {
	resource := spec.Resource{APIVersion: "connections.datascape.dev/v1alpha1", Kind: "DatabaseConnection", Metadata: spec.Metadata{Name: "appdb"}, Spec: []byte(`{"ownership":"external","engine":"postgres","database":"app"}`)}
	diags := ValidateResources(context.Background(), []spec.Resource{resource})
	if hasCode(diags, "DCONN003") {
		t.Fatalf("did not expect DCONN003 for external connection, got %#v", diags)
	}
}

func TestValidateResourceDefinitionSchema(t *testing.T) {
	def := testResource("ResourceDefinition", "custom-source", `{"apiVersion":"custom.datascape.dev/v1alpha1","kind":"CustomSource","schema":{"required":["endpoint"]}}`)
	custom := spec.Resource{APIVersion: "custom.datascape.dev/v1alpha1", Kind: "CustomSource", Metadata: spec.Metadata{Name: "upstream"}, Spec: []byte(`{}`)}
	diags := ValidateResources(context.Background(), []spec.Resource{def, custom})
	if !hasCode(diags, "DRES003") {
		t.Fatalf("expected DRES003, got %#v", diags)
	}
}

func TestValidateResourceDefinitionUsesJSONSchema2020(t *testing.T) {
	def := testResource("ResourceDefinition", "custom-source", `{"apiVersion":"custom.datascape.dev/v1alpha1","kind":"CustomSource","schema":{"type":"object","properties":{"mode":{"enum":["batch","stream"]}},"required":["mode"],"additionalProperties":false}}`)
	custom := spec.Resource{APIVersion: "custom.datascape.dev/v1alpha1", Kind: "CustomSource", Metadata: spec.Metadata{Name: "upstream"}, Spec: []byte(`{"mode":"invalid"}`)}
	diags := ValidateResources(context.Background(), []spec.Resource{def, custom})
	if !hasCode(diags, "DRES006") {
		t.Fatalf("expected DRES006, got %#v", diags)
	}
}

func TestValidateFileDatabaseConnectionRequiresClaimAndPath(t *testing.T) {
	resources := parseTestResources(t, `apiVersion: platform.datascape.dev/v1alpha1
kind: Provider
metadata: {name: database}
spec: {capabilities: [datascape.dev/database.provision]}
---
apiVersion: platform.datascape.dev/v1alpha1
kind: ProviderInstance
metadata: {name: database}
spec: {providerRef: Provider/database}
---
apiVersion: databases.datascape.dev/v1alpha1
kind: DatabaseClass
metadata: {name: sqlite}
spec: {engine: sqlite, providerInstanceRef: ProviderInstance/database}
---
apiVersion: connections.datascape.dev/v1alpha1
kind: ConnectorClass
metadata: {name: sqlite-odbc}
spec: {interface: odbc, transport: file, compatibleEngines: [sqlite]}
---
apiVersion: databases.datascape.dev/v1alpha1
kind: DatabaseInstance
metadata: {name: sqlite}
spec: {classRef: DatabaseClass/sqlite}
---
apiVersion: connections.datascape.dev/v1alpha1
kind: DatabaseConnection
metadata: {name: sqlite}
spec: {instanceRef: DatabaseInstance/sqlite, connectorClassRef: ConnectorClass/sqlite-odbc}
`)
	diags := ValidateResources(context.Background(), resources)
	if !hasCode(diags, "DCONN011") {
		t.Fatalf("expected DCONN011, got %#v", diags)
	}
}

func testResource(kind, name, body string) spec.Resource {
	return spec.Resource{
		APIVersion: spec.APIVersionV1Alpha1,
		Kind:       kind,
		Metadata:   spec.Metadata{Name: name},
		Spec:       []byte(body),
	}
}

func parseTestResources(t *testing.T, content string) []spec.Resource {
	t.Helper()
	resources, diags := spec.ParseDocuments(context.Background(), []spec.NamedDocument{{Name: "test.yaml", Content: []byte(content)}})
	if len(diags) > 0 {
		t.Fatalf("parse diagnostics: %#v", diags)
	}
	return resources
}

func hasCode(diags []domain.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

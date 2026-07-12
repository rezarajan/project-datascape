package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"datascape.dev/platformctl/internal/spec"
)

func TestDevelopmentSecretsCreatedWithoutPrintingValues(t *testing.T) {
	bundle := t.TempDir()
	if err := os.WriteFile(filepath.Join(bundle, ".env.example"), []byte("APP_USERNAME=change-me-local\nAPP_PASSWORD=change-me-local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSecrets(context.Background(), []string{"init", "--bundle", bundle, "--development"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(bundle, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected mode 0600, got %o", info.Mode().Perm())
	}
	content, err := os.ReadFile(filepath.Join(bundle, ".env"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "change-me-local") || !strings.Contains(string(content), "APP_USERNAME=datascape") {
		t.Fatalf("unexpected development secret file: %s", content)
	}
	if err := cmdSecrets(context.Background(), []string{"init", "--bundle", bundle, "--development"}); err == nil {
		t.Fatal("expected overwrite protection")
	}
	if err := os.Chmod(filepath.Join(bundle, ".env"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdSecrets(context.Background(), []string{"init", "--bundle", bundle, "--development", "--force"}); err != nil {
		t.Fatal(err)
	}
	info, err = os.Stat(filepath.Join(bundle, ".env"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("forced replacement must restore mode 0600: info=%v err=%v", info, err)
	}
}

func TestComposeRuntimeSummaryRejectsUnhealthyServices(t *testing.T) {
	services, err := parseComposePS([]byte(`[{"Service":"api","State":"running","Health":"healthy"},{"Service":"migrate","State":"exited","ExitCode":0}]`))
	if err != nil {
		t.Fatal(err)
	}
	if summary, err := summarizeComposePS(services); err != nil || !strings.Contains(summary, "1 running services") {
		t.Fatalf("unexpected runtime summary %q: %v", summary, err)
	}
	if _, err := summarizeComposePS([]composePS{{Service: "api", State: "running", Health: "unhealthy"}}); err == nil {
		t.Fatal("expected unhealthy service failure")
	}
	if _, err := summarizeComposePS([]composePS{{Service: "migrate", State: "exited", ExitCode: 2}}); err == nil {
		t.Fatal("expected failed completion job failure")
	}
}

func TestMigrateGenericBindingToTypedBinding(t *testing.T) {
	res := spec.Resource{
		APIVersion: spec.APIVersionV1Alpha1,
		Kind:       "Binding",
		Metadata:   spec.Metadata{Name: "appdb-cdc", Namespace: "education"},
		Spec:       []byte(`{"capability":"datascape.dev/source.change-stream","sourceRef":"RelationalSource/appdb","targetRef":"EventStream/app-changes"}`),
	}
	items := migrateResource(res)
	if len(items) != 1 {
		t.Fatalf("expected one migrated resource, got %d", len(items))
	}
	if items[0]["apiVersion"] != "bindings.datascape.dev/v1alpha1" || items[0]["kind"] != "CDCBinding" {
		t.Fatalf("expected CDCBinding, got %#v", items[0])
	}
	body, _ := items[0]["spec"].(map[string]any)
	if body["streamRef"] != "EventStream/app-changes" {
		t.Fatalf("expected streamRef migration, got %#v", body)
	}
}

func TestMigrateOlderDatabaseSourceAddsConnection(t *testing.T) {
	res := spec.Resource{
		APIVersion: spec.APIVersionV1Alpha1,
		Kind:       "PostgresSource",
		Metadata:   spec.Metadata{Name: "appdb", Namespace: "education"},
		Spec:       []byte(`{"database":"app","streamRef":"EventStream/app-changes"}`),
	}
	items := migrateResource(res)
	if len(items) != 3 {
		t.Fatalf("expected connection, source, and typed binding, got %#v", items)
	}
	if items[0]["kind"] != "DatabaseConnection" || items[1]["kind"] != "RelationalSource" || items[2]["kind"] != "CDCBinding" {
		t.Fatalf("unexpected migration output: %#v", items)
	}
	sourceSpec, _ := items[1]["spec"].(map[string]any)
	if sourceSpec["connectionRef"] != "DatabaseConnection/appdb-db" {
		t.Fatalf("expected source connectionRef, got %#v", sourceSpec)
	}
}

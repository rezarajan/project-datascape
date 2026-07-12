package spec

import (
	"context"
	"testing"
)

func TestParseDocumentsArray(t *testing.T) {
	resources, diags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`[
		  {"apiVersion":"platform.datascape.dev/v1alpha1","kind":"DataPlatform","metadata":{"name":"demo"},"spec":{}},
		  {"apiVersion":"platform.datascape.dev/v1alpha1","kind":"RuntimeProfile","metadata":{"name":"local"},"spec":{"target":"compose"}}
		]`),
	}})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	if resources[0].Kind != "DataPlatform" {
		t.Fatalf("resources should be sorted by identity, got %s", resources[0].Kind)
	}
}

func TestParseNativeYAML(t *testing.T) {
	resources, diags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: DataPlatform
metadata:
  name: demo
spec:
  description: native YAML
`),
	}})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(resources) != 1 || resources[0].Metadata.Name != "demo" {
		t.Fatalf("unexpected resources: %#v", resources)
	}
	if resources[0].Location.Line == 0 || resources[0].Location.Column == 0 {
		t.Fatalf("expected line/column location, got %#v", resources[0].Location)
	}
}

func TestParseMultiDocumentYAML(t *testing.T) {
	resources, diags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: DataPlatform
metadata:
  name: demo
---
apiVersion: platform.datascape.dev/v1alpha1
kind: RuntimeProfile
metadata:
  name: local
spec:
  target: compose
`),
	}})
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(resources))
	}
	if resources[1].Location.Document != 1 {
		t.Fatalf("expected second document index, got %#v", resources[1].Location)
	}
}

func TestParseRejectsDuplicateKeys(t *testing.T) {
	_, diags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: DataPlatform
kind: RuntimeProfile
metadata:
  name: demo
`),
	}})
	if len(diags) == 0 || diags[0].Code != "DSPEC018" {
		t.Fatalf("expected DSPEC018, got %#v", diags)
	}
}

func TestParseRejectsUnknownTopLevelFields(t *testing.T) {
	_, diags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: DataPlatform
metadata:
  name: demo
unexpected: true
`),
	}})
	if len(diags) == 0 || diags[0].Code != "DSPEC015" {
		t.Fatalf("expected DSPEC015, got %#v", diags)
	}
}

func TestParseYAMLAndJSONEquivalent(t *testing.T) {
	yamlResources, yamlDiags := ParseDocuments(context.Background(), []NamedDocument{{
		Name: "platform.yaml",
		Content: []byte(`apiVersion: platform.datascape.dev/v1alpha1
kind: DataPlatform
metadata:
  name: demo
spec:
  description: equivalent
`),
	}})
	jsonResources, jsonDiags := ParseDocuments(context.Background(), []NamedDocument{{
		Name:    "platform.json",
		Content: []byte(`{"apiVersion":"platform.datascape.dev/v1alpha1","kind":"DataPlatform","metadata":{"name":"demo"},"spec":{"description":"equivalent"}}`),
	}})
	if len(yamlDiags) != 0 || len(jsonDiags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v %#v", yamlDiags, jsonDiags)
	}
	if yamlResources[0].Identity("", "").CanonicalString() != jsonResources[0].Identity("", "").CanonicalString() {
		t.Fatalf("identities differ")
	}
	if string(yamlResources[0].Spec) != string(jsonResources[0].Spec) {
		t.Fatalf("spec differs: %s != %s", yamlResources[0].Spec, jsonResources[0].Spec)
	}
}

func FuzzValidLogicalName(f *testing.F) {
	f.Add("student-attendance")
	f.Add("Student_Attendance")
	f.Fuzz(func(t *testing.T, name string) {
		_ = ValidLogicalName(name)
	})
}

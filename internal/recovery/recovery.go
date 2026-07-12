package recovery

type Step struct {
	Order       int      `json:"order"`
	Name        string   `json:"name"`
	Requires    []string `json:"requires,omitempty"`
	Description string   `json:"description"`
}

func FoundationPlan() []Step {
	return []Step{
		{Order: 1, Name: "validate-source-manifests", Requires: []string{"git", "resource-definitions", "providers", "bindings"}, Description: "Verify authoritative declarative inputs before runtime reconstruction."},
		{Order: 2, Name: "recreate-provider-resources", Requires: []string{"provider-instances", "runtime-profile"}, Description: "Regenerate provider-owned target resources from deterministic specifications."},
		{Order: 3, Name: "rehydrate-derived-state", Requires: []string{"bindings", "schemas", "policies"}, Description: "Rebuild derived data products, metadata, lineage views, dashboards, and validation reports."},
	}
}

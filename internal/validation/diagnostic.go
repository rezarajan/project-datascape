package validation

import (
	"sort"

	"datascape.dev/platformctl/internal/domain"
)

func SortDiagnostics(diags []domain.Diagnostic) {
	sort.SliceStable(diags, func(i, j int) bool {
		if diags[i].Severity != diags[j].Severity {
			return diags[i].Severity < diags[j].Severity
		}
		if diags[i].Code != diags[j].Code {
			return diags[i].Code < diags[j].Code
		}
		if diags[i].Resource != diags[j].Resource {
			return diags[i].Resource < diags[j].Resource
		}
		return diags[i].FieldPath < diags[j].FieldPath
	})
}

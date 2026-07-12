package diff

import (
	"datascape.dev/platformctl/internal/ir"
)

type Change struct {
	Operation string `json:"operation"`
	Identity  string `json:"identity"`
	Before    string `json:"before,omitempty"`
	After     string `json:"after,omitempty"`
}

func Plans(before, after ir.PlatformPlan) []Change {
	beforeByID := map[string]ir.ResourcePlan{}
	afterByID := map[string]ir.ResourcePlan{}
	for _, resource := range before.Resources {
		beforeByID[resource.Identity.CanonicalString()] = resource
	}
	for _, resource := range after.Resources {
		afterByID[resource.Identity.CanonicalString()] = resource
	}
	changes := make([]Change, 0)
	for id, beforeResource := range beforeByID {
		afterResource, ok := afterByID[id]
		if !ok {
			changes = append(changes, Change{Operation: "delete", Identity: id, Before: beforeResource.CanonicalDigest})
			continue
		}
		if beforeResource.CanonicalDigest != afterResource.CanonicalDigest {
			changes = append(changes, Change{Operation: "change", Identity: id, Before: beforeResource.CanonicalDigest, After: afterResource.CanonicalDigest})
		}
	}
	for id, afterResource := range afterByID {
		if _, ok := beforeByID[id]; !ok {
			changes = append(changes, Change{Operation: "add", Identity: id, After: afterResource.CanonicalDigest})
		}
	}
	return changes
}

package planner

import (
	"fmt"

	"datascape.dev/platformctl/internal/ir"
)

func AddPlan(plan ir.PlatformPlan) []ir.ChangeAction {
	actions := make([]ir.ChangeAction, 0, len(plan.Resources))
	for _, resource := range plan.Resources {
		actions = append(actions, ir.ChangeAction{
			Operation: "add",
			Identity:  resource.Identity,
			Message:   fmt.Sprintf("+ Add %s %s", resource.Kind, resource.Identity.Display()),
		})
	}
	return actions
}

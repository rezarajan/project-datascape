package compiler

import (
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/spec"
)

func buildOperationPlans(resources []spec.Resource, cdc ir.CDCPlan, target string) ([]ir.OperationPlan, []domain.Diagnostic) {
	plans := make([]ir.OperationPlan, 0)
	diags := make([]domain.Diagnostic, 0)
	byID := resourceIndex(resources, target)
	seen := map[string]ir.OperationPlan{}
	for _, res := range resources {
		if res.Kind != "CDCOperation" {
			continue
		}
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		action := stringValue(body["action"])
		targetRef := parseCDCRef(stringValue(body["targetRef"]), res, "", target)
		providerInstance := parseCDCRef(stringValue(body["providerInstanceRef"]), res, "ProviderInstance", target)
		if providerInstance.Name == "" {
			providerInstance = operationProviderInstance(targetRef, byID, cdc)
		}
		idempotencyKey := stringValue(body["idempotencyKey"])
		plan := ir.OperationPlan{
			Identity:             res.Identity(target, ""),
			Target:               targetRef,
			Action:               action,
			IdempotencyKey:       idempotencyKey,
			ProviderInstance:     providerInstance,
			Parameters:           cloneMap(anyMap(body["parameters"])),
			State:                "planned",
			MutatesExternalState: operationMutatesPlan(action),
			Destructive:          operationDestructivePlan(action),
			ApprovalRequired:     operationDestructivePlan(action) || approvalRequired(body),
			Approved:             approvalApproved(body),
			Timeout:              stringValue(body["timeout"]),
			RetryPolicy:          cloneMap(anyMap(body["retryPolicy"])),
			Preconditions:        stringSlice(body["preconditions"]),
			Verification:         operationVerification(body, action),
			Recovery:             operationRecovery(action),
			TargetCompatibility:  stringSlice(body["targetCompatibility"]),
			Ownership:            stringValue(body["ownership"]),
			Team:                 stringValue(body["team"]),
			Classification:       operationClassification(action),
			DependsOn:            []domain.ResourceIdentity{targetRef},
		}
		if prior, ok := seen[idempotencyKey]; ok && idempotencyKey != "" {
			if prior.Target.CanonicalString() == plan.Target.CanonicalString() && prior.Action == plan.Action {
				continue
			}
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DOPS011", Resource: res.Identity(target, "").Display(), FieldPath: "spec.idempotencyKey", Message: "idempotency key collides with a different operation plan", Remediation: "use a distinct key for different operation targets or actions"})
		}
		if idempotencyKey != "" {
			seen[idempotencyKey] = plan
		}
		plans = append(plans, plan)
	}
	sort.SliceStable(plans, func(i, j int) bool {
		if plans[i].IdempotencyKey != plans[j].IdempotencyKey {
			return plans[i].IdempotencyKey < plans[j].IdempotencyKey
		}
		return plans[i].Identity.CanonicalString() < plans[j].Identity.CanonicalString()
	})
	return plans, diags
}

func operationProviderInstance(target domain.ResourceIdentity, byID map[string]spec.Resource, cdc ir.CDCPlan) domain.ResourceIdentity {
	instanceID := domain.ResourceIdentity{}
	if target.Kind == "CDCInstance" {
		instanceID = target
	}
	if target.Kind == "CDCBinding" {
		if binding, ok := byID[target.CanonicalString()]; ok {
			body, _ := resourceBody(binding)
			instanceID = parseCDCRef(stringValue(body["cdcRef"]), binding, "CDCInstance", target.Target)
		}
	}
	for _, instance := range cdc.Instances {
		if instance.Identity.CanonicalString() == instanceID.CanonicalString() {
			return instance.ProviderInstance
		}
	}
	return domain.ResourceIdentity{}
}

func approvalRequired(body map[string]any) bool {
	approval := anyMap(body["approval"])
	return boolValue(approval["required"], false)
}

func approvalApproved(body map[string]any) bool {
	approval := anyMap(body["approval"])
	return boolValue(approval["approved"], false)
}

func operationVerification(body map[string]any, action string) []ir.VerificationCheck {
	checks := verificationChecks(body["verification"])
	if len(checks) > 0 {
		return checks
	}
	return []ir.VerificationCheck{{ID: "OPERATION-" + strings.ToUpper(sanitizeName(action)), Description: "operation " + action + " verification checks complete"}}
}

func operationRecovery(action string) []ir.RecoveryStep {
	switch action {
	case "ResetOffsets":
		return []ir.RecoveryStep{
			{Order: 1, Name: "export-offsets", Description: "Export connector offsets before reset."},
			{Order: 2, Name: "restore-offsets", Requires: []string{"export-offsets"}, Description: "Import exported offsets if reset verification fails."},
		}
	case "MoveConnector":
		return []ir.RecoveryStep{
			{Order: 1, Name: "capture-source-offsets", Description: "Capture source offsets, topic routing, schema history, and duplicate-event risk before move."},
			{Order: 2, Name: "rollback-connector-placement", Requires: []string{"capture-source-offsets"}, Description: "Return connector to the prior CDCInstance if migration checks fail."},
		}
	default:
		return []ir.RecoveryStep{{Order: 1, Name: "verify-no-unexpected-mutation", Description: "Verify target resource state after operation planning or execution."}}
	}
}

func operationMutatesPlan(action string) bool {
	switch action {
	case "Inspect", "InspectOffsets", "VerifyConnector", "ValidateConnectivity", "PlanBackup", "PlanRestore":
		return false
	default:
		return action != ""
	}
}

func operationDestructivePlan(action string) bool {
	switch action {
	case "ResetOffsets", "DeleteConnector", "DeleteCDCInstance", "Delete", "DetachAndDelete":
		return true
	default:
		return false
	}
}

func operationClassification(action string) string {
	switch action {
	case "UpdateConnectorConfig", "ChangeTableFilters", "RotateDatabaseCredentials", "RotateStreamCredentials":
		return "connector-restart-required"
	case "ScaleWorker", "UpgradeWorker", "RestartWorker":
		return "worker-restart-required"
	case "MoveConnector":
		return "replacement-required"
	case "ResetOffsets", "DeleteConnector", "DeleteCDCInstance", "Delete":
		return "destructive"
	case "Inspect", "InspectOffsets", "VerifyConnector", "ValidateConnectivity":
		return "no-op"
	default:
		if action == "" {
			return ""
		}
		return "in-place"
	}
}

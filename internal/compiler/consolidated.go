package compiler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/binding"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/provider"
	"datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
)

func buildResourceGraph(resources []spec.Resource, target string, definitions *resource.Registry, bindings []binding.Resolved) (ir.ResourceGraphPlan, []domain.Diagnostic) {
	policy := effectivePolicy(resources, target)
	nodes := make([]ir.ResourceGraphNode, 0, len(resources))
	external := make([]ir.ExternalResourcePlan, 0)
	overrides := make([]ir.OverridePlan, 0)
	diags := make([]domain.Diagnostic, 0)
	for _, res := range resources {
		body, _ := resourceBody(res)
		ownership := resourceOwnership(res, body)
		lifecycle := resourceLifecycle(body)
		state := graphState(ownership, lifecycle)
		capabilities := resourceCapabilities(res, definitions)
		node := ir.ResourceGraphNode{
			Identity:     res.Identity(target, ""),
			Kind:         res.Kind,
			Ownership:    ownership,
			Lifecycle:    lifecycle,
			State:        state,
			Capabilities: capabilities,
			Verification: verificationChecks(body["verification"]),
		}
		nodes = append(nodes, node)
		if ownership == "external" || ownership == "imported" {
			ext := ir.ExternalResourcePlan{
				Identity:     res.Identity(target, ""),
				Kind:         res.Kind,
				Capability:   firstCapability(capabilities, stringValue(body["capability"])),
				Interface:    stringValue(body["interface"]),
				TrustPolicy:  trustPolicy(body["trustPolicy"]),
				State:        state,
				Verification: verificationChecks(body["verification"]),
			}
			external = append(external, ext)
			extOverrides, extDiags := validateExternalResource(res, ext, policy)
			overrides = append(overrides, extOverrides...)
			diags = append(diags, extDiags...)
		}
		explicitOverrides, overrideDiags := overrideDeclarations(res, body, policy)
		overrides = append(overrides, explicitOverrides...)
		diags = append(diags, overrideDiags...)
	}
	bindingPlans := bindingPlans(bindings)
	sort.SliceStable(nodes, func(i, j int) bool { return nodes[i].Identity.CanonicalString() < nodes[j].Identity.CanonicalString() })
	sort.SliceStable(external, func(i, j int) bool {
		return external[i].Identity.CanonicalString() < external[j].Identity.CanonicalString()
	})
	sortOverrides(overrides)
	diags = append(diags, validatePolicyRequirements(policy, bindingPlans)...)
	policies := []ir.PolicyPlan{policy}
	if policy.Identity.Name == "" {
		policies = nil
	}
	return ir.ResourceGraphPlan{
		ValidationMode: policy.ValidationMode,
		Nodes:          nodes,
		Bindings:       bindingPlans,
		External:       external,
		Policies:       policies,
		Overrides:      overrides,
	}, diags
}

func planIdentity(resources []spec.Resource, target string) domain.ResourceIdentity {
	for _, kind := range []string{"Target", "RuntimeProfile"} {
		for _, res := range resources {
			if res.APIVersion == api.PlatformV1Alpha1 && res.Kind == kind {
				return res.Identity(target, "")
			}
		}
	}
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "Target", Namespace: api.DefaultNamespace, Name: target, Target: target}
}

func buildTargetPlan(resources []spec.Resource, target string) ir.TargetPlan {
	plan := ir.TargetPlan{Type: target}
	if plan.Type == "" || plan.Type == "local" {
		plan.Type = "compose"
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || (res.Kind != "RuntimeProfile" && res.Kind != "Target") {
			continue
		}
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		if res.Kind == "RuntimeProfile" {
			plan.Profile = res.Metadata.Name
		}
		if value := stringValue(body["target"]); value != "" {
			plan.Type = value
		}
		if value := stringValue(body["type"]); value != "" {
			plan.Type = value
		}
		if availability, ok := body["availability"].(map[string]any); ok {
			plan.AvailabilityClass = stringValue(availability["class"])
		}
		if development, ok := body["development"].(map[string]any); ok {
			plan.DevelopmentMode = boolValue(development["enabled"], false)
			plan.AllowUnpinnedImages = boolValue(development["allowUnpinnedImages"], false)
		}
	}
	return plan
}

func definitionPlans(registry *resource.Registry) []ir.DefinitionPlan {
	defs := registry.Definitions()
	out := make([]ir.DefinitionPlan, 0, len(defs))
	for _, def := range defs {
		out = append(out, ir.DefinitionPlan{
			Identity:     def.Identity,
			APIVersion:   def.APIVersion,
			Kind:         def.Kind,
			Scope:        def.Scope,
			Category:     def.Category,
			ProviderType: def.ProviderType,
			Capabilities: append([]string{}, def.Capabilities...),
			BindingRoles: append([]string{}, def.BindingRoles...),
			Core:         def.Core,
			Extension:    def.Extension,
		})
	}
	return out
}

func providerPlans(registry *provider.Registry) []ir.ProviderPlan {
	descriptors := registry.Descriptors()
	out := make([]ir.ProviderPlan, 0, len(descriptors))
	for _, descriptor := range descriptors {
		out = append(out, ir.ProviderPlan{
			Identity:            descriptor.Identity,
			Type:                descriptor.Type,
			Capabilities:        append([]string{}, descriptor.Capabilities...),
			BindingKinds:        append([]string{}, descriptor.BindingKinds...),
			TargetCompatibility: append([]string{}, descriptor.TargetCompatibility...),
			RuntimeDependencies: append([]string{}, descriptor.RuntimeDependencies...),
			Services:            servicePlans(descriptor.Services),
			Artifacts:           artifactPlans(descriptor.Artifacts),
			RendererContract:    descriptor.RendererContract,
			Conformance:         append([]string{}, descriptor.Conformance...),
			PackageVersion:      descriptor.PackageVersion,
			ContractVersion:     descriptor.ContractVersion,
			PackageDigest:       descriptor.PackageDigest,
			Provenance:          descriptor.Provenance,
		})
	}
	return out
}

func providerInstancePlans(registry *provider.Registry) []ir.ProviderInstancePlan {
	instances := registry.Instances()
	out := make([]ir.ProviderInstancePlan, 0, len(instances))
	for _, instance := range instances {
		out = append(out, ir.ProviderInstancePlan{
			Identity:     instance.Identity,
			Provider:     instance.Provider,
			Type:         instance.Type,
			Target:       instance.Target,
			Capabilities: append([]string{}, instance.Capabilities...),
			Parameters:   cloneMap(instance.Parameters),
		})
	}
	return out
}

func plannedProviderResources(resources []spec.Resource, bindings []binding.Resolved, definitions *resource.Registry, providers *provider.Registry, target string) ([]ir.ProviderResourcePlan, []domain.Diagnostic) {
	required := map[string]string{}
	requested := map[string]domain.ResourceIdentity{}
	for _, res := range resources {
		body, _ := resourceBody(res)
		if graphState(resourceOwnership(res, body), resourceLifecycle(body)) != "satisfied" {
			continue
		}
		for _, capability := range resourceCapabilities(res, definitions) {
			required[capability] = res.Identity(target, "").Display()
			if ref := stringValue(body["providerInstanceRef"]); ref != "" {
				requested[capability] = providerInstanceIdentity(ref, res, target)
			}
		}
	}
	for _, b := range bindings {
		if b.State == "disabled" || b.State == "deferred" {
			continue
		}
		required[b.Capability] = b.Identity.Display()
		if b.ProviderInstance.Name != "" {
			requested[b.Capability] = b.ProviderInstance
		}
		for _, capability := range b.DependencyClosure {
			required[capability] = b.Identity.Display()
		}
	}
	if target == "compose" && len(required) > 0 {
		required["datascape.dev/runtime.utility"] = "compose target runtime tasks"
	}
	capabilities := make([]string, 0, len(required))
	for capability := range required {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	plans := make([]ir.ProviderResourcePlan, 0, len(capabilities))
	diags := make([]domain.Diagnostic, 0)
	for _, capability := range capabilities {
		instance, descriptor, ok := provider.Instance{}, provider.Descriptor{}, false
		if requestedInstance := requested[capability]; requestedInstance.Name != "" {
			instance, descriptor, ok = providers.Instance(requestedInstance)
			if ok && !containsValue(instance.Capabilities, capability) {
				ok = false
			}
		} else {
			instance, descriptor, ok = providers.ResolveCapability(capability, target)
		}
		if !ok {
			diags = append(diags, domain.Diagnostic{
				Severity:    domain.SeverityError,
				Code:        "DPROV008",
				FieldPath:   "spec.capability",
				Message:     "no provider instance can satisfy required capability " + capability,
				Remediation: "declare a Provider and ProviderInstance for the required capability",
			})
			continue
		}
		plans = append(plans, ir.ProviderResourcePlan{
			Identity: domain.ResourceIdentity{
				APIVersion: api.PlatformV1Alpha1,
				Kind:       "ProviderResource",
				Namespace:  api.DefaultNamespace,
				Name:       sanitizeName(capability),
				Target:     target,
				Adapter:    descriptor.Identity.Name,
			},
			Capability:       capability,
			ProviderInstance: instance.Identity,
			Provider:         descriptor.Identity,
			Reason:           required[capability],
			Services:         servicesForCapability(descriptor.Services, capability),
			Artifacts:        artifactsForCapability(descriptor.Artifacts, capability),
		})
	}
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Identity.CanonicalString() < plans[j].Identity.CanonicalString() })
	return plans, diags
}

func providerInstanceIdentity(value string, owner spec.Resource, target string) domain.ResourceIdentity {
	parts := strings.Split(value, "/")
	ns, name := defaultNamespace(owner.Metadata.Namespace), ""
	switch len(parts) {
	case 2:
		name = parts[1]
	case 3:
		ns, name = parts[1], parts[2]
	case 5:
		ns, name = parts[3], parts[4]
	}
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "ProviderInstance", Namespace: ns, Name: name, Target: target}
}

func bindingPlans(bindings []binding.Resolved) []ir.BindingPlan {
	out := make([]ir.BindingPlan, 0, len(bindings))
	for _, b := range bindings {
		out = append(out, ir.BindingPlan{
			Identity:          b.Identity,
			Kind:              b.Kind,
			Definition:        b.Definition,
			Capability:        b.Capability,
			Source:            b.Source,
			Target:            b.Target,
			ProviderInstance:  b.ProviderInstance,
			Mode:              b.Mode,
			Ownership:         b.Ownership,
			State:             b.State,
			Dependencies:      b.Dependencies,
			DependencyClosure: b.DependencyClosure,
			Digest:            b.Digest,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identity.CanonicalString() < out[j].Identity.CanonicalString() })
	return out
}

func buildVerification(resources []spec.Resource, graph ir.ResourceGraphPlan, planned []ir.ProviderResourcePlan, target string) ir.VerificationPlan {
	var policyRef domain.ResourceIdentity
	for _, res := range resources {
		if res.APIVersion == api.PlatformV1Alpha1 && res.Kind == "PlatformPolicy" {
			policyRef = res.Identity(target, "")
			break
		}
	}
	checks := []ir.VerificationCheck{{ID: "RECOVERY-001", Description: "recovery graph is present and deterministic"}}
	if target == "compose" {
		checks = append(checks, ir.VerificationCheck{ID: "COMPOSE-001", Description: "compose artifact is present and parseable"})
	}
	if len(graph.Bindings) > 0 {
		checks = append(checks, ir.VerificationCheck{ID: "BINDING-001", Description: "declared bindings resolve to source, target, and provider selections"})
	}
	capabilities := map[string]bool{}
	for _, item := range planned {
		capabilities[item.Capability] = true
	}
	if capabilities["datascape.dev/source.relational"] {
		checks = append(checks, ir.VerificationCheck{ID: "SOURCE-001", Description: "managed source dependency is reachable"})
	}
	if capabilities["datascape.dev/source.change-stream"] {
		checks = append(checks, ir.VerificationCheck{ID: "CDC-001", Description: "change-stream provider can publish a source change"})
	}
	if capabilities["datascape.dev/stream.publish"] {
		checks = append(checks, ir.VerificationCheck{ID: "EVENTSTREAM-001", Description: "event stream provider can publish and consume a test event"})
	}
	if capabilities["datascape.dev/store.object"] {
		checks = append(checks, ir.VerificationCheck{ID: "ARCHIVE-001", Description: "object-store provider can persist an immutable object"})
	}
	if capabilities["datascape.dev/lineage.admit"] {
		checks = append(checks, ir.VerificationCheck{ID: "LINEAGE-001", Description: "lineage provider can admit a valid lineage event"})
	}
	for _, ext := range graph.External {
		checks = append(checks, ext.Verification...)
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return ir.VerificationPlan{PolicyRef: policyRef, Checks: checks}
}

func buildRecoveryPlan(graph ir.ResourceGraphPlan, planned []ir.ProviderResourcePlan, storage ir.StoragePlan) ir.RecoveryPlan {
	steps := []ir.RecoveryStep{{Order: 1, Name: "recompile-bundle", Requires: []string{"source-manifests"}, Description: "Regenerate target artifacts from canonical resources, definitions, providers, and bindings."}}
	order := 2
	if len(graph.Bindings) > 0 {
		steps = append(steps, ir.RecoveryStep{Order: order, Name: "restore-binding-state", Requires: []string{"configuration/bindings"}, Description: "Replay binding declarations and provider selections."})
		order++
	}
	if len(storage.Volumes) > 0 {
		steps = append(steps, ir.RecoveryStep{Order: order, Name: "restore-persistent-volumes", Requires: []string{"storage/plan.json", "off-host-backup"}, Description: "Restore retained or imported volume data before starting dependent services."})
		order++
	}
	for _, capability := range plannedCapabilities(planned) {
		steps = append(steps, ir.RecoveryStep{Order: order, Name: "restore-" + sanitizeName(capability), Requires: []string{"provider:" + capability}, Description: "Restore provider-owned runtime resources for " + capability + "."})
		order++
	}
	return ir.RecoveryPlan{Steps: steps}
}

func effectivePolicy(resources []spec.Resource, target string) ir.PolicyPlan {
	policy := ir.PolicyPlan{
		ValidationMode:              "strict",
		AllowExternalOwnership:      true,
		AllowExternalTrustOverrides: false,
		AllowDeferrals:              false,
		AllowOverrides:              false,
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "PlatformPolicy" {
			continue
		}
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		policy.Identity = res.Identity(target, "")
		policy.ValidationMode = stringDefault(firstString(body, "validationMode", "mode"), policy.ValidationMode)
		policy.AllowExternalOwnership = boolValue(body["allowExternalOwnership"], policy.AllowExternalOwnership)
		policy.AllowDeferrals = boolValue(body["allowDeferrals"], policy.AllowDeferrals)
		policy.AllowExternalTrustOverrides = boolValue(body["allowExternalTrustOverrides"], policy.AllowExternalTrustOverrides)
		policy.AllowOverrides = boolValue(body["allowOverrides"], policy.AllowOverrides)
		if requirements, ok := body["requirements"].(map[string]any); ok {
			policy.RequireLineage = boolValue(requirements["lineage"], false)
			policy.RequireAudit = boolValue(requirements["audit"], false)
			policy.RequireIdempotency = boolValue(requirements["idempotency"], false)
		}
		break
	}
	if policy.ValidationMode == "permissive" {
		policy.AllowDeferrals = true
	}
	return policy
}

func validatePolicyRequirements(policy ir.PolicyPlan, bindings []ir.BindingPlan) []domain.Diagnostic {
	if policy.Identity.Name == "" {
		return nil
	}
	hasLineage := false
	hasAudit := false
	for _, binding := range bindings {
		if binding.State == "disabled" || binding.State == "deferred" {
			continue
		}
		if binding.Capability == "datascape.dev/lineage.admit" {
			hasLineage = true
		}
		if binding.Capability == "datascape.dev/audit.record" {
			hasAudit = true
		}
	}
	diags := make([]domain.Diagnostic, 0)
	if policy.RequireLineage && !hasLineage {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DGRAPH007", Resource: policy.Identity.Display(), FieldPath: "spec.requirements.lineage", Message: "selected policy requires lineage but no lineage binding is declared", Remediation: "add a Binding with capability datascape.dev/lineage.admit or remove the lineage requirement"})
	}
	if policy.RequireAudit && !hasAudit {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DGRAPH008", Resource: policy.Identity.Display(), FieldPath: "spec.requirements.audit", Message: "selected policy requires audit but no audit binding is declared", Remediation: "add a Binding with capability datascape.dev/audit.record or remove the audit requirement"})
	}
	return diags
}

func validateExternalResource(res spec.Resource, ext ir.ExternalResourcePlan, policy ir.PolicyPlan) ([]ir.OverridePlan, []domain.Diagnostic) {
	if ext.State == "disabled" {
		return nil, nil
	}
	overrides := make([]ir.OverridePlan, 0)
	diags := make([]domain.Diagnostic, 0)
	if !policy.AllowExternalOwnership {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DGRAPH009", Resource: ext.Identity.Display(), FieldPath: "spec.ownership", Message: "selected policy rejects externally owned resources", Remediation: "change ownership or allow external ownership in PlatformPolicy", Location: res.Location})
	}
	if len(ext.Verification) > 0 {
		return overrides, diags
	}
	if ext.TrustPolicy != "" && policy.AllowExternalTrustOverrides {
		override := ir.OverridePlan{Name: ext.Identity.Name + "-external-trust", Scope: "external-resource-verification", Resource: ext.Identity, Policy: ext.TrustPolicy, Reason: "external resource is accepted by declared trust policy without runtime verification", Remediation: "add runtime verification checks before production promotion"}
		overrides = append(overrides, override)
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityWarning, Code: "DOVR004", Resource: ext.Identity.Display(), FieldPath: "spec.trustPolicy", Message: "external trust override accepted without runtime verification", Remediation: override.Remediation, Location: res.Location})
		return overrides, diags
	}
	diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DGRAPH003", Resource: ext.Identity.Display(), FieldPath: "spec.verification", Message: "external resources must declare verification checks", Remediation: "add spec.verification.checks or permit an explicit external-trust override in PlatformPolicy", Location: res.Location})
	return overrides, diags
}

func overrideDeclarations(res spec.Resource, body map[string]any, policy ir.PolicyPlan) ([]ir.OverridePlan, []domain.Diagnostic) {
	values, ok := body["overrides"].([]any)
	if !ok {
		if item, ok := body["overrides"].(map[string]any); ok {
			values = []any{item}
		}
	}
	if len(values) == 0 {
		return nil, nil
	}
	overrides := make([]ir.OverridePlan, 0, len(values))
	diags := make([]domain.Diagnostic, 0)
	for i, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			diags = append(diags, overrideDiag(res, i, "override declaration must be an object", "declare name, scope, reason, and remediation"))
			continue
		}
		override := ir.OverridePlan{Name: stringValue(item["name"]), Scope: stringValue(item["scope"]), Resource: res.Identity("", ""), Policy: stringValue(item["policy"]), Reason: stringValue(item["reason"]), Remediation: stringValue(item["remediation"])}
		if override.Name == "" || override.Scope == "" || override.Remediation == "" {
			diags = append(diags, overrideDiag(res, i, "override must be named, scoped, and include remediation text", "set name, scope, and remediation for the override"))
			continue
		}
		if policy.ValidationMode == "strict" && !policy.AllowOverrides {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DOVR003", Resource: res.Identity("", "").Display(), FieldPath: fmt.Sprintf("spec.overrides[%d]", i), Message: "strict policy does not allow production overrides", Remediation: "allow overrides in PlatformPolicy or remove the override", Location: res.Location})
		}
		overrides = append(overrides, override)
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityWarning, Code: "DOVR001", Resource: res.Identity("", "").Display(), FieldPath: fmt.Sprintf("spec.overrides[%d]", i), Message: "override is declared and will be rendered into plan artifacts", Remediation: override.Remediation, Location: res.Location})
	}
	return overrides, diags
}

func overrideDiag(res spec.Resource, index int, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{Severity: domain.SeverityError, Code: "DOVR002", Resource: res.Identity("", "").Display(), FieldPath: fmt.Sprintf("spec.overrides[%d]", index), Message: message, Remediation: remediation, Location: res.Location}
}

func attachGraphOverrides(resources []ir.ResourcePlan, overrides []ir.OverridePlan) []ir.ResourcePlan {
	if len(overrides) == 0 {
		return resources
	}
	out := make([]ir.ResourcePlan, len(resources))
	copy(out, resources)
	for i := range out {
		for _, override := range overrides {
			if sameLogicalResource(out[i].Identity, override.Resource) {
				out[i].Overrides = append(out[i].Overrides, override)
			}
		}
		sortOverrides(out[i].Overrides)
	}
	return out
}

func resourceOverridesForPlan(res spec.Resource, target string) []ir.OverridePlan {
	body, ok := resourceBody(res)
	if !ok {
		return nil
	}
	values, ok := body["overrides"].([]any)
	if !ok {
		if item, ok := body["overrides"].(map[string]any); ok {
			values = []any{item}
		}
	}
	if len(values) == 0 {
		return nil
	}
	overrides := make([]ir.OverridePlan, 0, len(values))
	for _, value := range values {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		override := ir.OverridePlan{
			Name:        stringValue(item["name"]),
			Scope:       stringValue(item["scope"]),
			Resource:    res.Identity(target, "foundation"),
			Policy:      stringValue(item["policy"]),
			Reason:      stringValue(item["reason"]),
			Remediation: stringValue(item["remediation"]),
		}
		if override.Name == "" || override.Scope == "" {
			continue
		}
		overrides = append(overrides, override)
	}
	sortOverrides(overrides)
	return overrides
}

func sameLogicalResource(left, right domain.ResourceIdentity) bool {
	return left.APIVersion == right.APIVersion && left.Kind == right.Kind && left.Namespace == right.Namespace && left.Name == right.Name
}

func resourceCapabilities(res spec.Resource, definitions *resource.Registry) []string {
	body, _ := resourceBody(res)
	values := stringSlice(body["capabilities"])
	if capability := stringValue(body["capability"]); capability != "" {
		values = append(values, capability)
	}
	if def, ok := definitions.Lookup(res.APIVersion, res.Kind); ok {
		values = append(values, def.Capabilities...)
	}
	return sortedUnique(values)
}

func resourceOwnership(res spec.Resource, body map[string]any) string {
	if boolValue(body["external"], false) {
		return "external"
	}
	ownership := stringValue(body["ownership"])
	if ownership == "" {
		ownership = "managed"
	}
	return ownership
}

func resourceLifecycle(body map[string]any) string {
	if lifecycle := stringValue(body["lifecycle"]); lifecycle != "" {
		return lifecycle
	}
	return stringValue(body["state"])
}

func graphState(ownership, lifecycle string) string {
	if ownership == "disabled" || lifecycle == "disabled" {
		return "disabled"
	}
	if ownership == "external" || ownership == "imported" {
		return "externallySatisfied"
	}
	if ownership == "planned" || lifecycle == "planned" || lifecycle == "deferred" {
		return "deferred"
	}
	return "satisfied"
}

func verificationChecks(value any) []ir.VerificationCheck {
	if body, ok := value.(map[string]any); ok {
		value = body["checks"]
	}
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	checks := make([]ir.VerificationCheck, 0, len(values))
	for i, value := range values {
		switch typed := value.(type) {
		case string:
			checks = append(checks, ir.VerificationCheck{ID: fmt.Sprintf("EXTERNAL-%03d", i+1), Description: typed})
		case map[string]any:
			id := stringDefault(stringValue(typed["id"]), fmt.Sprintf("EXTERNAL-%03d", i+1))
			description := stringDefault(stringValue(typed["description"]), stringDefault(stringValue(typed["check"]), "external resource verification"))
			checks = append(checks, ir.VerificationCheck{ID: id, Description: description})
		}
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}

func trustPolicy(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	if body, ok := value.(map[string]any); ok {
		if name := stringValue(body["name"]); name != "" {
			return name
		}
		return stringValue(body["mode"])
	}
	return ""
}

func servicesForCapability(services []provider.Service, capability string) []ir.TargetServicePlan {
	out := make([]ir.TargetServicePlan, 0)
	for _, service := range services {
		if service.Capability != "" && service.Capability != capability {
			continue
		}
		out = append(out, servicePlan(service))
	}
	return out
}

func artifactsForCapability(artifacts []provider.Artifact, capability string) []ir.ProviderArtifactPlan {
	out := make([]ir.ProviderArtifactPlan, 0)
	for _, artifact := range artifacts {
		if artifact.Capability != "" && artifact.Capability != capability {
			continue
		}
		out = append(out, artifactPlan(artifact))
	}
	return out
}

func servicePlans(services []provider.Service) []ir.TargetServicePlan {
	out := make([]ir.TargetServicePlan, 0, len(services))
	for _, service := range services {
		out = append(out, servicePlan(service))
	}
	return out
}

func servicePlan(service provider.Service) ir.TargetServicePlan {
	return ir.TargetServicePlan{
		Name: service.Name, Capability: service.Capability, Image: service.Image,
		Command: append([]string{}, service.Command...), Ports: append([]string{}, service.Ports...),
		Environment: cloneStringMap(service.Environment), Volumes: append([]string{}, service.Volumes...),
		DependsOn: append([]string{}, service.DependsOn...), Healthcheck: append([]string{}, service.Healthcheck...),
		DependsOnCompleted: append([]string{}, service.DependsOnCompleted...),
		Restart:            service.Restart, User: service.User, ReadOnly: service.ReadOnly, Init: service.Init,
		CapDrop: append([]string{}, service.CapDrop...), SecurityOpt: append([]string{}, service.SecurityOpt...),
		Tmpfs: append([]string{}, service.Tmpfs...), Secrets: append([]string{}, service.Secrets...),
		Configs: append([]string{}, service.Configs...), Profiles: append([]string{}, service.Profiles...),
		StopGracePeriod: service.StopGracePeriod, CPUs: service.CPUs, Memory: service.Memory, PidsLimit: service.PidsLimit,
	}
}

func artifactPlans(artifacts []provider.Artifact) []ir.ProviderArtifactPlan {
	out := make([]ir.ProviderArtifactPlan, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, artifactPlan(artifact))
	}
	return out
}

func artifactPlan(artifact provider.Artifact) ir.ProviderArtifactPlan {
	return ir.ProviderArtifactPlan{Path: artifact.Path, Capability: artifact.Capability, Content: cloneMap(artifact.Content)}
}

func resourceBody(res spec.Resource) (map[string]any, bool) {
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(res.Spec))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return nil, false
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, true
}

func firstCapability(values []string, fallback string) string {
	if fallback != "" {
		return fallback
	}
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func plannedCapabilities(planned []ir.ProviderResourcePlan) []string {
	values := make([]string, 0, len(planned))
	for _, item := range planned {
		values = append(values, item.Capability)
	}
	return sortedUnique(values)
}

func cloneMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func firstString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(body[key]); value != "" {
			return value
		}
	}
	return ""
}

func stringDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func boolValue(value any, fallback bool) bool {
	if b, ok := value.(bool); ok {
		return b
	}
	return fallback
}

func stringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return sortedUnique(out)
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortOverrides(overrides []ir.OverridePlan) {
	sort.SliceStable(overrides, func(i, j int) bool {
		if overrides[i].Scope != overrides[j].Scope {
			return overrides[i].Scope < overrides[j].Scope
		}
		if overrides[i].Name != overrides[j].Name {
			return overrides[i].Name < overrides[j].Name
		}
		return overrides[i].Resource.CanonicalString() < overrides[j].Resource.CanonicalString()
	})
}

func sanitizeName(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

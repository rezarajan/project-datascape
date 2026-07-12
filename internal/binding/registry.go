package binding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/canonical"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/hash"
	"datascape.dev/platformctl/internal/provider"
	"datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
)

type Definition struct {
	Identity          domain.ResourceIdentity `json:"identity"`
	Name              string                  `json:"name"`
	BindingKind       string                  `json:"bindingKind,omitempty"`
	Capability        string                  `json:"capability"`
	SourceKinds       []resource.KindRef      `json:"sourceKinds,omitempty"`
	TargetKinds       []resource.KindRef      `json:"targetKinds,omitempty"`
	ProviderTypes     []string                `json:"providerTypes,omitempty"`
	AllowCrossNS      bool                    `json:"allowCrossNamespace,omitempty"`
	AllowCycles       bool                    `json:"allowCycles,omitempty"`
	DependencyClosure []string                `json:"dependencyClosure,omitempty"`
}

type Registry struct {
	byCapability map[string]Definition
	byName       map[string]Definition
	byKind       map[string]Definition
}

type Resolved struct {
	Identity          domain.ResourceIdentity   `json:"identity"`
	Kind              string                    `json:"kind"`
	Definition        domain.ResourceIdentity   `json:"definition"`
	Capability        string                    `json:"capability"`
	Source            domain.ResourceIdentity   `json:"source,omitempty"`
	Target            domain.ResourceIdentity   `json:"target,omitempty"`
	ProviderInstance  domain.ResourceIdentity   `json:"providerInstance,omitempty"`
	Mode              string                    `json:"mode,omitempty"`
	Ownership         string                    `json:"ownership,omitempty"`
	State             string                    `json:"state"`
	Dependencies      []domain.ResourceIdentity `json:"dependencies,omitempty"`
	DependencyClosure []string                  `json:"dependencyClosure,omitempty"`
	Digest            string                    `json:"digest"`
}

func BuildRegistry(resources []spec.Resource) (*Registry, []domain.Diagnostic) {
	registry := &Registry{byCapability: map[string]Definition{}, byName: map[string]Definition{}, byKind: map[string]Definition{}}
	diags := make([]domain.Diagnostic, 0)
	for _, def := range BuiltinDefinitions() {
		if err := registry.Register(def); err != nil {
			diags = append(diags, diag(spec.Resource{}, "DBIND000", "", err.Error(), "fix built-in binding definitions"))
		}
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "BindingDefinition" {
			continue
		}
		def, err := definitionFromResource(res)
		if err != nil {
			diags = append(diags, diag(res, "DBIND001", "spec", err.Error(), "declare spec.capability for the binding definition"))
			continue
		}
		if err := registry.Register(def); err != nil {
			diags = append(diags, diag(res, "DBIND002", "metadata.name", err.Error(), "declare each binding capability once"))
		}
	}
	return registry, diags
}

func (r *Registry) Register(def Definition) error {
	if def.Name == "" {
		def.Name = def.Identity.Name
	}
	if def.Capability == "" {
		return fmt.Errorf("binding definition must declare capability")
	}
	if _, ok := r.byCapability[def.Capability]; ok {
		return fmt.Errorf("duplicate binding capability %s", def.Capability)
	}
	def.ProviderTypes = sortedUnique(def.ProviderTypes)
	def.DependencyClosure = sortedUnique(def.DependencyClosure)
	sort.SliceStable(def.SourceKinds, func(i, j int) bool { return def.SourceKinds[i].Key() < def.SourceKinds[j].Key() })
	sort.SliceStable(def.TargetKinds, func(i, j int) bool { return def.TargetKinds[i].Key() < def.TargetKinds[j].Key() })
	r.byCapability[def.Capability] = def
	r.byName[def.Name] = def
	if def.BindingKind != "" {
		if _, ok := r.byKind[def.BindingKind]; ok {
			return fmt.Errorf("duplicate typed binding kind %s", def.BindingKind)
		}
		r.byKind[def.BindingKind] = def
	}
	return nil
}

func (r *Registry) ForCapability(capability string) (Definition, bool) {
	def, ok := r.byCapability[capability]
	return def, ok
}

func (r *Registry) ForName(name string) (Definition, bool) {
	def, ok := r.byName[name]
	return def, ok
}

func (r *Registry) ForBindingKind(kind string) (Definition, bool) {
	def, ok := r.byKind[kind]
	return def, ok
}

func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.byCapability))
	for _, def := range r.byCapability {
		out = append(out, def)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Capability < out[j].Capability })
	return out
}

func BuiltinDefinitions() []Definition {
	return []Definition{
		{Identity: bindingDefIdentity("cdc"), Name: "cdc", BindingKind: "CDCBinding", Capability: "datascape.dev/source.change-stream", SourceKinds: []resource.KindRef{{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "RelationalSource"}}, TargetKinds: []resource.KindRef{{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"}}, ProviderTypes: []string{"datascape.dev/source"}, DependencyClosure: []string{"datascape.dev/source.relational", "datascape.dev/stream.publish"}},
		{Identity: bindingDefIdentity("stream-publish"), Name: "stream-publish", BindingKind: "StreamPublishBinding", Capability: "datascape.dev/stream.publish", SourceKinds: []resource.KindRef{{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "EventProducer"}}, TargetKinds: []resource.KindRef{{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"}}, ProviderTypes: []string{"datascape.dev/stream"}, DependencyClosure: []string{"datascape.dev/stream.publish"}},
		{Identity: bindingDefIdentity("stream-archive"), Name: "stream-archive", BindingKind: "StreamArchiveBinding", Capability: "datascape.dev/store.object", SourceKinds: []resource.KindRef{{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"}}, TargetKinds: []resource.KindRef{{APIVersion: "stores.datascape.dev/v1alpha1", Kind: "ObjectStore"}}, ProviderTypes: []string{"datascape.dev/store"}, DependencyClosure: []string{"datascape.dev/store.object"}},
		{Identity: bindingDefIdentity("lineage"), Name: "lineage", BindingKind: "LineageBinding", Capability: "datascape.dev/lineage.admit", SourceKinds: bindingSourceKinds(), TargetKinds: []resource.KindRef{{APIVersion: "lineage.datascape.dev/v1alpha1", Kind: "LineageSink"}}, ProviderTypes: []string{"datascape.dev/lineage"}, DependencyClosure: []string{"datascape.dev/lineage.admit"}},
		{Identity: bindingDefIdentity("audit"), Name: "audit", BindingKind: "AuditBinding", Capability: "datascape.dev/audit.record", SourceKinds: bindingSourceKinds(), TargetKinds: []resource.KindRef{{APIVersion: "audit.datascape.dev/v1alpha1", Kind: "AuditStore"}}, ProviderTypes: []string{"datascape.dev/audit"}, DependencyClosure: []string{"datascape.dev/audit.record"}},
		{Identity: bindingDefIdentity("pipeline"), Name: "pipeline", BindingKind: "PipelineBinding", Capability: "datascape.dev/pipeline.run", SourceKinds: bindingSourceKinds(), TargetKinds: []resource.KindRef{{APIVersion: "pipelines.datascape.dev/v1alpha1", Kind: "Pipeline"}}, ProviderTypes: []string{"datascape.dev/pipeline"}, DependencyClosure: []string{"datascape.dev/pipeline.run"}},
		{Identity: bindingDefIdentity("access"), Name: "access", BindingKind: "AccessBinding", Capability: "datascape.dev/access.grant", SourceKinds: bindingSourceKinds(), ProviderTypes: []string{"datascape.dev/access"}, DependencyClosure: []string{"datascape.dev/access.grant"}},
		{Identity: bindingDefIdentity("batch-ingest"), Name: "batch-ingest", BindingKind: "BatchIngestBinding", Capability: "datascape.dev/ingest.batch", SourceKinds: []resource.KindRef{{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "RelationalSource"}}, TargetKinds: []resource.KindRef{{APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table"}}, ProviderTypes: []string{"datascape.dev/pipeline"}, DependencyClosure: []string{"datascape.dev/pipeline.run"}},
		{Identity: bindingDefIdentity("stream-ingest"), Name: "stream-ingest", BindingKind: "StreamIngestBinding", Capability: "datascape.dev/ingest.stream", SourceKinds: []resource.KindRef{{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"}}, TargetKinds: []resource.KindRef{{APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table"}}, ProviderTypes: []string{"datascape.dev/pipeline"}, DependencyClosure: []string{"datascape.dev/pipeline.run", "datascape.dev/catalog.table"}},
		{Identity: bindingDefIdentity("transform"), Name: "transform", BindingKind: "TransformBinding", Capability: "datascape.dev/transform.table", SourceKinds: []resource.KindRef{{APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table"}}, TargetKinds: []resource.KindRef{{APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table"}}, ProviderTypes: []string{"datascape.dev/pipeline"}, DependencyClosure: []string{"datascape.dev/pipeline.run", "datascape.dev/catalog.table"}},
		{Identity: bindingDefIdentity("volume-mount"), Name: "volume-mount", BindingKind: "VolumeMountBinding", Capability: "datascape.dev/storage.mount", SourceKinds: []resource.KindRef{{APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolumeClaim"}}, ProviderTypes: []string{"datascape.dev/storage"}, DependencyClosure: []string{"datascape.dev/storage.volume"}},
	}
}

type input struct {
	Kind                string
	Capability          string
	SourceRef           string
	SourceField         string
	SourceRequired      bool
	TargetRef           string
	TargetField         string
	TargetRequired      bool
	ProviderInstanceRef string
	Mode                string
	Ownership           string
	State               string
}

func bindingInputFromResource(res spec.Resource, registry *Registry) (input, bool, []domain.Diagnostic) {
	body, err := specBody(res)
	if err != nil {
		if res.APIVersion == api.PlatformV1Alpha1 && res.Kind == "Binding" {
			return input{}, false, []domain.Diagnostic{diag(res, "DBIND003", "spec", err.Error(), "use object-valued binding spec")}
		}
		if _, ok := registry.ForBindingKind(res.Kind); ok {
			return input{}, false, []domain.Diagnostic{diag(res, "DBIND003", "spec", err.Error(), "use object-valued binding spec")}
		}
		return input{}, false, nil
	}
	common := input{
		Kind:                res.Kind,
		ProviderInstanceRef: stringField(body, "providerInstanceRef"),
		Mode:                stringField(body, "mode"),
		Ownership:           stringFieldDefault(body, "ownership", "managed"),
		State:               stringField(body, "state"),
	}
	if res.APIVersion == api.PlatformV1Alpha1 && res.Kind == "Binding" {
		capability := stringField(body, "capability")
		if capability == "" {
			defName := stringField(body, "bindingDefinitionRef")
			if defName != "" {
				if def, ok := registry.ForName(refName(defName)); ok {
					capability = def.Capability
				}
			}
		}
		if capability == "" {
			return input{}, false, []domain.Diagnostic{diag(res, "DBIND004", "spec.capability", "binding must declare a capability", "set spec.capability or spec.bindingDefinitionRef")}
		}
		common.Capability = capability
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "targetRef")
		common.TargetField = "targetRef"
		return common, true, nil
	}
	def, ok := registry.ForBindingKind(res.Kind)
	if !ok {
		return input{}, false, nil
	}
	common.Capability = def.Capability
	common.SourceRequired = true
	common.TargetRequired = true
	switch res.Kind {
	case "CDCBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "streamRef")
		common.TargetField = "streamRef"
	case "StreamPublishBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "streamRef")
		common.TargetField = "streamRef"
	case "StreamArchiveBinding":
		common.SourceRef = stringField(body, "streamRef")
		common.SourceField = "streamRef"
		common.TargetRef = stringField(body, "objectStoreRef")
		common.TargetField = "objectStoreRef"
	case "LineageBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "sinkRef")
		common.TargetField = "sinkRef"
	case "AuditBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "auditStoreRef")
		common.TargetField = "auditStoreRef"
	case "PipelineBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = firstString(body, "pipelineRef", "targetRef")
		common.TargetField = "pipelineRef"
	case "AccessBinding":
		common.SourceRef = firstString(body, "subjectRef", "sourceRef")
		common.SourceField = "subjectRef"
		common.TargetRef = firstString(body, "resourceRef", "targetRef")
		common.TargetField = "resourceRef"
	case "BatchIngestBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "tableRef")
		common.TargetField = "tableRef"
	case "StreamIngestBinding":
		common.SourceRef = stringField(body, "streamRef")
		common.SourceField = "streamRef"
		common.TargetRef = stringField(body, "tableRef")
		common.TargetField = "tableRef"
	case "TransformBinding":
		common.SourceRef = stringField(body, "sourceRef")
		common.SourceField = "sourceRef"
		common.TargetRef = stringField(body, "targetRef")
		common.TargetField = "targetRef"
	case "VolumeMountBinding":
		common.SourceRef = stringField(body, "claimRef")
		common.SourceField = "claimRef"
		common.TargetRef = stringField(body, "workloadRef")
		common.TargetField = "workloadRef"
	default:
		return input{}, false, nil
	}
	return common, true, nil
}

func bindingSourceKinds() []resource.KindRef {
	return []resource.KindRef{
		{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "RelationalSource"},
		{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "EventProducer"},
		{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"},
		{APIVersion: "pipelines.datascape.dev/v1alpha1", Kind: "Pipeline"},
		{APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table"},
		{APIVersion: "databases.datascape.dev/v1alpha1", Kind: "DatabaseInstance"},
		{APIVersion: "catalogs.datascape.dev/v1alpha1", Kind: "TableCatalog"},
		{APIVersion: "compute.datascape.dev/v1alpha1", Kind: "QueryEngine"},
	}
}

func providerCompatibleForBinding(descriptor provider.Descriptor, instance provider.Instance, def Definition, capability, bindingKind, target string) bool {
	if target != "" && instance.Target != "" && instance.Target != target {
		return false
	}
	if len(def.ProviderTypes) > 0 && !contains(def.ProviderTypes, descriptor.Type) {
		return false
	}
	if !contains(instance.Capabilities, capability) {
		return false
	}
	return len(descriptor.BindingKinds) == 0 || contains(descriptor.BindingKinds, bindingKind)
}

func Resolve(resources []spec.Resource, registry *Registry, definitions *resource.Registry, providers *provider.Registry, target string) ([]Resolved, []domain.Diagnostic) {
	byID := resourceIndex(resources, target)
	diags := make([]domain.Diagnostic, 0)
	resolved := make([]Resolved, 0)
	for _, res := range resources {
		input, ok, inputDiags := bindingInputFromResource(res, registry)
		diags = append(diags, inputDiags...)
		if !ok {
			continue
		}
		capability := input.Capability
		def, ok := registry.ForCapability(input.Capability)
		if !ok {
			diags = append(diags, diag(res, "DBIND005", "spec.capability", "unknown binding capability "+capability, "declare a BindingDefinition for this capability"))
			continue
		}
		source := parseRef(input.SourceRef, res, "", target)
		targetRef := parseRef(input.TargetRef, res, "", target)
		if input.SourceRequired && input.SourceRef == "" {
			diags = append(diags, diag(res, "DBIND014", "spec."+input.SourceField, input.Kind+" must declare "+input.SourceField, "set spec."+input.SourceField))
		}
		if input.TargetRequired && input.TargetRef == "" {
			diags = append(diags, diag(res, "DBIND014", "spec."+input.TargetField, input.Kind+" must declare "+input.TargetField, "set spec."+input.TargetField))
		}
		if source.Name == "" && input.SourceRef != "" {
			diags = append(diags, diag(res, "DBIND006", "spec."+input.SourceField, input.SourceField+" must use Kind/name, Kind/namespace/name, or apiVersion/Kind/namespace/name", "use canonical reference syntax"))
		}
		if targetRef.Name == "" && input.TargetRef != "" {
			diags = append(diags, diag(res, "DBIND007", "spec."+input.TargetField, input.TargetField+" must use Kind/name, Kind/namespace/name, or apiVersion/Kind/namespace/name", "use canonical reference syntax"))
		}
		if source.Name != "" {
			if _, ok := byID[source.CanonicalString()]; !ok {
				diags = append(diags, diag(res, "DBIND008", "spec."+input.SourceField, "source resource does not exist: "+input.SourceRef, "declare the source resource or correct the reference"))
			}
			if !kindAllowed(def.SourceKinds, source) {
				diags = append(diags, diag(res, "DBIND009", "spec."+input.SourceField, "source kind is not compatible with "+capability, "choose a compatible source or update the BindingDefinition"))
			}
		}
		if targetRef.Name != "" {
			if _, ok := byID[targetRef.CanonicalString()]; !ok {
				diags = append(diags, diag(res, "DBIND010", "spec."+input.TargetField, "target resource does not exist: "+input.TargetRef, "declare the target resource or correct the reference"))
			}
			if !kindAllowed(def.TargetKinds, targetRef) {
				diags = append(diags, diag(res, "DBIND011", "spec."+input.TargetField, "target kind is not compatible with "+capability, "choose a compatible target or update the BindingDefinition"))
			}
		}
		providerInstance := domain.ResourceIdentity{}
		if providerRef := input.ProviderInstanceRef; providerRef != "" {
			providerInstance = parseRef(providerRef, res, "ProviderInstance", target)
			if instance, descriptor, ok := providers.Instance(providerInstance); !ok {
				diags = append(diags, diag(res, "DBIND012", "spec.providerInstanceRef", "provider instance does not exist: "+providerRef, "declare a compatible ProviderInstance or remove spec.providerInstanceRef"))
			} else if !providerCompatibleForBinding(descriptor, instance, def, capability, input.Kind, target) {
				diags = append(diags, diag(res, "DBIND015", "spec.providerInstanceRef", "provider instance does not satisfy "+input.Kind+" for "+capability, "choose a provider instance that advertises the typed binding kind and capability"))
			}
		} else if instance, descriptor, ok := providers.ResolveBinding(capability, input.Kind, target); ok {
			if len(def.ProviderTypes) == 0 || contains(def.ProviderTypes, descriptor.Type) {
				providerInstance = instance.Identity
			}
		}
		if providerInstance.Name == "" {
			diags = append(diags, diag(res, "DBIND012", "spec.providerInstanceRef", "no provider instance can satisfy "+capability, "declare a compatible ProviderInstance or provider capability"))
		}
		deps := make([]domain.ResourceIdentity, 0, 2)
		if source.Name != "" {
			deps = append(deps, source)
		}
		if targetRef.Name != "" {
			deps = append(deps, targetRef)
		}
		digest, _ := bindingDigest(res, capability, source, targetRef, providerInstance)
		state := graphState(input.Ownership, input.State)
		resolved = append(resolved, Resolved{
			Identity:          res.Identity(target, ""),
			Kind:              input.Kind,
			Definition:        def.Identity,
			Capability:        capability,
			Source:            source,
			Target:            targetRef,
			ProviderInstance:  providerInstance,
			Mode:              input.Mode,
			Ownership:         input.Ownership,
			State:             state,
			Dependencies:      deps,
			DependencyClosure: append([]string{}, def.DependencyClosure...),
			Digest:            digest,
		})
	}
	sort.SliceStable(resolved, func(i, j int) bool {
		return resolved[i].Identity.CanonicalString() < resolved[j].Identity.CanonicalString()
	})
	diags = append(diags, detectCycles(resolved)...)
	_ = definitions
	return resolved, diags
}

func definitionFromResource(res spec.Resource) (Definition, error) {
	body, err := specBody(res)
	if err != nil {
		return Definition{}, err
	}
	capability := stringField(body, "capability")
	if capability == "" {
		return Definition{}, fmt.Errorf("binding definition must declare capability")
	}
	return Definition{
		Identity:          res.Identity("", ""),
		Name:              res.Metadata.Name,
		BindingKind:       stringField(body, "bindingKind"),
		Capability:        capability,
		SourceKinds:       kindRefs(body["sourceKinds"]),
		TargetKinds:       kindRefs(body["targetKinds"]),
		ProviderTypes:     stringSlice(body["providerTypes"]),
		AllowCrossNS:      boolField(body, "allowCrossNamespace"),
		AllowCycles:       boolField(body, "allowCycles"),
		DependencyClosure: stringSlice(body["dependencyClosure"]),
	}, nil
}

func detectCycles(bindings []Resolved) []domain.Diagnostic {
	edges := map[string][]string{}
	bindingForEdge := map[string]Resolved{}
	for _, binding := range bindings {
		if binding.State == "disabled" || binding.Source.Name == "" || binding.Target.Name == "" {
			continue
		}
		source := binding.Source.CanonicalString()
		target := binding.Target.CanonicalString()
		edges[source] = append(edges[source], target)
		bindingForEdge[source+"->"+target] = binding
	}
	visited := map[string]int{}
	var stack []string
	var diags []domain.Diagnostic
	var visit func(string) bool
	visit = func(node string) bool {
		if visited[node] == 1 {
			if len(stack) > 0 {
				prev := stack[len(stack)-1]
				if binding, ok := bindingForEdge[prev+"->"+node]; ok {
					diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DBIND013", Resource: binding.Identity.Display(), FieldPath: "spec.targetRef", Message: "binding graph contains a cycle", Remediation: "remove the cyclic binding or mark a BindingDefinition as allowing cycles"})
				}
			}
			return true
		}
		if visited[node] == 2 {
			return false
		}
		visited[node] = 1
		stack = append(stack, node)
		for _, next := range edges[node] {
			if visit(next) {
				return true
			}
		}
		stack = stack[:len(stack)-1]
		visited[node] = 2
		return false
	}
	nodes := make([]string, 0, len(edges))
	for node := range edges {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)
	for _, node := range nodes {
		if visit(node) {
			break
		}
	}
	return diags
}

func resourceIndex(resources []spec.Resource, target string) map[string]spec.Resource {
	out := map[string]spec.Resource{}
	for _, res := range resources {
		out[res.Identity(target, "").CanonicalString()] = res
	}
	return out
}

func parseRef(value string, owner spec.Resource, expectedKind, target string) domain.ResourceIdentity {
	if value == "" {
		return domain.ResourceIdentity{}
	}
	parts := strings.Split(value, "/")
	ns := owner.Metadata.Namespace
	if ns == "" {
		ns = api.DefaultNamespace
	}
	apiVersion := ""
	kind := expectedKind
	name := ""
	switch len(parts) {
	case 2:
		apiVersion = apiVersionForKind(parts[0])
		kind, name = parts[0], parts[1]
	case 3:
		apiVersion = apiVersionForKind(parts[0])
		kind, ns, name = parts[0], parts[1], parts[2]
	case 5:
		apiVersion = parts[0] + "/" + parts[1]
		kind, ns, name = parts[2], parts[3], parts[4]
	default:
		return domain.ResourceIdentity{}
	}
	if clusterScopedKind(kind) {
		ns = api.DefaultNamespace
	}
	if apiVersion == "" {
		apiVersion = api.PlatformV1Alpha1
	}
	return domain.ResourceIdentity{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name, Target: target}
}

func clusterScopedKind(kind string) bool {
	switch kind {
	case "StorageClass", "PersistentVolume", "DatabaseClass", "ConnectorClass":
		return true
	default:
		return false
	}
}

func apiVersionForKind(kind string) string {
	switch kind {
	case "RelationalSource", "EventProducer":
		return "sources.datascape.dev/v1alpha1"
	case "EventStream":
		return "streams.datascape.dev/v1alpha1"
	case "EventContract":
		return "contracts.datascape.dev/v1alpha1"
	case "DatabaseConnection", "ObjectStoreConnection", "EventStreamConnection", "ConnectorClass":
		return "connections.datascape.dev/v1alpha1"
	case "DatabaseClass", "DatabaseInstance":
		return "databases.datascape.dev/v1alpha1"
	case "StorageClass", "PersistentVolume", "PersistentVolumeClaim":
		return "storage.datascape.dev/v1alpha1"
	case "ObjectStore", "Warehouse":
		return "stores.datascape.dev/v1alpha1"
	case "LineageSink":
		return "lineage.datascape.dev/v1alpha1"
	case "AuditStore":
		return "audit.datascape.dev/v1alpha1"
	case "Pipeline":
		return "pipelines.datascape.dev/v1alpha1"
	case "Table":
		return "tables.datascape.dev/v1alpha1"
	case "TableCatalog", "MetadataCatalog":
		return "catalogs.datascape.dev/v1alpha1"
	case "QueryEngine":
		return "compute.datascape.dev/v1alpha1"
	case "DataQualityRule":
		return "quality.datascape.dev/v1alpha1"
	case "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding", "BatchIngestBinding", "StreamIngestBinding", "TransformBinding", "VolumeMountBinding":
		return "bindings.datascape.dev/v1alpha1"
	default:
		return api.PlatformV1Alpha1
	}
}

func kindAllowed(allowed []resource.KindRef, id domain.ResourceIdentity) bool {
	if len(allowed) == 0 || id.Name == "" {
		return true
	}
	for _, ref := range allowed {
		if ref.APIVersion == id.APIVersion && ref.Kind == id.Kind {
			return true
		}
	}
	return false
}

func bindingDigest(res spec.Resource, capability string, source, target, instance domain.ResourceIdentity) (string, error) {
	material := map[string]any{
		"apiVersion":       res.APIVersion,
		"kind":             res.Kind,
		"namespace":        defaultNamespace(res.Metadata.Namespace),
		"name":             res.Metadata.Name,
		"capability":       capability,
		"source":           source.CanonicalString(),
		"target":           target.CanonicalString(),
		"providerInstance": instance.CanonicalString(),
		"spec":             rawSpec(res),
	}
	content, err := canonical.JSON(material)
	if err != nil {
		return "", err
	}
	return hash.Bytes(content), nil
}

func rawSpec(res spec.Resource) any {
	var body any
	dec := json.NewDecoder(bytes.NewReader(res.Spec))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return map[string]any{}
	}
	return body
}

func specBody(res spec.Resource) (map[string]any, error) {
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(res.Spec))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return nil, err
	}
	if body == nil {
		body = map[string]any{}
	}
	return body, nil
}

func kindRefs(value any) []resource.KindRef {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]resource.KindRef, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			parts := strings.Split(typed, "/")
			if len(parts) == 2 {
				out = append(out, resource.KindRef{APIVersion: parts[0], Kind: parts[1]})
			}
		case map[string]any:
			out = append(out, resource.KindRef{APIVersion: stringField(typed, "apiVersion"), Kind: stringField(typed, "kind")})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func stringField(body map[string]any, key string) string {
	value, _ := body[key].(string)
	return value
}

func stringFieldDefault(body map[string]any, key, fallback string) string {
	if value := stringField(body, key); value != "" {
		return value
	}
	return fallback
}

func firstString(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringField(body, key); value != "" {
			return value
		}
	}
	return ""
}

func boolField(body map[string]any, key string) bool {
	value, _ := body[key].(bool)
	return value
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

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func refName(value string) string {
	parts := strings.Split(value, "/")
	if len(parts) == 0 {
		return value
	}
	return parts[len(parts)-1]
}

func graphState(ownership, state string) string {
	if ownership == "disabled" || state == "disabled" {
		return "disabled"
	}
	if ownership == "external" || ownership == "imported" {
		return "externallySatisfied"
	}
	if ownership == "planned" || state == "planned" || state == "deferred" {
		return "deferred"
	}
	return "satisfied"
}

func defaultNamespace(namespace string) string {
	if namespace == "" {
		return api.DefaultNamespace
	}
	return namespace
}

func bindingDefIdentity(name string) domain.ResourceIdentity {
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "BindingDefinition", Namespace: api.DefaultNamespace, Name: name}
}

func diag(res spec.Resource, code, field, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{Severity: domain.SeverityError, Code: code, Resource: res.Identity("", "").Display(), FieldPath: field, Message: message, Remediation: remediation, Location: res.Location}
}

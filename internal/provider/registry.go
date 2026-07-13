package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
)

type Service struct {
	Name               string            `json:"name"`
	Capability         string            `json:"capability,omitempty"`
	Image              string            `json:"image"`
	Command            []string          `json:"command,omitempty"`
	Ports              []string          `json:"ports,omitempty"`
	Environment        map[string]string `json:"environment,omitempty"`
	Volumes            []string          `json:"volumes,omitempty"`
	DependsOn          []string          `json:"dependsOn,omitempty"`
	DependsOnCompleted []string          `json:"dependsOnCompleted,omitempty"`
	Healthcheck        []string          `json:"healthcheck,omitempty"`
	Restart            string            `json:"restart,omitempty"`
	User               string            `json:"user,omitempty"`
	ReadOnly           bool              `json:"readOnly,omitempty"`
	Init               bool              `json:"init,omitempty"`
	CapDrop            []string          `json:"capDrop,omitempty"`
	SecurityOpt        []string          `json:"securityOpt,omitempty"`
	Tmpfs              []string          `json:"tmpfs,omitempty"`
	Secrets            []string          `json:"secrets,omitempty"`
	Configs            []string          `json:"configs,omitempty"`
	Profiles           []string          `json:"profiles,omitempty"`
	StopGracePeriod    string            `json:"stopGracePeriod,omitempty"`
	CPUs               string            `json:"cpus,omitempty"`
	Memory             string            `json:"memory,omitempty"`
	PidsLimit          int               `json:"pidsLimit,omitempty"`
}

type Artifact struct {
	Path       string         `json:"path"`
	Capability string         `json:"capability,omitempty"`
	Content    map[string]any `json:"content,omitempty"`
}

type Descriptor struct {
	Identity            domain.ResourceIdentity `json:"identity"`
	Type                string                  `json:"type"`
	Capabilities        []string                `json:"capabilities"`
	ResourceKinds       []resource.KindRef      `json:"resourceKinds,omitempty"`
	BindingKinds        []string                `json:"bindingKinds,omitempty"`
	TargetCompatibility []string                `json:"targetCompatibility,omitempty"`
	RuntimeDependencies []string                `json:"runtimeDependencies,omitempty"`
	Services            []Service               `json:"services,omitempty"`
	Artifacts           []Artifact              `json:"artifacts,omitempty"`
	RendererContract    string                  `json:"rendererContract,omitempty"`
	Conformance         []string                `json:"conformance,omitempty"`
	PackageVersion      string                  `json:"packageVersion,omitempty"`
	ContractVersion     string                  `json:"contractVersion,omitempty"`
	PackageDigest       string                  `json:"packageDigest,omitempty"`
	Provenance          string                  `json:"provenance,omitempty"`
}

type Instance struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	Provider     domain.ResourceIdentity `json:"provider"`
	Type         string                  `json:"type"`
	Target       string                  `json:"target,omitempty"`
	Capabilities []string                `json:"capabilities"`
	Parameters   map[string]any          `json:"parameters,omitempty"`
}

type Registry struct {
	descriptors map[string]Descriptor
	instances   map[string]Instance
}

func NewRegistry() *Registry {
	return &Registry{descriptors: map[string]Descriptor{}, instances: map[string]Instance{}}
}

func BuildRegistry(resources []spec.Resource, target string) (*Registry, []domain.Diagnostic) {
	registry := NewRegistry()
	diags := make([]domain.Diagnostic, 0)
	for _, descriptor := range BuiltinDescriptors() {
		if err := registry.RegisterDescriptor(descriptor); err != nil {
			diags = append(diags, diag(spec.Resource{}, "DPROV000", "", err.Error(), "fix built-in provider descriptors"))
		}
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "Provider" {
			continue
		}
		descriptor, err := descriptorFromResource(res)
		if err != nil {
			diags = append(diags, diag(res, "DPROV001", "spec", err.Error(), "declare provider type and capabilities"))
			continue
		}
		if err := registry.RegisterDescriptor(descriptor); err != nil {
			diags = append(diags, diag(res, "DPROV002", "metadata.name", err.Error(), "declare each provider once"))
		}
	}
	for _, instance := range BuiltinInstances(target) {
		if err := registry.RegisterInstance(instance); err != nil {
			diags = append(diags, diag(spec.Resource{}, "DPROV003", "", err.Error(), "fix built-in provider instances"))
		}
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "ProviderInstance" {
			continue
		}
		instance, err := instanceFromResource(res, target)
		if err != nil {
			diags = append(diags, diag(res, "DPROV004", "spec", err.Error(), "declare spec.providerRef for the provider instance"))
			continue
		}
		if err := registry.RegisterInstance(instance); err != nil {
			diags = append(diags, diag(res, "DPROV005", "metadata.name", err.Error(), "declare each provider instance once"))
		}
	}
	for _, instance := range registry.Instances() {
		descriptor, ok := registry.descriptors[instance.Provider.CanonicalString()]
		if !ok {
			diags = append(diags, domain.Diagnostic{
				Severity:    domain.SeverityError,
				Code:        "DPROV006",
				Resource:    instance.Identity.Display(),
				FieldPath:   "spec.providerRef",
				Message:     "provider instance references an unknown provider",
				Remediation: "declare the Provider or correct spec.providerRef",
			})
			continue
		}
		if len(instance.Capabilities) == 0 {
			instance.Capabilities = append([]string{}, descriptor.Capabilities...)
			registry.instances[instance.Identity.CanonicalString()] = instance
		}
		if !targetCompatible(descriptor, instance.Target) {
			diags = append(diags, domain.Diagnostic{
				Severity:    domain.SeverityError,
				Code:        "DPROV007",
				Resource:    instance.Identity.Display(),
				FieldPath:   "spec.target",
				Message:     "provider instance is not compatible with target " + instance.Target,
				Remediation: "choose a provider that lists this target in targetCompatibility",
			})
		}
	}
	return registry, diags
}

func (r *Registry) RegisterDescriptor(descriptor Descriptor) error {
	if descriptor.Identity.Name == "" {
		return fmt.Errorf("provider must have an identity")
	}
	key := descriptor.Identity.CanonicalString()
	if strings.HasPrefix(descriptor.Identity.Name, "local-") {
		if descriptor.PackageVersion == "" {
			descriptor.PackageVersion = "builtin"
		}
		if descriptor.ContractVersion == "" {
			descriptor.ContractVersion = "v1alpha1"
		}
	}
	if _, ok := r.descriptors[key]; ok {
		return fmt.Errorf("duplicate provider %s", descriptor.Identity.Display())
	}
	descriptor.Capabilities = sortedUnique(descriptor.Capabilities)
	sort.SliceStable(descriptor.ResourceKinds, func(i, j int) bool { return descriptor.ResourceKinds[i].Key() < descriptor.ResourceKinds[j].Key() })
	descriptor.BindingKinds = sortedUnique(descriptor.BindingKinds)
	descriptor.TargetCompatibility = sortedUnique(descriptor.TargetCompatibility)
	descriptor.RuntimeDependencies = sortedUnique(descriptor.RuntimeDependencies)
	sort.SliceStable(descriptor.Services, func(i, j int) bool { return descriptor.Services[i].Name < descriptor.Services[j].Name })
	sort.SliceStable(descriptor.Artifacts, func(i, j int) bool { return descriptor.Artifacts[i].Path < descriptor.Artifacts[j].Path })
	r.descriptors[key] = descriptor
	return nil
}

func (r *Registry) RegisterInstance(instance Instance) error {
	if instance.Identity.Name == "" {
		return fmt.Errorf("provider instance must have an identity")
	}
	key := instance.Identity.CanonicalString()
	if _, ok := r.instances[key]; ok {
		return fmt.Errorf("duplicate provider instance %s", instance.Identity.Display())
	}
	instance.Capabilities = sortedUnique(instance.Capabilities)
	if instance.Parameters == nil {
		instance.Parameters = map[string]any{}
	}
	r.instances[key] = instance
	return nil
}

func (r *Registry) Descriptor(id domain.ResourceIdentity) (Descriptor, bool) {
	descriptor, ok := r.descriptors[id.CanonicalString()]
	return descriptor, ok
}

func (r *Registry) ProviderForInstance(instance Instance) (Descriptor, bool) {
	return r.Descriptor(instance.Provider)
}

func (r *Registry) Descriptors() []Descriptor {
	out := make([]Descriptor, 0, len(r.descriptors))
	for _, descriptor := range r.descriptors {
		out = append(out, descriptor)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identity.CanonicalString() < out[j].Identity.CanonicalString() })
	return out
}

func (r *Registry) Instances() []Instance {
	out := make([]Instance, 0, len(r.instances))
	for _, instance := range r.instances {
		out = append(out, instance)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Identity.CanonicalString() < out[j].Identity.CanonicalString() })
	return out
}

func (r *Registry) Instance(id domain.ResourceIdentity) (Instance, Descriptor, bool) {
	instance, ok := r.instances[id.CanonicalString()]
	if !ok {
		return Instance{}, Descriptor{}, false
	}
	descriptor, ok := r.ProviderForInstance(instance)
	if !ok {
		return instance, Descriptor{}, false
	}
	return instance, descriptor, true
}

func (r *Registry) ResolveCapability(capability, target string) (Instance, Descriptor, bool) {
	candidates := r.CapabilityCandidates(capability, target)
	if len(candidates) == 1 {
		return candidates[0].Instance, candidates[0].Descriptor, true
	}
	return Instance{}, Descriptor{}, false
}

func (r *Registry) ResolveBinding(capability, bindingKind, target string) (Instance, Descriptor, bool) {
	candidates := r.BindingCandidates(capability, bindingKind, target)
	if len(candidates) == 1 {
		return candidates[0].Instance, candidates[0].Descriptor, true
	}
	return Instance{}, Descriptor{}, false
}

type Candidate struct {
	Instance   Instance
	Descriptor Descriptor
}

func (r *Registry) CapabilityCandidates(capability, target string) []Candidate {
	out := make([]Candidate, 0)
	for _, instance := range r.Instances() {
		if target != "" && instance.Target != "" && instance.Target != target {
			continue
		}
		if !contains(instance.Capabilities, capability) {
			continue
		}
		descriptor, ok := r.ProviderForInstance(instance)
		if !ok || !targetCompatible(descriptor, target) {
			continue
		}
		out = append(out, Candidate{Instance: instance, Descriptor: descriptor})
	}
	return out
}

func (r *Registry) BindingCandidates(capability, bindingKind, target string) []Candidate {
	out := make([]Candidate, 0)
	for _, candidate := range r.CapabilityCandidates(capability, target) {
		if len(candidate.Descriptor.BindingKinds) > 0 && !contains(candidate.Descriptor.BindingKinds, bindingKind) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func BuiltinDescriptors() []Descriptor {
	return []Descriptor{
		{
			Identity:            providerIdentity("local-storage"),
			Type:                "datascape.dev/storage",
			Capabilities:        []string{"datascape.dev/storage.volume", "datascape.dev/storage.mount"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolume"}, {APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolumeClaim"}},
			BindingKinds:        []string{"Binding", "VolumeMountBinding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			ContractVersion:     "v1alpha1",
			PackageVersion:      "builtin",
			Conformance:         []string{"STORAGE-001"},
		},
		{
			Identity:            providerIdentity("local-source"),
			Type:                "datascape.dev/source",
			Capabilities:        []string{"datascape.dev/source.relational"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "sources.datascape.dev/v1alpha1", Kind: "RelationalSource"}},
			BindingKinds:        []string{"Binding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			Conformance:         []string{"SOURCE-001"},
			Services: []Service{{
				Name:        "relational-source",
				Capability:  "datascape.dev/source.relational",
				Image:       "postgres:16",
				Ports:       []string{"5432:5432"},
				Environment: map[string]string{"POSTGRES_DB": "datascape", "POSTGRES_PASSWORD": "${DATASCAPE_SOURCE_PASSWORD:?set DATASCAPE_SOURCE_PASSWORD}", "POSTGRES_USER": "datascape"},
				Volumes:     []string{"relational-source-data:/var/lib/postgresql/data"},
				Healthcheck: []string{"CMD-SHELL", "pg_isready -U datascape -d datascape"},
			}},
		},
		{
			Identity:            providerIdentity("local-cdc"),
			Type:                "datascape.dev/cdc",
			Capabilities:        []string{"datascape.dev/source.change-stream"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "cdc.datascape.dev/v1alpha1", Kind: "CDCInstance"}},
			BindingKinds:        []string{"Binding", "CDCBinding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			ContractVersion:     "v1alpha1",
			PackageVersion:      "builtin",
			Conformance:         []string{"CDC-001"},
		},
		{
			Identity:            providerIdentity("local-event-stream"),
			Type:                "datascape.dev/stream",
			Capabilities:        []string{"datascape.dev/stream.publish", "datascape.dev/schema.register"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream"}, {APIVersion: "contracts.datascape.dev/v1alpha1", Kind: "EventContract"}},
			BindingKinds:        []string{"Binding", "StreamPublishBinding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			Conformance:         []string{"EVENTSTREAM-001"},
			Services: []Service{{
				Name:       "event-stream",
				Capability: "datascape.dev/stream.publish",
				Image:      "redpandadata/redpanda:v24.1.1",
				Command:    []string{"redpanda", "start", "--overprovisioned", "--smp", "1", "--memory", "1G", "--reserve-memory", "0M", "--node-id", "0", "--check=false", "--kafka-addr", "PLAINTEXT://0.0.0.0:9092", "--advertise-kafka-addr", "PLAINTEXT://event-stream:9092"},
				Ports:      []string{"9092:9092", "9644:9644"},
				Volumes:    []string{"event-stream-data:/var/lib/redpanda/data"},
				Healthcheck: []string{
					"CMD-SHELL",
					"rpk cluster health -X brokers=localhost:9092 | grep -E 'Healthy:.+true' || exit 1",
				},
			}},
		},
		{
			Identity:            providerIdentity("local-object-store"),
			Type:                "datascape.dev/store",
			Capabilities:        []string{"datascape.dev/store.object", "datascape.dev/audit.record"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "stores.datascape.dev/v1alpha1", Kind: "ObjectStore"}, {APIVersion: "audit.datascape.dev/v1alpha1", Kind: "AuditStore"}},
			BindingKinds:        []string{"AuditBinding", "Binding", "StreamArchiveBinding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			Conformance:         []string{"ARCHIVE-001"},
			Services: []Service{{
				Name:        "object-store",
				Capability:  "datascape.dev/store.object",
				Image:       "minio/minio:RELEASE.2024-05-10T01-41-38Z",
				Command:     []string{"server", "/data", "--console-address", ":9001"},
				Ports:       []string{"9000:9000", "9001:9001"},
				Environment: map[string]string{"MINIO_ROOT_PASSWORD": "${MINIO_ROOT_PASSWORD:?set MINIO_ROOT_PASSWORD}", "MINIO_ROOT_USER": "${MINIO_ROOT_USER:-datascape}"},
				Volumes:     []string{"object-store-data:/data"},
				Healthcheck: []string{"CMD-SHELL", "curl -fsS http://localhost:9000/minio/health/live || exit 1"},
			}},
		},
		{
			Identity:            providerIdentity("local-lineage"),
			Type:                "datascape.dev/lineage",
			Capabilities:        []string{"datascape.dev/lineage.admit"},
			ResourceKinds:       []resource.KindRef{{APIVersion: "lineage.datascape.dev/v1alpha1", Kind: "LineageSink"}},
			BindingKinds:        []string{"Binding", "LineageBinding"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			Conformance:         []string{"LINEAGE-001"},
			Services: []Service{{
				Name:       "lineage-admission",
				Capability: "datascape.dev/lineage.admit",
				Image:      "alpine:3.20",
				Command:    []string{"sh", "-c", "mkdir -p /www && echo ok > /www/healthz && httpd -f -p 8080 -h /www"},
				Ports:      []string{"8088:8080"},
				Volumes:    []string{"lineage-journal:/journal", "lineage-quarantine:/quarantine"},
				Healthcheck: []string{
					"CMD-SHELL",
					"wget -qO- http://localhost:8080/healthz | grep ok",
				},
			}},
		},
		{
			Identity:            providerIdentity("local-utility"),
			Type:                "datascape.dev/runtime",
			Capabilities:        []string{"datascape.dev/runtime.utility"},
			TargetCompatibility: []string{"compose"},
			RendererContract:    "datascape.dev/provider-plan/v1alpha1",
			Services: []Service{{
				Name:       "runtime-utility",
				Capability: "datascape.dev/runtime.utility",
				Image:      "alpine:3.20",
				Command:    []string{"sh", "-c", "sleep infinity"},
			}},
		},
	}
}

func BuiltinInstances(target string) []Instance {
	if target == "" {
		target = "compose"
	}
	out := make([]Instance, 0, len(BuiltinDescriptors()))
	for _, descriptor := range BuiltinDescriptors() {
		name := descriptor.Identity.Name
		out = append(out, Instance{
			Identity:     domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "ProviderInstance", Namespace: api.DefaultNamespace, Name: name, Target: target},
			Provider:     descriptor.Identity,
			Type:         descriptor.Type,
			Target:       target,
			Capabilities: descriptor.Capabilities,
			Parameters:   map[string]any{"builtin": true},
		})
	}
	return out
}

func descriptorFromResource(res spec.Resource) (Descriptor, error) {
	body, err := specBody(res)
	if err != nil {
		return Descriptor{}, err
	}
	descriptor := Descriptor{
		Identity:            res.Identity("", ""),
		Type:                stringFieldDefault(body, "type", "datascape.dev/provider"),
		Capabilities:        stringSlice(body["capabilities"]),
		BindingKinds:        stringSlice(body["bindingKinds"]),
		TargetCompatibility: stringSlice(first(body, "targetCompatibility", "targets")),
		RuntimeDependencies: stringSlice(body["runtimeDependencies"]),
		RendererContract:    stringField(body, "rendererContract"),
		Conformance:         stringSlice(body["conformance"]),
		PackageVersion:      stringField(body, "packageVersion"),
		ContractVersion:     stringFieldDefault(body, "contractVersion", "v1alpha1"),
		PackageDigest:       stringField(body, "packageDigest"),
		Provenance:          stringField(body, "provenance"),
		Services:            servicesFromValue(body["services"]),
		Artifacts:           artifactsFromValue(body["artifacts"]),
	}
	if len(descriptor.Capabilities) == 0 {
		return Descriptor{}, fmt.Errorf("provider must declare at least one capability")
	}
	return descriptor, nil
}

func instanceFromResource(res spec.Resource, target string) (Instance, error) {
	body, err := specBody(res)
	if err != nil {
		return Instance{}, err
	}
	ref := stringField(body, "providerRef")
	if ref == "" {
		return Instance{}, fmt.Errorf("provider instance must declare providerRef")
	}
	providerID := parseRef(ref, res, "Provider")
	instanceTarget := stringFieldDefault(body, "target", target)
	features := stringSlice(body["capabilities"])
	if len(features) == 0 {
		if descriptorName := providerID.Name; descriptorName != "" {
			features = stringSlice(body["enabledCapabilities"])
		}
	}
	parameters, _ := body["parameters"].(map[string]any)
	return Instance{Identity: res.Identity(instanceTarget, ""), Provider: providerID, Type: stringField(body, "type"), Target: instanceTarget, Capabilities: features, Parameters: parameters}, nil
}

func targetCompatible(descriptor Descriptor, target string) bool {
	if target == "" || len(descriptor.TargetCompatibility) == 0 {
		return true
	}
	return contains(descriptor.TargetCompatibility, target)
}

func providerIdentity(name string) domain.ResourceIdentity {
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "Provider", Namespace: api.DefaultNamespace, Name: name}
}

func parseRef(value string, owner spec.Resource, expectedKind string) domain.ResourceIdentity {
	parts := splitRef(value)
	ns := owner.Metadata.Namespace
	if ns == "" {
		ns = api.DefaultNamespace
	}
	kind := expectedKind
	name := value
	if len(parts) == 2 {
		kind, name = parts[0], parts[1]
	}
	if len(parts) == 3 {
		kind, ns, name = parts[0], parts[1], parts[2]
	}
	if len(parts) == 5 {
		kind, ns, name = parts[2], parts[3], parts[4]
	}
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: kind, Namespace: ns, Name: name}
}

func splitRef(value string) []string {
	out := make([]string, 0)
	for _, part := range bytes.Split([]byte(value), []byte("/")) {
		out = append(out, string(part))
	}
	return out
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

func first(body map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := body[key]; ok {
			return value
		}
	}
	return nil
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

func servicesFromValue(value any) []Service {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]Service, 0, len(values))
	for _, value := range values {
		body, ok := value.(map[string]any)
		if !ok {
			continue
		}
		env := map[string]string{}
		if raw, ok := body["environment"].(map[string]any); ok {
			for key, value := range raw {
				if s, ok := value.(string); ok {
					env[key] = s
				}
			}
		}
		out = append(out, Service{
			Name:               stringField(body, "name"),
			Capability:         stringField(body, "capability"),
			Image:              stringField(body, "image"),
			Command:            orderedStringSlice(body["command"]),
			Ports:              stringSlice(body["ports"]),
			Environment:        env,
			Volumes:            stringSlice(body["volumes"]),
			DependsOn:          stringSlice(body["dependsOn"]),
			DependsOnCompleted: stringSlice(body["dependsOnCompleted"]),
			Healthcheck:        orderedStringSlice(body["healthcheck"]),
			Restart:            stringField(body, "restart"),
			User:               stringField(body, "user"),
			ReadOnly:           boolField(body, "readOnly"),
			Init:               boolField(body, "init"),
			CapDrop:            stringSlice(body["capDrop"]),
			SecurityOpt:        stringSlice(body["securityOpt"]),
			Tmpfs:              stringSlice(body["tmpfs"]),
			Secrets:            stringSlice(body["secrets"]),
			Configs:            stringSlice(body["configs"]),
			Profiles:           stringSlice(body["profiles"]),
			StopGracePeriod:    stringField(body, "stopGracePeriod"),
			CPUs:               stringField(body, "cpus"),
			Memory:             stringField(body, "memory"),
			PidsLimit:          intField(body, "pidsLimit"),
		})
	}
	return out
}

func intField(body map[string]any, key string) int {
	switch value := body[key].(type) {
	case json.Number:
		n, _ := value.Int64()
		return int(n)
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func artifactsFromValue(value any) []Artifact {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]Artifact, 0, len(values))
	for _, value := range values {
		body, ok := value.(map[string]any)
		if !ok {
			continue
		}
		content, _ := body["content"].(map[string]any)
		out = append(out, Artifact{Path: stringField(body, "path"), Capability: stringField(body, "capability"), Content: content})
	}
	return out
}

func orderedStringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if item, ok := value.(string); ok && item != "" {
			out = append(out, item)
		}
	}
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

func diag(res spec.Resource, code, field, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{Severity: domain.SeverityError, Code: code, Resource: res.Identity("", "").Display(), FieldPath: field, Message: message, Remediation: remediation, Location: res.Location}
}

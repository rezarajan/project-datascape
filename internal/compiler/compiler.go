package compiler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"

	composeadapter "datascape.dev/platformctl/internal/adapters/targets/compose"
	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/binding"
	"datascape.dev/platformctl/internal/canonical"
	"datascape.dev/platformctl/internal/conformance"
	"datascape.dev/platformctl/internal/docsgen"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/hash"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/planner"
	"datascape.dev/platformctl/internal/provenance"
	"datascape.dev/platformctl/internal/provider"
	"datascape.dev/platformctl/internal/recovery"
	"datascape.dev/platformctl/internal/render"
	"datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
	"datascape.dev/platformctl/internal/validation"
)

const DefaultVersion = "0.1.0-dev"

type Options struct {
	Target            string
	CompilerVersion   string
	CompilerDigest    string
	SourceCommit      string
	SourceDateEpoch   string
	ContinueOnWarning bool
}

type Result struct {
	Plan         ir.PlatformPlan     `json:"plan"`
	Actions      []ir.ChangeAction   `json:"actions"`
	Files        []artifact.File     `json:"-"`
	Diagnostics  []domain.Diagnostic `json:"diagnostics"`
	Passes       []PassResult        `json:"passes"`
	Provenance   provenance.Record   `json:"provenance"`
	BundleDigest string              `json:"bundleDigest"`
	Resources    []ir.ResourcePlan   `json:"resources"`
}

type PassResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

var passNames = []string{
	"parse",
	"schema validation",
	"semantic validation",
	"definition registration",
	"reference resolution",
	"provider registration",
	"binding resolution",
	"policy enforcement",
	"normalization",
	"dependency graph construction",
	"execution-plan derivation",
	"target planning",
	"artifact rendering",
	"canonicalization",
	"per-resource hashing",
	"bundle hashing",
	"provenance generation",
	"documentation generation",
	"conformance-test generation",
	"recovery-artifact generation",
}

func PassNames() []string {
	out := make([]string, len(passNames))
	copy(out, passNames)
	return out
}

func CompileDocuments(ctx context.Context, docs []spec.NamedDocument, opts Options) Result {
	opts = normalizeOptions(opts)
	result := Result{Passes: makePasses("pending")}

	resources, parseDiags := spec.ParseDocuments(ctx, docs)
	result.Diagnostics = append(result.Diagnostics, parseDiags...)
	result.markPass("parse", statusFromDiagnostics(parseDiags))
	if domain.HasErrors(result.Diagnostics) {
		result.skipRemaining()
		validation.SortDiagnostics(result.Diagnostics)
		return result
	}

	validateDiags := validation.ValidateResources(ctx, resources)
	result.Diagnostics = append(result.Diagnostics, validateDiags...)
	result.markPass("schema validation", statusFromDiagnostics(validateDiags))
	result.markPass("semantic validation", statusFromDiagnostics(validateDiags))
	if domain.HasErrors(result.Diagnostics) {
		result.skipRemaining()
		validation.SortDiagnostics(result.Diagnostics)
		return result
	}

	target := opts.Target
	if target == "" {
		target = inferTarget(resources)
	}
	result.markPass("definition registration", "ok")
	result.markPass("reference resolution", "ok")
	result.markPass("provider registration", "ok")
	result.markPass("binding resolution", "ok")
	result.markPass("policy enforcement", "ok")
	result.markPass("normalization", "ok")
	result.markPass("dependency graph construction", "ok")
	result.markPass("execution-plan derivation", "ok")
	result.markPass("target planning", "ok")

	plan, irDiags := BuildPlan(ctx, resources, target)
	result.Diagnostics = append(result.Diagnostics, irDiags...)
	result.Plan = plan
	result.Resources = plan.Resources
	if domain.HasErrors(result.Diagnostics) {
		result.skipRemaining()
		validation.SortDiagnostics(result.Diagnostics)
		return result
	}
	result.Actions = planner.AddPlan(plan)
	result.markPass("per-resource hashing", "ok")

	files, err := renderArtifacts(ctx, docs, plan, opts)
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, domain.Diagnostic{
			Severity:    domain.SeverityError,
			Code:        "DART001",
			Message:     err.Error(),
			Remediation: "inspect artifact rendering inputs and retry",
		})
		result.markPass("artifact rendering", "failed")
		result.skipRemaining()
		validation.SortDiagnostics(result.Diagnostics)
		return result
	}
	result.markPass("artifact rendering", "ok")
	result.markPass("canonicalization", "ok")
	result.markPass("documentation generation", "ok")
	result.markPass("conformance-test generation", "ok")
	result.markPass("recovery-artifact generation", "ok")
	result.markPass("bundle hashing", "ok")
	result.markPass("provenance generation", "ok")
	result.Files = files
	result.BundleDigest = artifact.BundleDigest(files)
	result.Provenance = extractProvenance(files)
	validation.SortDiagnostics(result.Diagnostics)
	return result
}

func BuildPlan(ctx context.Context, resources []spec.Resource, target string) (ir.PlatformPlan, []domain.Diagnostic) {
	plan := ir.PlatformPlan{APIVersion: spec.APIVersionV1Alpha1, Target: target}
	if err := ctx.Err(); err != nil {
		return plan, []domain.Diagnostic{{Severity: domain.SeverityError, Code: "DCOMP001", Message: err.Error()}}
	}
	definitions, definitionDiags := resource.BuildRegistry(resources)
	providers, providerDiags := provider.BuildRegistry(resources, target)
	bindingDefinitions, bindingDefinitionDiags := binding.BuildRegistry(resources)
	resolvedBindings, bindingDiags := binding.Resolve(resources, bindingDefinitions, definitions, providers, target)
	graph, graphDiags := buildResourceGraph(resources, target, definitions, resolvedBindings)
	resourcePlans, diags := hashResources(ctx, resources, target)
	resourcePlans = attachGraphOverrides(resourcePlans, graph.Overrides)
	diags = append(diags, definitionDiags...)
	diags = append(diags, providerDiags...)
	diags = append(diags, bindingDefinitionDiags...)
	diags = append(diags, bindingDiags...)
	diags = append(diags, graphDiags...)
	plan.Resources = resourcePlans
	plan.ResourceGraph = graph
	plan.Bindings = graph.Bindings
	plan.External = graph.External
	plan.Policies = graph.Policies
	plan.Overrides = graph.Overrides
	plan.Identity = planIdentity(resources, target)
	plan.TargetPlan = buildTargetPlan(resources, target)
	plan.Definitions = definitionPlans(definitions)
	plan.Providers = providerPlans(providers)
	plan.ProviderInstances = providerInstancePlans(providers)
	planned, plannedDiags := plannedProviderResources(resources, resolvedBindings, definitions, providers, target)
	diags = append(diags, plannedDiags...)
	plan.PlannedResources = planned
	diags = append(diags, validateComposeProduction(plan.TargetPlan, planned, plan.Providers)...)
	storagePlan, storageDiags := buildStoragePlan(resources, target)
	plan.Storage = storagePlan
	diags = append(diags, storageDiags...)
	plan.Verification = buildVerification(resources, graph, planned, target)
	plan.Recovery = buildRecoveryPlan(graph, planned, storagePlan)
	return plan, diags
}

func validateComposeProduction(target ir.TargetPlan, planned []ir.ProviderResourcePlan, providers []ir.ProviderPlan) []domain.Diagnostic {
	if target.Type != "compose" || target.DevelopmentMode || target.AllowUnpinnedImages || (target.AvailabilityClass != "single-host-production" && target.AvailabilityClass != "production") {
		return nil
	}
	diags := make([]domain.Diagnostic, 0)
	providerByID := map[string]ir.ProviderPlan{}
	for _, descriptor := range providers {
		providerByID[descriptor.Identity.CanonicalString()] = descriptor
	}
	for _, resource := range planned {
		if descriptor, ok := providerByID[resource.Provider.CanonicalString()]; ok && descriptor.PackageVersion != "builtin" && !strings.HasPrefix(descriptor.PackageDigest, "sha256:") {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DPROV009", Resource: resource.Provider.Display(), FieldPath: "spec.packageDigest", Message: "production provider package is not checksummed", Remediation: "set packageDigest to the verified sha256 digest of the provider package"})
		}
		for _, service := range resource.Services {
			if !strings.Contains(service.Image, "@sha256:") {
				diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCOMPOSE002", Resource: resource.Identity.Display(), FieldPath: "services.image", Message: "production Compose image is not pinned by digest: " + service.Image, Remediation: "pin the provider service image as repository:version@sha256:digest"})
			}
			for _, port := range service.Ports {
				if !strings.HasPrefix(port, "127.0.0.1:") {
					diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCOMPOSE003", Resource: resource.Identity.Display(), FieldPath: "services.ports", Message: "production Compose port is not bound to localhost: " + port, Remediation: "bind the published port to 127.0.0.1 or remove host exposure"})
				}
			}
		}
	}
	return diags
}

func hashResources(ctx context.Context, resources []spec.Resource, target string) ([]ir.ResourcePlan, []domain.Diagnostic) {
	type job struct {
		index    int
		resource spec.Resource
	}
	type result struct {
		index int
		plan  ir.ResourcePlan
		diags []domain.Diagnostic
	}
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > 4 {
		workers = 4
	}
	jobs := make(chan job)
	results := make(chan result, len(resources))
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				plan, diags := resourcePlan(item.resource, target)
				results <- result{index: item.index, plan: plan, diags: diags}
			}
		}()
	}
	for i, resource := range resources {
		if ctx.Err() != nil {
			break
		}
		jobs <- job{index: i, resource: resource}
	}
	close(jobs)
	wg.Wait()
	close(results)

	ordered := make([]result, 0, len(resources))
	for result := range results {
		ordered = append(ordered, result)
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].index < ordered[j].index })
	plans := make([]ir.ResourcePlan, 0, len(ordered))
	diags := make([]domain.Diagnostic, 0)
	for _, result := range ordered {
		plans = append(plans, result.plan)
		diags = append(diags, result.diags...)
	}
	if err := ctx.Err(); err != nil {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCOMP001", Message: err.Error()})
	}
	sort.SliceStable(plans, func(i, j int) bool {
		return plans[i].Identity.CanonicalString() < plans[j].Identity.CanonicalString()
	})
	return plans, diags
}

func resourcePlan(resource spec.Resource, target string) (ir.ResourcePlan, []domain.Diagnostic) {
	material, err := canonicalResourceMaterial(resource)
	if err != nil {
		return ir.ResourcePlan{}, []domain.Diagnostic{{
			Severity:    domain.SeverityError,
			Code:        "DIR001",
			Resource:    resource.Identity(target, "foundation").Display(),
			FieldPath:   "spec",
			Message:     err.Error(),
			Remediation: "provide canonicalizable JSON spec content",
			Location:    resource.Location,
		}}
	}
	digest, err := hash.Canonical(material)
	if err != nil {
		return ir.ResourcePlan{}, []domain.Diagnostic{{
			Severity: domain.SeverityError,
			Code:     "DHASH001",
			Resource: resource.Identity(target, "foundation").Display(),
			Message:  err.Error(),
			Location: resource.Location,
		}}
	}
	dependencies := dependencies(resource, target)
	sort.SliceStable(dependencies, func(i, j int) bool {
		return dependencies[i].CanonicalString() < dependencies[j].CanonicalString()
	})
	source := resource.SourceName()
	return ir.ResourcePlan{
		Identity:               resource.Identity(target, "foundation"),
		Kind:                   resource.Kind,
		Dependencies:           dependencies,
		SourceDeclarations:     []string{source},
		Adapter:                "foundation",
		GeneratedFiles:         generatedFilesForResource(resource, target),
		CanonicalDigest:        digest,
		RolloutSensitiveDigest: rolloutDigest(resource, digest),
		RecoveryClassification: recoveryClass(resource.Kind),
		SecretBackend:          secretBackend(resource),
		SecretKeys:             secretKeys(resource),
		Ownership:              resourceOwnership(resource, bodyForResourcePlan(resource)),
		Lifecycle:              resourceLifecycle(bodyForResourcePlan(resource)),
		GraphState:             graphState(resourceOwnership(resource, bodyForResourcePlan(resource)), resourceLifecycle(bodyForResourcePlan(resource))),
		Overrides:              resourceOverridesForPlan(resource, target),
	}, nil
}

func bodyForResourcePlan(resource spec.Resource) map[string]any {
	body, ok := resourceBody(resource)
	if !ok {
		return map[string]any{}
	}
	return body
}

func canonicalResourceMaterial(resource spec.Resource) (map[string]any, error) {
	var specBody any
	dec := json.NewDecoder(bytes.NewReader(resource.Spec))
	dec.UseNumber()
	if err := dec.Decode(&specBody); err != nil {
		return nil, err
	}
	return map[string]any{
		"apiVersion": resource.APIVersion,
		"kind":       resource.Kind,
		"metadata": map[string]any{
			"name":        resource.Metadata.Name,
			"namespace":   defaultNamespace(resource.Metadata.Namespace),
			"labels":      stableStringMap(resource.Metadata.Labels),
			"annotations": stableStringMap(resource.Metadata.Annotations),
		},
		"spec": specBody,
	}, nil
}

func dependencies(resource spec.Resource, target string) []domain.ResourceIdentity {
	var body map[string]any
	dec := json.NewDecoder(bytes.NewReader(resource.Spec))
	dec.UseNumber()
	if err := dec.Decode(&body); err != nil {
		return nil
	}
	refs := make([]domain.ResourceIdentity, 0)
	if values, ok := body["dependsOn"].([]any); ok {
		for _, value := range values {
			if ref, ok := value.(string); ok {
				refs = append(refs, parseReference(ref, resource, target))
			}
		}
	}
	for key, value := range body {
		if strings.HasSuffix(key, "Ref") {
			if ref, ok := value.(string); ok {
				refs = append(refs, parseReference(ref, resource, target))
			}
		}
	}
	return refs
}

func parseReference(ref string, owner spec.Resource, target string) domain.ResourceIdentity {
	parts := strings.Split(ref, "/")
	ns := defaultNamespace(owner.Metadata.Namespace)
	kind := "Resource"
	name := ref
	if len(parts) == 2 {
		kind, name = parts[0], parts[1]
	}
	if len(parts) == 3 {
		kind, ns, name = parts[0], parts[1], parts[2]
	}
	apiVersion := apiVersionForKind(kind)
	if len(parts) == 5 {
		apiVersion = parts[0] + "/" + parts[1]
		kind, ns, name = parts[2], parts[3], parts[4]
	}
	return domain.ResourceIdentity{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name, Target: target, Adapter: "foundation"}
}

func apiVersionForKind(kind string) string {
	switch kind {
	case "RelationalSource", "EventProducer":
		return "sources.datascape.dev/v1alpha1"
	case "EventStream":
		return "streams.datascape.dev/v1alpha1"
	case "EventContract":
		return "contracts.datascape.dev/v1alpha1"
	case "DatabaseConnection", "ObjectStoreConnection", "EventStreamConnection":
		return "connections.datascape.dev/v1alpha1"
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
	case "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding":
		return "bindings.datascape.dev/v1alpha1"
	default:
		return spec.APIVersionV1Alpha1
	}
}

func rolloutDigest(resource spec.Resource, canonicalDigest string) string {
	switch resource.Kind {
	case "RuntimeProfile", "Provider", "ProviderInstance", "ResourceDefinition", "BindingDefinition", "PlatformPolicy", "Target":
		return ""
	default:
		material, err := canonicalResourceMaterial(resource)
		if err != nil {
			return canonicalDigest
		}
		if metadata, ok := material["metadata"].(map[string]any); ok {
			delete(metadata, "labels")
			delete(metadata, "annotations")
		}
		digest, err := hash.Canonical(material)
		if err != nil {
			return canonicalDigest
		}
		return digest
	}
}

func recoveryClass(kind string) string {
	switch kind {
	case "ObjectStore", "AuditStore", "SecretReference":
		return "authoritative"
	case "Table", "Warehouse":
		return "derived"
	default:
		return "configuration"
	}
}

func secretBackend(resource spec.Resource) string {
	if resource.APIVersion != spec.APIVersionV1Alpha1 || resource.Kind != "SecretReference" {
		return ""
	}
	body := bodyForResourcePlan(resource)
	value, _ := body["backend"].(string)
	return value
}

func secretKeys(resource spec.Resource) []string {
	if resource.APIVersion != spec.APIVersionV1Alpha1 || resource.Kind != "SecretReference" {
		return nil
	}
	body := bodyForResourcePlan(resource)
	values, ok := body["keys"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func generatedFilesForResource(resource spec.Resource, target string) []string {
	files := []string{"plan.json", "resources.json", "resource-graph.json"}
	switch resource.Kind {
	case "Binding", "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding", "BatchIngestBinding", "StreamIngestBinding", "TransformBinding", "VolumeMountBinding":
		files = append(files, "configuration/bindings/"+resource.Metadata.Name+".json", "verification/checks.json")
	case "StorageClass", "PersistentVolume", "PersistentVolumeClaim":
		files = append(files, "storage/plan.json")
	case "Provider", "ProviderInstance":
		files = append(files, "providers/providers.json", "providers/instances.json")
	case "ResourceDefinition", "BindingDefinition":
		files = append(files, "definitions/resources.json", "definitions/bindings.json")
	case "RuntimeProfile", "Target":
		if target == "compose" {
			files = append(files, "compose.yaml", ".env.example")
		}
	}
	sort.Strings(files)
	return files
}

func adapterVersions(plan ir.PlatformPlan, compilerVersion string) map[string]string {
	out := map[string]string{"compiler": compilerVersion}
	for _, provider := range plan.Providers {
		out[provider.Identity.Name] = provider.RendererContract
	}
	return out
}

func renderArtifacts(ctx context.Context, docs []spec.NamedDocument, plan ir.PlatformPlan, opts Options) ([]artifact.File, error) {
	var files []artifact.File
	var err error
	if plan.TargetPlan.Type == "compose" {
		files, err = composeadapter.Render(ctx, plan)
		if err != nil {
			return nil, err
		}
	} else {
		files, err = render.FoundationFiles(ctx, plan)
		if err != nil {
			return nil, err
		}
	}
	docsFiles := docsgen.FoundationDocs()
	files = append(files, docsFiles...)

	schema, err := SchemaFile()
	if err != nil {
		return nil, err
	}
	files = append(files, schema)

	conformanceJSON, err := canonical.JSON(conformance.FoundationSuites())
	if err != nil {
		return nil, err
	}
	files = append(files, artifact.File{Path: "verification/conformance-plan.json", Mode: 0o644, Content: append(conformanceJSON, '\n'), Deterministic: true})

	recoveryJSON, err := canonical.JSON(recovery.FoundationPlan())
	if err != nil {
		return nil, err
	}
	files = append(files, artifact.File{Path: "recovery/recovery-plan.json", Mode: 0o644, Content: append(recoveryJSON, '\n'), Deterministic: true})

	payloadDigest := artifact.BundleDigest(files)
	prov := provenance.Record{
		PlatformSpecificationDigest: sourceDigest(docs, ""),
		RuntimeProfileDigest:        sourceDigestByKind(docs, "RuntimeProfile"),
		ProviderSetDigest:           sourceDigestByKind(docs, "Provider"),
		CompilerVersion:             opts.CompilerVersion,
		CompilerBinaryDigest:        opts.CompilerDigest,
		SourceCommit:                opts.SourceCommit,
		Target:                      plan.Target,
		AdapterVersions:             adapterVersions(plan, opts.CompilerVersion),
		BundleDigest:                payloadDigest,
		SourceDateEpoch:             opts.SourceDateEpoch,
	}
	provJSON, err := canonical.JSON(prov)
	if err != nil {
		return nil, err
	}
	files = append(files, artifact.File{Path: "provenance.json", Mode: 0o644, Content: append(provJSON, '\n'), Deterministic: true})

	manifestJSON, err := canonical.JSON(artifact.BuildManifest(files, payloadDigest))
	if err != nil {
		return nil, err
	}
	files = append(files, artifact.File{Path: "bundle.manifest.json", Mode: 0o644, Content: append(manifestJSON, '\n'), Deterministic: true})
	files = append(files, artifact.File{Path: "checksums.txt", Mode: 0o644, Content: artifact.Checksums(files), Deterministic: true})
	return artifact.Normalize(files), nil
}

func SchemaFile() (artifact.File, error) {
	schema := map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  "https://platform.datascape.dev/schemas/v1alpha1/resource.schema.json",
		"title":                "Datascape platform resource",
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"apiVersion", "kind", "metadata"},
		"properties": map[string]any{
			"apiVersion": map[string]any{"type": "string", "minLength": 1},
			"kind":       map[string]any{"type": "string", "minLength": 1},
			"metadata": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []string{"name"},
				"properties": map[string]any{
					"name":        map[string]any{"type": "string", "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"},
					"namespace":   map[string]any{"type": "string", "pattern": "^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"},
					"labels":      map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"annotations": map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
				},
			},
			"spec":   map[string]any{"type": "object"},
			"status": map[string]any{"type": "object"},
		},
	}
	content, err := canonical.JSON(schema)
	if err != nil {
		return artifact.File{}, err
	}
	return artifact.File{Path: "schemas/platform.datascape.dev_v1alpha1.schema.json", Mode: 0o644, Content: append(content, '\n'), Deterministic: true}, nil
}

func sourceDigest(docs []spec.NamedDocument, kind string) string {
	type entry struct {
		name    string
		content []byte
	}
	entries := make([]entry, 0, len(docs))
	for _, doc := range docs {
		if kind != "" && !documentContainsKind(doc, kind) {
			continue
		}
		entries = append(entries, entry{name: doc.Name, content: canonical.NormalizeText(doc.Content)})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].name < entries[j].name })
	var buf bytes.Buffer
	for _, entry := range entries {
		buf.WriteString(entry.name)
		buf.WriteByte('\n')
		buf.Write(entry.content)
		if !bytes.HasSuffix(entry.content, []byte("\n")) {
			buf.WriteByte('\n')
		}
	}
	if len(entries) == 0 {
		return ""
	}
	return hash.Bytes(buf.Bytes())
}

func sourceDigestByKind(docs []spec.NamedDocument, kind string) string {
	return sourceDigest(docs, kind)
}

func documentContainsKind(doc spec.NamedDocument, kind string) bool {
	resources, diags := spec.ParseDocuments(context.Background(), []spec.NamedDocument{doc})
	if domain.HasErrors(diags) {
		return false
	}
	for _, resource := range resources {
		if resource.Kind == kind {
			return true
		}
	}
	return false
}

func extractProvenance(files []artifact.File) provenance.Record {
	for _, file := range files {
		if file.Path != "provenance.json" {
			continue
		}
		var record provenance.Record
		_ = json.Unmarshal(file.Content, &record)
		return record
	}
	return provenance.Record{}
}

func normalizeOptions(opts Options) Options {
	if opts.CompilerVersion == "" {
		opts.CompilerVersion = DefaultVersion
	}
	return opts
}

func inferTarget(resources []spec.Resource) string {
	for _, resource := range resources {
		if resource.Kind != "Target" {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(resource.Spec, &body); err != nil {
			continue
		}
		if target, ok := body["type"].(string); ok && target != "" {
			return target
		}
	}
	for _, resource := range resources {
		if resource.Kind != "RuntimeProfile" {
			continue
		}
		var body map[string]any
		if err := json.Unmarshal(resource.Spec, &body); err != nil {
			continue
		}
		if target, ok := body["target"].(string); ok && target != "" {
			return target
		}
	}
	return "local"
}

func makePasses(status string) []PassResult {
	passes := make([]PassResult, 0, len(passNames))
	for _, name := range passNames {
		passes = append(passes, PassResult{Name: name, Status: status})
	}
	return passes
}

func (r *Result) markPass(name, status string) {
	for i := range r.Passes {
		if r.Passes[i].Name == name {
			r.Passes[i].Status = status
			return
		}
	}
}

func (r *Result) skipRemaining() {
	for i := range r.Passes {
		if r.Passes[i].Status == "pending" {
			r.Passes[i].Status = "skipped"
		}
	}
}

func statusFromDiagnostics(diags []domain.Diagnostic) string {
	if domain.HasErrors(diags) {
		return "failed"
	}
	return "ok"
}

func defaultNamespace(namespace string) string {
	if namespace == "" {
		return "default"
	}
	return namespace
}

func stableStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func FormatActions(actions []ir.ChangeAction) string {
	var b strings.Builder
	for _, action := range actions {
		fmt.Fprintln(&b, action.Message)
	}
	return b.String()
}

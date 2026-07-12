package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/spec"
)

type KindRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
}

func (r KindRef) Key() string {
	return r.APIVersion + "/" + r.Kind
}

func (r KindRef) String() string {
	return r.Key()
}

type Definition struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	APIVersion   string                  `json:"apiVersion"`
	Kind         string                  `json:"kind"`
	Plural       string                  `json:"plural,omitempty"`
	Scope        string                  `json:"scope"`
	Category     string                  `json:"category,omitempty"`
	ProviderType string                  `json:"providerType,omitempty"`
	Capabilities []string                `json:"capabilities,omitempty"`
	BindingRoles []string                `json:"bindingRoles,omitempty"`
	Schema       map[string]any          `json:"schema,omitempty"`
	Core         bool                    `json:"core,omitempty"`
	Extension    bool                    `json:"extension,omitempty"`
}

func (d Definition) KindRef() KindRef {
	return KindRef{APIVersion: d.APIVersion, Kind: d.Kind}
}

type Registry struct {
	byKey map[string]Definition
}

func NewRegistry() *Registry {
	return &Registry{byKey: map[string]Definition{}}
}

func (r *Registry) Register(def Definition) error {
	if def.APIVersion == "" || def.Kind == "" {
		return fmt.Errorf("resource definition must include apiVersion and kind")
	}
	if def.Scope == "" {
		def.Scope = "Namespaced"
	}
	key := def.KindRef().Key()
	if prior, ok := r.byKey[key]; ok {
		return fmt.Errorf("duplicate resource definition for %s first declared by %s", key, prior.Identity.Display())
	}
	r.byKey[key] = normalizeDefinition(def)
	return nil
}

func (r *Registry) Lookup(apiVersion, kind string) (Definition, bool) {
	def, ok := r.byKey[KindRef{APIVersion: apiVersion, Kind: kind}.Key()]
	return def, ok
}

func (r *Registry) Definitions() []Definition {
	out := make([]Definition, 0, len(r.byKey))
	for _, def := range r.byKey {
		out = append(out, def)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].APIVersion != out[j].APIVersion {
			return out[i].APIVersion < out[j].APIVersion
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func BuiltinDefinitions() []Definition {
	core := []struct {
		kind      string
		category  string
		extension bool
	}{
		{kind: "Binding", category: "extension", extension: true},
		{kind: "BindingDefinition", category: "extension", extension: true},
		{kind: "PlatformPolicy", category: "policy"},
		{kind: "Provider", category: "extension", extension: true},
		{kind: "ProviderInstance", category: "extension", extension: true},
		{kind: "ResourceDefinition", category: "extension", extension: true},
		{kind: "RuntimeProfile", category: "target"},
		{kind: "SecretReference", category: "connection"},
		{kind: "Target", category: "target"},
	}
	defs := make([]Definition, 0, len(core)+45)
	for _, item := range core {
		defs = append(defs, Definition{
			Identity:   builtinIdentity("ResourceDefinition", strings.ToLower(item.kind)),
			APIVersion: api.PlatformV1Alpha1,
			Kind:       item.kind,
			Plural:     strings.ToLower(item.kind) + "s",
			Scope:      "Namespaced",
			Category:   item.category,
			Core:       true,
			Extension:  item.extension,
		})
	}
	defs = append(defs,
		Definition{Identity: builtinIdentity("ResourceDefinition", "storage-class"), APIVersion: "storage.datascape.dev/v1alpha1", Kind: "StorageClass", Plural: "storageclasses", Scope: "Cluster", Category: "class", ProviderType: "datascape.dev/storage", Schema: objectSchema([]string{"provisioner"}, map[string]string{"provisioner": "string", "targetCompatibility": "array", "parameters": "object", "reclaimPolicy": "string", "volumeBindingMode": "string", "allowVolumeExpansion": "boolean", "default": "boolean"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "persistent-volume"), APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolume", Plural: "persistentvolumes", Scope: "Cluster", Category: "storage", ProviderType: "datascape.dev/storage", BindingRoles: []string{"source"}, Schema: objectSchema([]string{"storageClassRef", "capacity", "accessModes"}, map[string]string{"storageClassRef": "string", "capacity": "string", "accessModes": "array", "ownership": "string", "source": "object"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "persistent-volume-claim"), APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolumeClaim", Plural: "persistentvolumeclaims", Scope: "Namespaced", Category: "storage", ProviderType: "datascape.dev/storage", BindingRoles: []string{"source"}, Schema: objectSchema([]string{"capacity", "accessModes"}, map[string]string{"storageClassRef": "string", "volumeRef": "string", "capacity": "string", "accessModes": "array"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "database-class"), APIVersion: "databases.datascape.dev/v1alpha1", Kind: "DatabaseClass", Plural: "databaseclasses", Scope: "Cluster", Category: "class", ProviderType: "datascape.dev/database", Schema: objectSchema([]string{"engine", "providerInstanceRef"}, map[string]string{"engine": "string", "version": "string", "providerInstanceRef": "string", "supportedConnectorClasses": "array", "storage": "object", "parameters": "object"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "connector-class"), APIVersion: "connections.datascape.dev/v1alpha1", Kind: "ConnectorClass", Plural: "connectorclasses", Scope: "Cluster", Category: "class", ProviderType: "datascape.dev/connector", Schema: objectSchema([]string{"interface", "transport"}, map[string]string{"interface": "string", "transport": "string", "driver": "string", "operations": "array", "compatibleEngines": "array", "providerInstanceRef": "string", "targetCompatibility": "array", "parameters": "object"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "database-instance"), APIVersion: "databases.datascape.dev/v1alpha1", Kind: "DatabaseInstance", Plural: "databaseinstances", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/database", BindingRoles: []string{"source", "target"}, Schema: objectSchema([]string{"classRef"}, map[string]string{"classRef": "string", "storageClaimRef": "string", "credentialsRef": "string", "ownership": "string", "database": "string", "parameters": "object"})},
		Definition{Identity: builtinIdentity("ResourceDefinition", "relational-source"), APIVersion: "sources.datascape.dev/v1alpha1", Kind: "RelationalSource", Plural: "relationalsources", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/source", Capabilities: []string{"datascape.dev/source.relational"}, BindingRoles: []string{"source"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "event-producer"), APIVersion: "sources.datascape.dev/v1alpha1", Kind: "EventProducer", Plural: "eventproducers", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/source", BindingRoles: []string{"source"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "event-stream"), APIVersion: "streams.datascape.dev/v1alpha1", Kind: "EventStream", Plural: "eventstreams", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/stream", Capabilities: []string{"datascape.dev/stream.publish"}, BindingRoles: []string{"source", "target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "event-contract"), APIVersion: "contracts.datascape.dev/v1alpha1", Kind: "EventContract", Plural: "eventcontracts", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/schema", Capabilities: []string{"datascape.dev/schema.register"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "object-store"), APIVersion: "stores.datascape.dev/v1alpha1", Kind: "ObjectStore", Plural: "objectstores", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/store", Capabilities: []string{"datascape.dev/store.object"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "lineage-sink"), APIVersion: "lineage.datascape.dev/v1alpha1", Kind: "LineageSink", Plural: "lineagesinks", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/lineage", Capabilities: []string{"datascape.dev/lineage.admit"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "audit-store"), APIVersion: "audit.datascape.dev/v1alpha1", Kind: "AuditStore", Plural: "auditstores", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/audit", Capabilities: []string{"datascape.dev/audit.record"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "pipeline"), APIVersion: "pipelines.datascape.dev/v1alpha1", Kind: "Pipeline", Plural: "pipelines", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/pipeline", BindingRoles: []string{"source", "target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "warehouse"), APIVersion: "stores.datascape.dev/v1alpha1", Kind: "Warehouse", Plural: "warehouses", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/warehouse", BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "table"), APIVersion: "tables.datascape.dev/v1alpha1", Kind: "Table", Plural: "tables", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/table", BindingRoles: []string{"source", "target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "table-catalog"), APIVersion: "catalogs.datascape.dev/v1alpha1", Kind: "TableCatalog", Plural: "tablecatalogs", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/catalog", Capabilities: []string{"datascape.dev/catalog.table"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "metadata-catalog"), APIVersion: "catalogs.datascape.dev/v1alpha1", Kind: "MetadataCatalog", Plural: "metadatacatalogs", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/metadata", Capabilities: []string{"datascape.dev/metadata.catalog"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "query-engine"), APIVersion: "compute.datascape.dev/v1alpha1", Kind: "QueryEngine", Plural: "queryengines", Scope: "Namespaced", Category: "component", ProviderType: "datascape.dev/query", Capabilities: []string{"datascape.dev/query.sql"}, BindingRoles: []string{"source", "target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "data-quality-rule"), APIVersion: "quality.datascape.dev/v1alpha1", Kind: "DataQualityRule", Plural: "dataqualityrules", Scope: "Namespaced", Category: "policy", ProviderType: "datascape.dev/quality", Capabilities: []string{"datascape.dev/quality.evaluate"}, BindingRoles: []string{"target"}},
		Definition{Identity: builtinIdentity("ResourceDefinition", "database-connection"), APIVersion: "connections.datascape.dev/v1alpha1", Kind: "DatabaseConnection", Plural: "databaseconnections", Scope: "Namespaced", Category: "connection"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "object-store-connection"), APIVersion: "connections.datascape.dev/v1alpha1", Kind: "ObjectStoreConnection", Plural: "objectstoreconnections", Scope: "Namespaced", Category: "connection"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "event-stream-connection"), APIVersion: "connections.datascape.dev/v1alpha1", Kind: "EventStreamConnection", Plural: "eventstreamconnections", Scope: "Namespaced", Category: "connection"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "cdc-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "CDCBinding", Plural: "cdcbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "stream-publish-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "StreamPublishBinding", Plural: "streampublishbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "stream-archive-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "StreamArchiveBinding", Plural: "streamarchivebindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "lineage-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "LineageBinding", Plural: "lineagebindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "audit-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "AuditBinding", Plural: "auditbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "pipeline-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "PipelineBinding", Plural: "pipelinebindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "access-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "AccessBinding", Plural: "accessbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "batch-ingest-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "BatchIngestBinding", Plural: "batchingestbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "stream-ingest-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "StreamIngestBinding", Plural: "streamingestbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "transform-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "TransformBinding", Plural: "transformbindings", Scope: "Namespaced", Category: "binding"},
		Definition{Identity: builtinIdentity("ResourceDefinition", "volume-mount-binding"), APIVersion: "bindings.datascape.dev/v1alpha1", Kind: "VolumeMountBinding", Plural: "volumemountbindings", Scope: "Namespaced", Category: "binding"},
	)
	return defs
}

func objectSchema(required []string, properties map[string]string) map[string]any {
	common := map[string]string{"ownership": "string", "lifecycle": "string", "state": "string", "providerInstanceRef": "string", "capabilities": "array", "verification": "object"}
	for name, typ := range common {
		if _, exists := properties[name]; !exists {
			properties[name] = typ
		}
	}
	props := make(map[string]any, len(properties))
	for name, typ := range properties {
		props[name] = map[string]any{"type": typ}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           props,
	}
}

func BuildRegistry(resources []spec.Resource) (*Registry, []domain.Diagnostic) {
	registry := NewRegistry()
	diags := make([]domain.Diagnostic, 0)
	for _, def := range BuiltinDefinitions() {
		if err := registry.Register(def); err != nil {
			diags = append(diags, diagnostic(spec.Resource{}, "DDEF000", "", err.Error(), "fix built-in resource definitions"))
		}
	}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "ResourceDefinition" {
			continue
		}
		def, err := definitionFromResource(res)
		if err != nil {
			diags = append(diags, diagnostic(res, "DDEF001", "spec", err.Error(), "declare spec.apiVersion and spec.kind for the resource definition"))
			continue
		}
		if err := registry.Register(def); err != nil {
			diags = append(diags, diagnostic(res, "DDEF002", "spec.kind", err.Error(), "keep exactly one ResourceDefinition per apiVersion/kind"))
		}
	}
	return registry, diags
}

func definitionFromResource(res spec.Resource) (Definition, error) {
	body, err := specBody(res)
	if err != nil {
		return Definition{}, err
	}
	apiVersion := stringField(body, "apiVersion")
	kind := stringField(body, "kind")
	if names, ok := body["names"].(map[string]any); ok {
		if kind == "" {
			kind = stringField(names, "kind")
		}
	}
	if apiVersion == "" {
		group := stringField(body, "group")
		version := stringField(body, "version")
		if group != "" && version != "" {
			apiVersion = group + "/" + version
		}
	}
	if kind == "" || apiVersion == "" {
		return Definition{}, fmt.Errorf("resource definition must declare apiVersion and kind")
	}
	schema, _ := body["schema"].(map[string]any)
	plural := stringField(body, "plural")
	if names, ok := body["names"].(map[string]any); ok && plural == "" {
		plural = stringField(names, "plural")
	}
	return Definition{
		Identity:     res.Identity("", ""),
		APIVersion:   apiVersion,
		Kind:         kind,
		Plural:       plural,
		Scope:        stringFieldDefault(body, "scope", "Namespaced"),
		Category:     stringField(body, "category"),
		ProviderType: stringField(body, "providerType"),
		Capabilities: stringSlice(body["capabilities"]),
		BindingRoles: stringSlice(body["bindingRoles"]),
		Schema:       schema,
		Extension:    boolField(body, "extension"),
	}, nil
}

func LegacyCurrentKind(kind string) bool {
	switch kind {
	case "AccessBinding",
		"ArchiveBinding",
		"AuditBinding",
		"ComponentCatalogue",
		"ConsumerBinding",
		"EventSource",
		"LineageBinding",
		"PipelineBinding",
		"PostgresSource",
		"ProducerBinding",
		"RawArchive",
		"SourceBinding":
		return true
	default:
		return false
	}
}

func CoreKinds() []string {
	defs := BuiltinDefinitions()
	kinds := make([]string, 0)
	for _, def := range defs {
		if def.APIVersion == api.PlatformV1Alpha1 && def.Core {
			kinds = append(kinds, def.Kind)
		}
	}
	sort.Strings(kinds)
	return kinds
}

func KindRefs(defs []Definition) []KindRef {
	out := make([]KindRef, 0, len(defs))
	for _, def := range defs {
		out = append(out, def.KindRef())
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func normalizeDefinition(def Definition) Definition {
	sort.Strings(def.Capabilities)
	sort.Strings(def.BindingRoles)
	if def.Scope == "" {
		def.Scope = "Namespaced"
	}
	if def.Category == "" {
		def.Category = "component"
	}
	return def
}

func builtinIdentity(kind, name string) domain.ResourceIdentity {
	return domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: kind, Namespace: api.DefaultNamespace, Name: name}
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
	sort.Strings(out)
	return out
}

func diagnostic(res spec.Resource, code, field, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{
		Severity:    domain.SeverityError,
		Code:        code,
		Resource:    res.Identity("", "").Display(),
		FieldPath:   field,
		Message:     message,
		Remediation: remediation,
		Location:    res.Location,
	}
}

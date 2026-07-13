package spec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/domain"
	"gopkg.in/yaml.v3"
)

const APIVersionV1Alpha1 = api.PlatformV1Alpha1

var logicalNameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Metadata contains the stable user-facing resource identity fields.
type Metadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// Resource is a versioned user-facing platform resource.
type Resource struct {
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Metadata   Metadata        `json:"metadata"`
	Spec       json.RawMessage `json:"spec,omitempty"`
	Status     json.RawMessage `json:"status,omitempty"`
	Location   domain.SourceLocation
}

// NamedDocument is a source document supplied to the compiler.
type NamedDocument struct {
	Name    string
	Content []byte
}

func SupportedKinds() []string {
	kinds := []string{
		"Binding",
		"BindingDefinition",
		"AccessBinding",
		"AuditBinding",
		"BatchIngestBinding",
		"CDCClass",
		"CDCBinding",
		"CDCInstance",
		"CDCOperation",
		"ConnectorClass",
		"DatabaseClass",
		"DatabaseConnection",
		"DatabaseInstance",
		"DataQualityRule",
		"EventContract",
		"EventProducer",
		"EventStream",
		"EventStreamConnection",
		"LineageBinding",
		"LineageSink",
		"ObjectStore",
		"ObjectStoreConnection",
		"PlatformPolicy",
		"Pipeline",
		"PipelineBinding",
		"PersistentVolume",
		"PersistentVolumeClaim",
		"Provider",
		"ProviderInstance",
		"RelationalSource",
		"ResourceDefinition",
		"RuntimeProfile",
		"SecretReference",
		"StreamArchiveBinding",
		"StreamIngestBinding",
		"StreamPublishBinding",
		"Table",
		"TableCatalog",
		"Target",
		"TransformBinding",
		"VolumeMountBinding",
		"StorageClass",
		"MetadataCatalog",
		"QueryEngine",
		"Warehouse",
	}
	sort.Strings(kinds)
	return kinds
}

func IsSupportedKind(kind string) bool {
	i := sort.SearchStrings(SupportedKinds(), kind)
	kinds := SupportedKinds()
	return i < len(kinds) && kinds[i] == kind
}

func (r Resource) Identity(target, adapter string) domain.ResourceIdentity {
	ns := r.Metadata.Namespace
	if ns == "" {
		ns = "default"
	}
	return domain.ResourceIdentity{
		APIVersion: r.APIVersion,
		Kind:       r.Kind,
		Namespace:  ns,
		Name:       r.Metadata.Name,
		Target:     target,
		Adapter:    adapter,
	}
}

func (r Resource) SourceName() string {
	if r.Location.File == "" {
		return r.Identity("", "").Display()
	}
	return r.Location.File
}

func ValidLogicalName(name string) bool {
	return logicalNameRE.MatchString(name)
}

// ParseDocuments parses YAML or JSON documents into normalized resources.
func ParseDocuments(ctx context.Context, docs []NamedDocument) ([]Resource, []domain.Diagnostic) {
	resources := make([]Resource, 0)
	diags := make([]domain.Diagnostic, 0)
	for _, doc := range docs {
		if err := ctx.Err(); err != nil {
			diags = append(diags, domain.Diagnostic{
				Severity: domain.SeverityError,
				Code:     "DCOMP001",
				Message:  err.Error(),
			})
			return resources, diags
		}
		parsed, docDiags := parseDocument(doc)
		resources = append(resources, parsed...)
		diags = append(diags, docDiags...)
	}
	sort.SliceStable(resources, func(i, j int) bool {
		return resources[i].Identity("", "").CanonicalString() < resources[j].Identity("", "").CanonicalString()
	})
	return resources, diags
}

func parseDocument(doc NamedDocument) ([]Resource, []domain.Diagnostic) {
	content := normalizeNewlines(doc.Content)
	trimmed := bytes.TrimSpace(content)
	if len(trimmed) == 0 {
		return nil, []domain.Diagnostic{parseDiagnostic(doc.Name, "DSPEC001", "", "document is empty", "provide at least one versioned platform resource")}
	}
	dec := yaml.NewDecoder(bytes.NewReader(content))
	resources := make([]Resource, 0)
	diags := make([]domain.Diagnostic, 0)
	docIndex := 0
	for {
		var node yaml.Node
		if err := dec.Decode(&node); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			diags = append(diags, parseDiagnostic(doc.Name, "DSPEC001", "", fmt.Sprintf("invalid YAML/JSON document: %v", err), "fix the document syntax"))
			break
		}
		if node.Kind == 0 || isEmptyDocument(&node) {
			docIndex++
			continue
		}
		docResources, docDiags := parseYAMLDocument(doc.Name, docIndex, &node)
		resources = append(resources, docResources...)
		diags = append(diags, docDiags...)
		docIndex++
	}
	return resources, diags
}

func parseYAMLDocument(file string, docIndex int, node *yaml.Node) ([]Resource, []domain.Diagnostic) {
	node = unwrapDocument(node)
	resourceNodes, diags := splitResourceNodes(file, docIndex, node)
	resources := make([]Resource, 0, len(resourceNodes))
	for itemIndex, resourceNode := range resourceNodes {
		raw, rawDiags := nodeToValue(file, docIndex, resourceNode, "")
		diags = append(diags, rawDiags...)
		if len(rawDiags) > 0 {
			continue
		}
		rawMap, ok := raw.(map[string]any)
		if !ok {
			diags = append(diags, nodeDiagnostic(file, docIndex, resourceNode, "DSPEC010", "", "resource document must be a mapping", "use apiVersion, kind, metadata, and spec fields"))
			continue
		}
		diags = append(diags, validateResourceShape(file, docIndex, resourceNode, rawMap)...)
		content, err := json.Marshal(rawMap)
		if err != nil {
			diags = append(diags, nodeDiagnostic(file, docIndex, resourceNode, "DSPEC011", "", fmt.Sprintf("resource %d cannot be encoded: %v", itemIndex, err), "remove unsupported YAML values"))
			continue
		}
		var resource Resource
		if err := json.Unmarshal(content, &resource); err != nil {
			diags = append(diags, nodeDiagnostic(file, docIndex, resourceNode, "DSPEC012", "", fmt.Sprintf("resource %d is not a valid resource object: %v", itemIndex, err), "use apiVersion, kind, metadata, and spec fields"))
			continue
		}
		resource.Location = domain.SourceLocation{File: file, Document: docIndex, Line: resourceNode.Line, Column: resourceNode.Column}
		if len(resource.Spec) == 0 {
			resource.Spec = json.RawMessage(`{}`)
		}
		resources = append(resources, resource)
	}
	return resources, diags
}

func splitResourceNodes(file string, docIndex int, node *yaml.Node) ([]*yaml.Node, []domain.Diagnostic) {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]*yaml.Node, 0, len(node.Content))
		for _, item := range node.Content {
			out = append(out, item)
		}
		return out, nil
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == "items" && node.Content[i+1].Kind == yaml.SequenceNode {
				out := make([]*yaml.Node, 0, len(node.Content[i+1].Content))
				for _, item := range node.Content[i+1].Content {
					out = append(out, item)
				}
				return out, nil
			}
		}
		return []*yaml.Node{node}, nil
	default:
		return nil, []domain.Diagnostic{nodeDiagnostic(file, docIndex, node, "DSPEC010", "", "top-level document must be a resource object, array, or items array", "use a resource object, an array of resources, or an object with an items array")}
	}
}

func validateResourceShape(file string, docIndex int, node *yaml.Node, raw map[string]any) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	allowedTop := map[string]struct{}{"apiVersion": {}, "kind": {}, "metadata": {}, "spec": {}, "status": {}}
	for key := range raw {
		if _, ok := allowedTop[key]; !ok {
			diags = append(diags, nodeDiagnostic(file, docIndex, findValueNode(node, key), "DSPEC015", key, "unknown top-level resource field "+key, "use only apiVersion, kind, metadata, spec, and status"))
		}
	}
	if metadata, ok := raw["metadata"].(map[string]any); ok {
		allowedMetadata := map[string]struct{}{"name": {}, "namespace": {}, "labels": {}, "annotations": {}}
		metadataNode := findValueNode(node, "metadata")
		for key := range metadata {
			if _, ok := allowedMetadata[key]; !ok {
				diags = append(diags, nodeDiagnostic(file, docIndex, findValueNode(metadataNode, key), "DSPEC016", "metadata."+key, "unknown metadata field "+key, "use only name, namespace, labels, and annotations"))
			}
		}
	}
	return diags
}

func nodeToValue(file string, docIndex int, node *yaml.Node, path string) (any, []domain.Diagnostic) {
	node = unwrapDocument(node)
	switch node.Kind {
	case yaml.MappingNode:
		out := make(map[string]any, len(node.Content)/2)
		seen := make(map[string]*yaml.Node, len(node.Content)/2)
		diags := make([]domain.Diagnostic, 0)
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			if keyNode.Kind != yaml.ScalarNode {
				diags = append(diags, nodeDiagnostic(file, docIndex, keyNode, "DSPEC017", path, "mapping keys must be scalar strings", "use string keys only"))
				continue
			}
			key := keyNode.Value
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if prior, ok := seen[key]; ok {
				diags = append(diags, nodeDiagnostic(file, docIndex, keyNode, "DSPEC018", nextPath, "duplicate key "+key, fmt.Sprintf("remove the duplicate key; first occurrence is at line %d column %d", prior.Line, prior.Column)))
				continue
			}
			seen[key] = keyNode
			value, childDiags := nodeToValue(file, docIndex, valueNode, nextPath)
			diags = append(diags, childDiags...)
			out[key] = value
		}
		return out, diags
	case yaml.SequenceNode:
		out := make([]any, 0, len(node.Content))
		diags := make([]domain.Diagnostic, 0)
		for i, child := range node.Content {
			value, childDiags := nodeToValue(file, docIndex, child, fmt.Sprintf("%s[%d]", path, i))
			diags = append(diags, childDiags...)
			out = append(out, value)
		}
		return out, diags
	case yaml.AliasNode:
		if node.Alias == nil {
			return nil, []domain.Diagnostic{nodeDiagnostic(file, docIndex, node, "DSPEC019", path, "alias has no target", "remove the alias or define the anchor")}
		}
		return nodeToValue(file, docIndex, node.Alias, path)
	case yaml.ScalarNode:
		return scalarValue(node), nil
	default:
		return nil, []domain.Diagnostic{nodeDiagnostic(file, docIndex, node, "DSPEC020", path, "unsupported YAML node", "use mappings, sequences, scalars, and aliases only")}
	}
}

func scalarValue(node *yaml.Node) any {
	switch node.Tag {
	case "!!null":
		return nil
	case "!!bool":
		return strings.EqualFold(node.Value, "true")
	case "!!int":
		var number json.Number = json.Number(node.Value)
		return number
	case "!!float":
		var number json.Number = json.Number(node.Value)
		return number
	default:
		return node.Value
	}
}

func parseDiagnostic(file, code, fieldPath, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{
		Severity:    domain.SeverityError,
		Code:        code,
		FieldPath:   fieldPath,
		Message:     message,
		Remediation: remediation,
		Location:    domain.SourceLocation{File: file},
	}
}

func nodeDiagnostic(file string, docIndex int, node *yaml.Node, code, fieldPath, message, remediation string) domain.Diagnostic {
	location := domain.SourceLocation{File: file, Document: docIndex}
	if node != nil {
		location.Line = node.Line
		location.Column = node.Column
	}
	return domain.Diagnostic{
		Severity:    domain.SeverityError,
		Code:        code,
		FieldPath:   fieldPath,
		Message:     message,
		Remediation: remediation,
		Location:    location,
	}
}

func normalizeNewlines(content []byte) []byte {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(content, []byte("\r"), []byte("\n"))
}

func unwrapDocument(node *yaml.Node) *yaml.Node {
	if node != nil && node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		return node.Content[0]
	}
	return node
}

func isEmptyDocument(node *yaml.Node) bool {
	node = unwrapDocument(node)
	return node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null" && node.Value == "")
}

func findValueNode(node *yaml.Node, key string) *yaml.Node {
	node = unwrapDocument(node)
	if node == nil || node.Kind != yaml.MappingNode {
		return node
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return node
}

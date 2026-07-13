package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/domain"
	resourcepkg "datascape.dev/platformctl/internal/resource"
	"datascape.dev/platformctl/internal/spec"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

var storageQuantityRE = regexp.MustCompile(`^[1-9][0-9]*(Ki|Mi|Gi|Ti|Pi)$`)

// ValidateResources runs structural, definition, reference, and invariant validation.
func ValidateResources(ctx context.Context, resources []spec.Resource) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	definitions, defDiags := resourcepkg.BuildRegistry(resources)
	diags = append(diags, defDiags...)
	diags = append(diags, validateDuplicates(resources)...)
	for _, res := range resources {
		if err := ctx.Err(); err != nil {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCOMP001", Message: err.Error()})
			break
		}
		diags = append(diags, validateResource(res, definitions)...)
	}
	diags = append(diags, validateReferences(resources)...)
	SortDiagnostics(diags)
	return diags
}

func validateDuplicates(resources []spec.Resource) []domain.Diagnostic {
	seen := map[string]spec.Resource{}
	diags := make([]domain.Diagnostic, 0)
	for _, res := range resources {
		id := res.Identity("", "").CanonicalString()
		if prior, ok := seen[id]; ok {
			diags = append(diags, domain.Diagnostic{
				Severity:    domain.SeverityError,
				Code:        "DVAL001",
				Resource:    res.Identity("", "").Display(),
				FieldPath:   "metadata.name",
				Message:     fmt.Sprintf("duplicate resource identity also declared in %s", prior.SourceName()),
				Remediation: "rename one resource or place it in a distinct logical namespace",
				Location:    res.Location,
			})
			continue
		}
		seen[id] = res
	}
	return diags
}

func validateResource(res spec.Resource, definitions *resourcepkg.Registry) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	if res.APIVersion == "" {
		diags = append(diags, diag(res, "DSPEC002", "apiVersion", "apiVersion is required", "set apiVersion to the resource definition apiVersion"))
	} else if res.APIVersion == api.PlatformV1Alpha1 && resourcepkg.LegacyCurrentKind(res.Kind) {
		diags = append(diags, diag(res, "DLEGACY001", "kind", "legacy kind "+res.Kind+" is not part of the current v1alpha1 API", "run platformctl migrate and compile the migrated resource/binding model"))
	}
	if res.Kind == "" {
		diags = append(diags, diag(res, "DSPEC004", "kind", "kind is required", "set kind to a registered resource kind"))
	} else if _, ok := definitions.Lookup(res.APIVersion, res.Kind); !ok {
		diags = append(diags, diag(res, "DRES001", "kind", "unregistered resource kind "+res.APIVersion+"/"+res.Kind, "declare a ResourceDefinition or use a built-in definition"))
	}
	if res.Metadata.Name == "" {
		diags = append(diags, diag(res, "DSPEC006", "metadata.name", "metadata.name is required", "use a stable lowercase logical name"))
	} else if !spec.ValidLogicalName(res.Metadata.Name) {
		diags = append(diags, diag(res, "DSPEC007", "metadata.name", "metadata.name must use lowercase DNS-label syntax", "use lowercase letters, digits, and hyphens"))
	}
	if res.Metadata.Namespace != "" && !spec.ValidLogicalName(res.Metadata.Namespace) {
		diags = append(diags, diag(res, "DSPEC008", "metadata.namespace", "metadata.namespace must use lowercase DNS-label syntax", "use lowercase letters, digits, and hyphens"))
	}
	if !json.Valid(res.Spec) {
		diags = append(diags, diag(res, "DSPEC013", "spec", "spec must be valid JSON", "provide an object-valued spec"))
		return diags
	}
	if len(res.Status) > 0 && !json.Valid(res.Status) {
		diags = append(diags, diag(res, "DSPEC022", "status", "status must be valid JSON", "remove invalid status content"))
	}
	body, err := specBody(res)
	if err != nil {
		diags = append(diags, diag(res, "DSPEC014", "spec", "spec must be a JSON object", "provide object-valued spec fields"))
		return diags
	}
	findSecretValues(res, "", body, &diags)
	if def, ok := definitions.Lookup(res.APIVersion, res.Kind); ok {
		diags = append(diags, validateDefinitionRules(res, body, def)...)
	}
	diags = append(diags, validateCoreInvariants(res, body)...)
	return diags
}

func validateDefinitionRules(res spec.Resource, body map[string]any, def resourcepkg.Definition) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	if strings.EqualFold(def.Scope, "Cluster") && res.Metadata.Namespace != "" {
		diags = append(diags, diag(res, "DRES002", "metadata.namespace", "cluster-scoped resources must not set metadata.namespace", "remove metadata.namespace or change the ResourceDefinition scope"))
	}
	if len(def.Schema) == 0 {
		return diags
	}
	required := stringSlice(def.Schema["required"])
	for _, key := range required {
		if _, ok := body[key]; !ok {
			diags = append(diags, diag(res, "DRES003", "spec."+key, "required field is missing", "set spec."+key+" or update the ResourceDefinition schema"))
		}
	}
	properties, _ := def.Schema["properties"].(map[string]any)
	if additional, ok := def.Schema["additionalProperties"].(bool); ok && !additional {
		for key := range body {
			if _, known := properties[key]; !known {
				diags = append(diags, diag(res, "DRES005", "spec."+key, "field is not declared by the resource definition schema", "remove the field or update the ResourceDefinition schema"))
			}
		}
	}
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value, ok := body[key]
		if !ok {
			continue
		}
		prop, _ := properties[key].(map[string]any)
		if typ, _ := prop["type"].(string); typ != "" && !matchesJSONType(value, typ) {
			diags = append(diags, diag(res, "DRES004", "spec."+key, "field does not match ResourceDefinition schema type "+typ, "change the value type or update the schema"))
		}
	}
	diags = append(diags, validateJSONSchema2020(res, body, def)...)
	return diags
}

func validateJSONSchema2020(res spec.Resource, body map[string]any, def resourcepkg.Definition) []domain.Diagnostic {
	schemaDoc := cloneAnyMap(def.Schema)
	if _, ok := schemaDoc["$schema"]; !ok {
		schemaDoc["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	}
	schemaJSON, err := json.Marshal(schemaDoc)
	if err != nil {
		return []domain.Diagnostic{diag(res, "DDEF003", "spec", "resource definition schema could not be encoded: "+err.Error(), "fix the ResourceDefinition schema")}
	}
	normalizedSchema, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaJSON))
	if err != nil {
		return []domain.Diagnostic{diag(res, "DDEF003", "spec", "resource definition schema is not JSON: "+err.Error(), "fix the ResourceDefinition schema")}
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	url := "https://platform.datascape.dev/schemas/" + strings.ReplaceAll(def.APIVersion, "/", "_") + "/" + strings.ToLower(def.Kind) + ".json"
	if err := compiler.AddResource(url, normalizedSchema); err != nil {
		return []domain.Diagnostic{diag(res, "DDEF003", "spec", "resource definition schema could not be loaded: "+err.Error(), "fix the ResourceDefinition schema")}
	}
	schema, err := compiler.Compile(url)
	if err != nil {
		return []domain.Diagnostic{diag(res, "DDEF003", "spec", "resource definition schema is invalid: "+err.Error(), "use valid JSON Schema 2020-12")}
	}
	if err := schema.Validate(body); err != nil {
		field := "spec"
		if validationErr, ok := err.(*jsonschema.ValidationError); ok && len(validationErr.InstanceLocation) > 0 {
			field += "." + strings.Join(validationErr.InstanceLocation, ".")
		}
		return []domain.Diagnostic{diag(res, "DRES006", field, "JSON Schema 2020-12 validation failed: "+err.Error(), "correct the field using platformctl explain "+res.Kind)}
	}
	return nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+1)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func validateCoreInvariants(res spec.Resource, body map[string]any) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	switch res.Kind {
	case "RuntimeProfile":
		target := stringValue(body["target"])
		availability, _ := body["availability"].(map[string]any)
		class := stringValue(availability["class"])
		if target == "compose" && class == "production-ha" {
			diags = append(diags, diag(res, "DVAL006", "spec.availability.class", "Compose target cannot be declared production-ha", "use a production target profile or lower the Compose availability class"))
		}
	case "Provider":
		if len(stringSlice(body["capabilities"])) == 0 {
			diags = append(diags, diag(res, "DPROV001", "spec.capabilities", "provider must declare at least one capability", "set spec.capabilities"))
		}
	case "ProviderInstance":
		if stringValue(body["providerRef"]) == "" {
			diags = append(diags, diag(res, "DPROV004", "spec.providerRef", "provider instance must reference a Provider", "set spec.providerRef"))
		}
	case "SecretReference":
		backend := stringValue(body["backend"])
		if !validSecretBackend(backend) {
			diags = append(diags, diag(res, "DCONN001", "spec.backend", "secret backend must be env, file, kubernetes, or vault", "set spec.backend to a supported secret backend"))
		}
		if len(stringSlice(body["keys"])) == 0 {
			diags = append(diags, diag(res, "DCONN002", "spec.keys", "secret reference must declare logical keys", "set spec.keys to logical keys such as username, password, token, accessKey, or secretKey"))
		}
	case "StorageClass":
		if policy := stringValue(body["reclaimPolicy"]); policy != "" && policy != "Retain" && policy != "Delete" {
			diags = append(diags, diag(res, "DSTOR001", "spec.reclaimPolicy", "reclaimPolicy must be Retain or Delete", "choose an explicit supported reclaim policy"))
		}
		if mode := stringValue(body["volumeBindingMode"]); mode != "" && mode != "Immediate" && mode != "WaitForFirstConsumer" {
			diags = append(diags, diag(res, "DSTOR002", "spec.volumeBindingMode", "volumeBindingMode must be Immediate or WaitForFirstConsumer", "choose a supported binding mode"))
		}
	case "PersistentVolume", "PersistentVolumeClaim":
		if !validCapacity(stringValue(body["capacity"])) {
			diags = append(diags, diag(res, "DSTOR003", "spec.capacity", "capacity must be a positive quantity such as 10Gi or 500Mi", "set a positive binary storage quantity"))
		}
		accessModes := stringSlice(body["accessModes"])
		if len(accessModes) == 0 {
			diags = append(diags, diag(res, "DSTOR004", "spec.accessModes", "at least one access mode is required", "declare ReadWriteOnce, ReadOnlyMany, or ReadWriteMany"))
		}
		for _, mode := range accessModes {
			if mode != "ReadWriteOnce" && mode != "ReadOnlyMany" && mode != "ReadWriteMany" {
				diags = append(diags, diag(res, "DSTOR004", "spec.accessModes", "unsupported access mode "+mode, "use ReadWriteOnce, ReadOnlyMany, or ReadWriteMany"))
			}
		}
	case "DatabaseClass":
		if strings.TrimSpace(stringValue(body["engine"])) == "" {
			diags = append(diags, diag(res, "DDB001", "spec.engine", "database class must declare an engine compatibility name", "set the engine, for example postgresql or sqlite"))
		}
	case "ConnectorClass":
		if !containsString([]string{"native", "jdbc", "odbc"}, stringValue(body["interface"])) {
			diags = append(diags, diag(res, "DCONN007", "spec.interface", "connector interface must be native, jdbc, or odbc", "choose the driver interface exposed to consumers"))
		}
		if !containsString([]string{"tcp", "unix", "file"}, stringValue(body["transport"])) {
			diags = append(diags, diag(res, "DCONN008", "spec.transport", "connector transport must be tcp, unix, or file", "model network and file access separately from the driver interface"))
		}
	case "CDCClass":
		if !containsString([]string{"kafka-connect"}, stringValue(body["engine"])) {
			diags = append(diags, diag(res, "DCDC001", "spec.engine", "CDC class engine must be kafka-connect", "declare a supported CDC runtime engine"))
		}
	case "CDCInstance":
		if stringValue(body["classRef"]) == "" {
			diags = append(diags, diag(res, "DCDC002", "spec.classRef", "CDC instance must reference a CDCClass", "set spec.classRef"))
		}
		ownership := stringValue(body["ownership"])
		if ownership != "" && !containsString([]string{"managed", "imported", "external"}, ownership) {
			diags = append(diags, diag(res, "DCDC003", "spec.ownership", "CDC ownership must be managed, imported, or external", "choose an explicit supported ownership mode"))
		}
		policy := stringValue(body["managementPolicy"])
		if policy != "" && !containsString([]string{"ManagedConnectors", "ObserveOnly"}, policy) {
			diags = append(diags, diag(res, "DCDC004", "spec.managementPolicy", "CDC managementPolicy must be ManagedConnectors or ObserveOnly", "choose whether Datascape can manage connectors or only observe"))
		}
	case "CDCOperation":
		if stringValue(body["targetRef"]) == "" {
			diags = append(diags, diag(res, "DOPS001", "spec.targetRef", "CDC operation must target a CDC resource", "set targetRef to a CDCBinding or CDCInstance"))
		}
		if stringValue(body["action"]) == "" {
			diags = append(diags, diag(res, "DOPS002", "spec.action", "CDC operation must declare an action", "set a provider-supported action such as PauseConnector or ResetOffsets"))
		}
		if stringValue(body["idempotencyKey"]) == "" {
			diags = append(diags, diag(res, "DOPS003", "spec.idempotencyKey", "CDC operation must declare a stable idempotency key", "set a unique operation key that can be rerun safely"))
		}
	case "DatabaseInstance":
		if stringValue(body["classRef"]) == "" {
			diags = append(diags, diag(res, "DDB002", "spec.classRef", "database instance must reference a DatabaseClass", "set spec.classRef"))
		}
	case "DatabaseConnection":
		legacy := stringValue(body["engine"]) != "" || stringValue(body["host"]) != ""
		if legacy && !externallyOwned(body) {
			for _, field := range []string{"engine", "host", "port", "database", "credentialsRef"} {
				if _, ok := body[field]; !ok || stringLikeEmpty(body[field]) {
					diags = append(diags, diag(res, "DCONN003", "spec."+field, "legacy network database connection is missing required field "+field, "set all legacy network fields or migrate to instanceRef and connectorClassRef"))
				}
			}
		} else if !legacy && !externallyOwned(body) {
			for _, field := range []string{"instanceRef", "connectorClassRef"} {
				if stringValue(body[field]) == "" {
					diags = append(diags, diag(res, "DCONN009", "spec."+field, "database connection must bind a database instance to a connector class", "set instanceRef and connectorClassRef"))
				}
			}
		}
	case "ObjectStoreConnection", "EventStreamConnection":
		if !externallyOwned(body) && stringValue(body["credentialsRef"]) == "" {
			diags = append(diags, diag(res, "DCONN003", "spec.credentialsRef", res.Kind+" must reference credentials", "set spec.credentialsRef to a SecretReference"))
		}
	case "RelationalSource":
		if !externallyOwned(body) && stringValue(body["connectionRef"]) == "" {
			diags = append(diags, diag(res, "DCONN006", "spec.connectionRef", "relational source must reference a DatabaseConnection", "set spec.connectionRef and keep credentials on the connection resource"))
		}
	case "Table":
		if layer := stringValue(body["layer"]); layer != "" && !containsString([]string{"bronze", "silver", "gold"}, layer) {
			diags = append(diags, diag(res, "DTABLE001", "spec.layer", "table layer must be bronze, silver, or gold", "use the medallion classification only for dataset purpose"))
		}
	case "Binding":
		if stringValue(body["capability"]) == "" && stringValue(body["bindingDefinitionRef"]) == "" {
			diags = append(diags, diag(res, "DBIND004", "spec.capability", "binding must declare a capability", "set spec.capability or spec.bindingDefinitionRef"))
		}
	case "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding", "BatchIngestBinding", "StreamIngestBinding", "TransformBinding", "VolumeMountBinding":
		for _, field := range typedBindingRequiredFields(res.Kind) {
			if stringValue(body[field]) == "" {
				diags = append(diags, diag(res, "DBIND014", "spec."+field, res.Kind+" must declare "+field, "set spec."+field))
			}
		}
	}
	return diags
}

func validateReferences(resources []spec.Resource) []domain.Diagnostic {
	byID := map[string]spec.Resource{}
	for _, res := range resources {
		byID[res.Identity("", "").CanonicalString()] = res
	}
	secretKeys := secretKeysByID(resources)
	diags := make([]domain.Diagnostic, 0)
	for _, res := range resources {
		body, ok := specBodyOK(res)
		if !ok || plannedOrDisabled(body) {
			continue
		}
		switch res.Kind {
		case "ProviderInstance":
			ref := parseRef(stringValue(body["providerRef"]), res, "Provider")
			if ref.Name != "" {
				if _, ok := byID[ref.CanonicalString()]; !ok && !isBuiltinProvider(ref) {
					diags = append(diags, diag(res, "DREF002", "spec.providerRef", "referenced provider does not exist: "+stringValue(body["providerRef"]), "declare the Provider or correct the reference"))
				}
			}
		case "Binding":
			diags = append(diags, validateRefField(res, body, byID, "sourceRef", "", false)...)
			diags = append(diags, validateRefField(res, body, byID, "targetRef", "", false)...)
			diags = append(diags, validateRefField(res, body, byID, "providerInstanceRef", "ProviderInstance", true)...)
		case "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding", "BatchIngestBinding", "StreamIngestBinding", "TransformBinding", "VolumeMountBinding":
			for _, ref := range typedBindingRefs(res.Kind) {
				diags = append(diags, validateRefField(res, body, byID, ref.field, ref.expectedKind, ref.allowBuiltin)...)
			}
		case "CDCClass":
			diags = append(diags, validateRefField(res, body, byID, "providerInstanceRef", "ProviderInstance", true)...)
			for _, connectorName := range stringSlice(body["supportedConnectorClasses"]) {
				connectorRef := connectorName
				if !strings.Contains(connectorRef, "/") {
					connectorRef = "ConnectorClass/" + connectorRef
				}
				ref := parseRef(connectorRef, res, "ConnectorClass")
				if ref.Name != "" {
					if _, ok := byID[ref.CanonicalString()]; !ok {
						diags = append(diags, diag(res, "DCDC005", "spec.supportedConnectorClasses", "supported connector class does not exist: "+connectorName, "declare the ConnectorClass or remove it from the CDCClass"))
					}
				}
			}
		case "CDCInstance":
			diags = append(diags, validateRefField(res, body, byID, "classRef", "CDCClass", false)...)
			diags = append(diags, validateRefField(res, body, byID, "providerInstanceRef", "ProviderInstance", true)...)
			diags = append(diags, validateRefField(res, body, byID, "credentialsRef", "SecretReference", false)...)
		case "CDCOperation":
			diags = append(diags, validateRefField(res, body, byID, "targetRef", "", false)...)
			diags = append(diags, validateRefField(res, body, byID, "providerInstanceRef", "ProviderInstance", true)...)
		case "RelationalSource":
			diags = append(diags, validateRefField(res, body, byID, "connectionRef", "DatabaseConnection", false)...)
		case "DatabaseInstance":
			diags = append(diags, validateRefField(res, body, byID, "classRef", "DatabaseClass", false)...)
			diags = append(diags, validateRefField(res, body, byID, "storageClaimRef", "PersistentVolumeClaim", false)...)
			diags = append(diags, validateRefField(res, body, byID, "credentialsRef", "SecretReference", false)...)
		case "PersistentVolume":
			diags = append(diags, validateRefField(res, body, byID, "storageClassRef", "StorageClass", false)...)
		case "PersistentVolumeClaim":
			diags = append(diags, validateRefField(res, body, byID, "storageClassRef", "StorageClass", false)...)
			diags = append(diags, validateRefField(res, body, byID, "volumeRef", "PersistentVolume", false)...)
		case "DatabaseConnection", "ObjectStoreConnection", "EventStreamConnection":
			if res.Kind == "DatabaseConnection" {
				diags = append(diags, validateRefField(res, body, byID, "instanceRef", "DatabaseInstance", false)...)
				diags = append(diags, validateRefField(res, body, byID, "connectorClassRef", "ConnectorClass", false)...)
				diags = append(diags, validateRefField(res, body, byID, "claimRef", "PersistentVolumeClaim", false)...)
			}
			diags = append(diags, validateRefField(res, body, byID, "credentialsRef", "SecretReference", false)...)
			diags = append(diags, validateRequiredSecretKeys(res, body, secretKeys)...)
		}
	}
	diags = append(diags, validateDatabaseTopology(resources, byID)...)
	diags = append(diags, validateCDCTopology(resources, byID)...)
	diags = append(diags, validateCDCOperations(resources, byID)...)
	return diags
}

func validateDatabaseTopology(resources []spec.Resource, byID map[string]spec.Resource) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	for _, class := range resources {
		if class.Kind != "DatabaseClass" {
			continue
		}
		body, ok := specBodyOK(class)
		if !ok {
			continue
		}
		engine := stringValue(body["engine"])
		for _, connectorName := range stringSlice(body["supportedConnectorClasses"]) {
			connectorRef := connectorName
			if !strings.Contains(connectorRef, "/") {
				connectorRef = "ConnectorClass/" + connectorRef
			}
			connectorID := parseRef(connectorRef, class, "ConnectorClass")
			connector, exists := byID[connectorID.CanonicalString()]
			if !exists {
				diags = append(diags, diag(class, "DDB003", "spec.supportedConnectorClasses", "supported connector class does not exist: "+connectorName, "declare the ConnectorClass or remove it from the compatibility list"))
				continue
			}
			connectorBody, _ := specBodyOK(connector)
			if compatible := stringSlice(connectorBody["compatibleEngines"]); len(compatible) > 0 && !containsString(compatible, engine) {
				diags = append(diags, diag(class, "DCONN010", "spec.supportedConnectorClasses", "connector class "+connectorName+" is not compatible with database engine "+engine, "align DatabaseClass and ConnectorClass engine compatibility"))
			}
		}
	}
	for _, connection := range resources {
		if connection.Kind != "DatabaseConnection" {
			continue
		}
		body, ok := specBodyOK(connection)
		if !ok || stringValue(body["instanceRef"]) == "" {
			continue
		}
		instanceID := parseRef(stringValue(body["instanceRef"]), connection, "DatabaseInstance")
		connectorID := parseRef(stringValue(body["connectorClassRef"]), connection, "ConnectorClass")
		instance, instanceOK := byID[instanceID.CanonicalString()]
		connector, connectorOK := byID[connectorID.CanonicalString()]
		if !instanceOK || !connectorOK {
			continue
		}
		instanceBody, _ := specBodyOK(instance)
		connectorBody, _ := specBodyOK(connector)
		classID := parseRef(stringValue(instanceBody["classRef"]), instance, "DatabaseClass")
		class, classOK := byID[classID.CanonicalString()]
		if !classOK {
			continue
		}
		classBody, _ := specBodyOK(class)
		engine := stringValue(classBody["engine"])
		compatible := stringSlice(connectorBody["compatibleEngines"])
		if len(compatible) > 0 && !containsString(compatible, engine) {
			diags = append(diags, diag(connection, "DCONN010", "spec.connectorClassRef", "connector class is not compatible with database engine "+engine, "choose a connector whose compatibleEngines includes the DatabaseClass engine"))
		}
		transport := stringValue(connectorBody["transport"])
		if transport == "file" {
			file, _ := body["file"].(map[string]any)
			if stringValue(body["claimRef"]) == "" || stringValue(file["path"]) == "" {
				diags = append(diags, diag(connection, "DCONN011", "spec.file", "file transport requires claimRef and file.path", "mount the database claim and declare the path visible to the connector"))
			}
		}
		if transport == "tcp" {
			endpoint, _ := body["endpoint"].(map[string]any)
			if stringValue(endpoint["host"]) == "" || stringLikeEmpty(endpoint["port"]) {
				diags = append(diags, diag(connection, "DCONN012", "spec.endpoint", "TCP transport requires endpoint.host and endpoint.port", "declare the managed service name or external endpoint"))
			}
		}
	}
	for _, binding := range resources {
		if binding.Kind != "CDCBinding" {
			continue
		}
		body, ok := specBodyOK(binding)
		if !ok || stringValue(body["connectorClassRef"]) == "" {
			continue
		}
		connectorID := parseRef(stringValue(body["connectorClassRef"]), binding, "ConnectorClass")
		connector, exists := byID[connectorID.CanonicalString()]
		if !exists {
			continue
		}
		connectorBody, _ := specBodyOK(connector)
		if !containsString(stringSlice(connectorBody["operations"]), "change-stream") {
			diags = append(diags, diag(binding, "DCONN013", "spec.connectorClassRef", "CDC connector class must advertise the change-stream operation", "use a ConnectorClass with operations containing change-stream"))
		}
		engine := databaseEngineForSource(body["sourceRef"], binding, byID)
		compatible := stringSlice(connectorBody["compatibleEngines"])
		if engine != "" && len(compatible) > 0 && !containsString(compatible, engine) {
			diags = append(diags, diag(binding, "DCONN010", "spec.connectorClassRef", "CDC connector class is not compatible with source database engine "+engine, "choose a CDC connector whose compatibleEngines includes the source DatabaseClass engine"))
		}
	}
	return diags
}

func databaseEngineForSource(sourceRef any, owner spec.Resource, byID map[string]spec.Resource) string {
	sourceID := parseRef(stringValue(sourceRef), owner, "RelationalSource")
	source, ok := byID[sourceID.CanonicalString()]
	if !ok {
		return ""
	}
	sourceBody, _ := specBodyOK(source)
	connectionID := parseRef(stringValue(sourceBody["connectionRef"]), source, "DatabaseConnection")
	connection, ok := byID[connectionID.CanonicalString()]
	if !ok {
		return ""
	}
	connectionBody, _ := specBodyOK(connection)
	instanceID := parseRef(stringValue(connectionBody["instanceRef"]), connection, "DatabaseInstance")
	instance, ok := byID[instanceID.CanonicalString()]
	if !ok {
		return ""
	}
	instanceBody, _ := specBodyOK(instance)
	classID := parseRef(stringValue(instanceBody["classRef"]), instance, "DatabaseClass")
	class, ok := byID[classID.CanonicalString()]
	if !ok {
		return ""
	}
	classBody, _ := specBodyOK(class)
	return stringValue(classBody["engine"])
}

func validateCDCTopology(resources []spec.Resource, byID map[string]spec.Resource) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	target := runtimeTarget(resources)
	production := productionProfile(resources)
	cdcInstances := make([]spec.Resource, 0)
	for _, res := range resources {
		if res.Kind == "CDCInstance" {
			cdcInstances = append(cdcInstances, res)
		}
	}
	for _, cdcClass := range resources {
		if cdcClass.Kind != "CDCClass" {
			continue
		}
		body, ok := specBodyOK(cdcClass)
		if !ok {
			continue
		}
		if target != "" {
			compat := stringSlice(body["targetCompatibility"])
			if len(compat) > 0 && !containsString(compat, target) {
				diags = append(diags, diag(cdcClass, "DCDC006", "spec.targetCompatibility", "CDC class is not compatible with target "+target, "choose a CDCClass that includes the deployment target"))
			}
		}
	}
	for _, instance := range cdcInstances {
		body, ok := specBodyOK(instance)
		if !ok {
			continue
		}
		classID := parseRef(stringValue(body["classRef"]), instance, "CDCClass")
		class, ok := byID[classID.CanonicalString()]
		if !ok {
			continue
		}
		classBody, _ := specBodyOK(class)
		if target != "" {
			compat := stringSlice(classBody["targetCompatibility"])
			if len(compat) > 0 && !containsString(compat, target) {
				diags = append(diags, diag(instance, "DCDC006", "spec.classRef", "CDC instance class is not compatible with target "+target, "choose a target-compatible CDCClass"))
			}
		}
		ownership := stringValue(body["ownership"])
		if ownership == "external" || ownership == "imported" {
			endpoint, _ := body["endpoint"].(map[string]any)
			if stringValue(endpoint["url"]) == "" && stringValue(endpoint["host"]) == "" {
				diags = append(diags, diag(instance, "DCDC007", "spec.endpoint", "external CDC instance must declare a control endpoint", "set endpoint.url or endpoint.host/port for verification and connector management"))
			}
		}
	}
	for _, binding := range resources {
		if binding.Kind != "CDCBinding" {
			continue
		}
		body, ok := specBodyOK(binding)
		if !ok {
			continue
		}
		cdcRef := stringValue(body["cdcRef"])
		if cdcRef == "" {
			if production {
				diags = append(diags, diag(binding, "DCDC008", "spec.cdcRef", "production CDC bindings must explicitly reference a CDCInstance", "set spec.cdcRef to avoid implicit runtime selection"))
			} else if len(cdcInstances) > 1 {
				diags = append(diags, diag(binding, "DCDC009", "spec.cdcRef", "CDC runtime inference is ambiguous because multiple CDCInstance resources exist", "set spec.cdcRef explicitly"))
			}
			continue
		}
		cdcID := parseRef(cdcRef, binding, "CDCInstance")
		cdcInstance, ok := byID[cdcID.CanonicalString()]
		if !ok {
			continue
		}
		connectorID := parseRef(stringValue(body["connectorClassRef"]), binding, "ConnectorClass")
		if connectorID.Name == "" {
			continue
		}
		cdcBody, _ := specBodyOK(cdcInstance)
		classID := parseRef(stringValue(cdcBody["classRef"]), cdcInstance, "CDCClass")
		cdcClass, ok := byID[classID.CanonicalString()]
		if !ok {
			continue
		}
		classBody, _ := specBodyOK(cdcClass)
		supported := stringSlice(classBody["supportedConnectorClasses"])
		if len(supported) > 0 && !refListContains(supported, connectorID, cdcClass) {
			diags = append(diags, diag(binding, "DCDC010", "spec.connectorClassRef", "connector class is not supported by the referenced CDCClass", "choose one of the CDCClass supportedConnectorClasses"))
		}
	}
	return diags
}

func validateCDCOperations(resources []spec.Resource, byID map[string]spec.Resource) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	operationKeys := map[string]spec.Resource{}
	for _, op := range resources {
		if op.Kind != "CDCOperation" {
			continue
		}
		body, ok := specBodyOK(op)
		if !ok {
			continue
		}
		target := parseRef(stringValue(body["targetRef"]), op, "")
		action := stringValue(body["action"])
		key := stringValue(body["idempotencyKey"])
		if prior, exists := operationKeys[key]; key != "" && exists {
			priorBody, _ := specBodyOK(prior)
			if stringValue(priorBody["targetRef"]) != stringValue(body["targetRef"]) || stringValue(priorBody["action"]) != action {
				diags = append(diags, diag(op, "DOPS004", "spec.idempotencyKey", "idempotency key is reused for a different operation", "use one stable key per logical operation"))
			}
		} else if key != "" {
			operationKeys[key] = op
		}
		cdcInstance := operationCDCInstance(target, op, byID)
		if cdcInstance.Name != "" {
			if instance, ok := byID[cdcInstance.CanonicalString()]; ok {
				instanceBody, _ := specBodyOK(instance)
				ownership := stringValue(instanceBody["ownership"])
				policy := cdcManagementPolicy(instanceBody)
				if policy == "ObserveOnly" && operationMutates(action) {
					diags = append(diags, diag(op, "DOPS005", "spec.action", "ObserveOnly CDC instances reject mutation operations", "change the CDCInstance managementPolicy or choose a read-only operation"))
				}
				if (ownership == "external" || ownership == "imported") && policy == "ManagedConnectors" && operationManagesWorker(action) {
					diags = append(diags, diag(op, "DOPS006", "spec.action", "external ManagedConnectors CDC instances do not allow worker-management operations", "target connector operations only or declare an explicit worker-management contract"))
				}
			}
		}
		if operationDestructive(action) {
			approval, _ := body["approval"].(map[string]any)
			if !boolValue(approval["required"], false) && !boolValue(approval["approved"], false) {
				diags = append(diags, diag(op, "DOPS007", "spec.approval", "destructive CDC operation requires explicit approval", "set approval.required: true and record the approval workflow"))
			}
		}
		if action == "ResetOffsets" {
			parameters, _ := body["parameters"].(map[string]any)
			if stringValue(parameters["backupRef"]) == "" && stringValue(parameters["backupMetadata"]) == "" {
				diags = append(diags, diag(op, "DOPS008", "spec.parameters.backupRef", "offset reset requires backup metadata", "capture and reference exported offsets before reset"))
			}
			if !containsString(stringSlice(body["preconditions"]), "ConnectorPaused") {
				diags = append(diags, diag(op, "DOPS009", "spec.preconditions", "offset reset requires the connector to be paused", "add ConnectorPaused to preconditions and verify it before execution"))
			}
		}
		if action == "DeleteCDCInstance" || action == "Delete" {
			if target.Kind == "CDCInstance" && cdcInstanceHasBindings(target, resources) {
				diags = append(diags, diag(op, "DOPS010", "spec.targetRef", "shared CDC instance cannot be deleted while bindings reference it", "delete or move referencing CDCBinding resources first"))
			}
		}
	}
	return diags
}

func refListContains(values []string, target domain.ResourceIdentity, owner spec.Resource) bool {
	for _, value := range values {
		refValue := value
		if !strings.Contains(refValue, "/") {
			refValue = target.Kind + "/" + refValue
		}
		ref := parseRef(refValue, owner, target.Kind)
		if ref.CanonicalString() == target.CanonicalString() {
			return true
		}
	}
	return false
}

func operationCDCInstance(target domain.ResourceIdentity, owner spec.Resource, byID map[string]spec.Resource) domain.ResourceIdentity {
	switch target.Kind {
	case "CDCInstance":
		return target
	case "CDCBinding":
		binding, ok := byID[target.CanonicalString()]
		if !ok {
			return domain.ResourceIdentity{}
		}
		body, _ := specBodyOK(binding)
		return parseRef(stringValue(body["cdcRef"]), binding, "CDCInstance")
	default:
		_ = owner
		return domain.ResourceIdentity{}
	}
}

func cdcInstanceHasBindings(instance domain.ResourceIdentity, resources []spec.Resource) bool {
	for _, res := range resources {
		if res.Kind != "CDCBinding" {
			continue
		}
		body, ok := specBodyOK(res)
		if !ok {
			continue
		}
		ref := parseRef(stringValue(body["cdcRef"]), res, "CDCInstance")
		if ref.CanonicalString() == instance.CanonicalString() {
			return true
		}
	}
	return false
}

func cdcManagementPolicy(body map[string]any) string {
	if policy := stringValue(body["managementPolicy"]); policy != "" {
		return policy
	}
	if stringValue(body["ownership"]) == "external" || stringValue(body["ownership"]) == "imported" {
		return "ObserveOnly"
	}
	return "ManagedConnectors"
}

func operationMutates(action string) bool {
	return !containsString([]string{"Inspect", "InspectOffsets", "VerifyConnector", "ValidateConnectivity", "PlanBackup", "PlanRestore"}, action)
}

func operationManagesWorker(action string) bool {
	return containsString([]string{"ScaleWorker", "UpgradeWorker", "RestartWorker", "ReplaceWorker", "RotateWorkerCertificate"}, action)
}

func operationDestructive(action string) bool {
	return containsString([]string{"ResetOffsets", "DeleteConnector", "DeleteCDCInstance", "Delete", "DetachAndDelete"}, action)
}

func runtimeTarget(resources []spec.Resource) string {
	for _, res := range resources {
		if res.Kind != "RuntimeProfile" && res.Kind != "Target" {
			continue
		}
		body, ok := specBodyOK(res)
		if !ok {
			continue
		}
		if target := stringValue(body["target"]); target != "" {
			return target
		}
		if target := stringValue(body["type"]); target != "" {
			return target
		}
	}
	return ""
}

func productionProfile(resources []spec.Resource) bool {
	for _, res := range resources {
		if res.Kind != "RuntimeProfile" && res.Kind != "Target" {
			continue
		}
		body, ok := specBodyOK(res)
		if !ok {
			continue
		}
		development, _ := body["development"].(map[string]any)
		if boolValue(development["enabled"], false) || boolValue(development["allowUnpinnedImages"], false) {
			continue
		}
		availability, _ := body["availability"].(map[string]any)
		class := stringValue(availability["class"])
		if class == "production" || class == "single-host-production" {
			return true
		}
	}
	return false
}

type typedRef struct {
	field        string
	expectedKind string
	allowBuiltin bool
}

func validateRefField(res spec.Resource, body map[string]any, byID map[string]spec.Resource, field, expectedKind string, allowBuiltinProviderInstance bool) []domain.Diagnostic {
	value := stringValue(body[field])
	if value == "" {
		return nil
	}
	ref := parseRef(value, res, expectedKind)
	if ref.Name == "" {
		return []domain.Diagnostic{diag(res, "DREF003", "spec."+field, "reference must use Kind/name, Kind/namespace/name, or group/version/Kind/namespace/name", "use stable logical resource identity syntax")}
	}
	if _, ok := byID[ref.CanonicalString()]; !ok && !(allowBuiltinProviderInstance && isBuiltinProviderInstance(ref)) {
		return []domain.Diagnostic{diag(res, "DREF002", "spec."+field, "referenced resource does not exist: "+value, "create the referenced resource or correct the reference")}
	}
	return nil
}

func typedBindingRefs(kind string) []typedRef {
	refs := []typedRef{{field: "providerInstanceRef", expectedKind: "ProviderInstance", allowBuiltin: true}}
	switch kind {
	case "CDCBinding":
		refs = append(refs, typedRef{field: "sourceRef"}, typedRef{field: "streamRef", expectedKind: "EventStream"}, typedRef{field: "cdcRef", expectedKind: "CDCInstance"}, typedRef{field: "connectorClassRef", expectedKind: "ConnectorClass"})
	case "StreamPublishBinding":
		refs = append(refs, typedRef{field: "sourceRef", expectedKind: "EventProducer"}, typedRef{field: "streamRef", expectedKind: "EventStream"})
	case "StreamArchiveBinding":
		refs = append(refs, typedRef{field: "streamRef", expectedKind: "EventStream"}, typedRef{field: "objectStoreRef", expectedKind: "ObjectStore"})
	case "LineageBinding":
		refs = append(refs, typedRef{field: "sourceRef"}, typedRef{field: "sinkRef", expectedKind: "LineageSink"})
	case "AuditBinding":
		refs = append(refs, typedRef{field: "sourceRef"}, typedRef{field: "auditStoreRef", expectedKind: "AuditStore"})
	case "PipelineBinding":
		refs = append(refs, typedRef{field: "sourceRef"}, typedRef{field: "pipelineRef", expectedKind: "Pipeline"})
	case "AccessBinding":
		refs = append(refs, typedRef{field: "subjectRef"}, typedRef{field: "resourceRef"})
	case "BatchIngestBinding":
		refs = append(refs, typedRef{field: "sourceRef", expectedKind: "RelationalSource"}, typedRef{field: "tableRef", expectedKind: "Table"})
	case "StreamIngestBinding":
		refs = append(refs, typedRef{field: "streamRef", expectedKind: "EventStream"}, typedRef{field: "tableRef", expectedKind: "Table"})
	case "TransformBinding":
		refs = append(refs, typedRef{field: "sourceRef", expectedKind: "Table"}, typedRef{field: "targetRef", expectedKind: "Table"})
	case "VolumeMountBinding":
		refs = append(refs, typedRef{field: "claimRef", expectedKind: "PersistentVolumeClaim"}, typedRef{field: "workloadRef"})
	}
	return refs
}

func parseRef(value string, owner spec.Resource, expectedKind string) domain.ResourceIdentity {
	if value == "" {
		return domain.ResourceIdentity{}
	}
	parts := strings.Split(value, "/")
	ns := defaultNamespace(owner.Metadata.Namespace)
	apiVersion := apiVersionForKind(expectedKind)
	kind := expectedKind
	name := ""
	switch len(parts) {
	case 2:
		kind, name = parts[0], parts[1]
		apiVersion = apiVersionForKind(kind)
	case 3:
		kind, ns, name = parts[0], parts[1], parts[2]
		apiVersion = apiVersionForKind(kind)
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
	return domain.ResourceIdentity{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name}
}

func clusterScopedKind(kind string) bool {
	switch kind {
	case "StorageClass", "PersistentVolume", "DatabaseClass", "ConnectorClass", "CDCClass":
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
	case "CDCClass", "CDCInstance":
		return "cdc.datascape.dev/v1alpha1"
	case "CDCOperation":
		return "operations.datascape.dev/v1alpha1"
	case "StorageClass", "PersistentVolume", "PersistentVolumeClaim":
		return "storage.datascape.dev/v1alpha1"
	case "ObjectStore", "Warehouse":
		return "stores.datascape.dev/v1alpha1"
	case "TableCatalog", "MetadataCatalog":
		return "catalogs.datascape.dev/v1alpha1"
	case "QueryEngine":
		return "compute.datascape.dev/v1alpha1"
	case "DataQualityRule":
		return "quality.datascape.dev/v1alpha1"
	case "LineageSink":
		return "lineage.datascape.dev/v1alpha1"
	case "AuditStore":
		return "audit.datascape.dev/v1alpha1"
	case "Pipeline":
		return "pipelines.datascape.dev/v1alpha1"
	case "Table":
		return "tables.datascape.dev/v1alpha1"
	case "CDCBinding", "StreamPublishBinding", "StreamArchiveBinding", "LineageBinding", "AuditBinding", "PipelineBinding", "AccessBinding", "BatchIngestBinding", "StreamIngestBinding", "TransformBinding", "VolumeMountBinding":
		return "bindings.datascape.dev/v1alpha1"
	default:
		return api.PlatformV1Alpha1
	}
}

func validSecretBackend(value string) bool {
	switch value {
	case "env", "file", "kubernetes", "vault":
		return true
	default:
		return false
	}
}

func validCapacity(value string) bool {
	return storageQuantityRE.MatchString(value)
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func typedBindingRequiredFields(kind string) []string {
	switch kind {
	case "CDCBinding", "StreamPublishBinding":
		return []string{"sourceRef", "streamRef"}
	case "StreamArchiveBinding":
		return []string{"streamRef", "objectStoreRef"}
	case "LineageBinding":
		return []string{"sourceRef", "sinkRef"}
	case "AuditBinding":
		return []string{"sourceRef", "auditStoreRef"}
	case "PipelineBinding":
		return []string{"sourceRef", "pipelineRef"}
	case "AccessBinding":
		return []string{"subjectRef", "resourceRef"}
	case "BatchIngestBinding":
		return []string{"sourceRef", "tableRef"}
	case "StreamIngestBinding":
		return []string{"streamRef", "tableRef"}
	case "TransformBinding":
		return []string{"sourceRef", "targetRef"}
	case "VolumeMountBinding":
		return []string{"claimRef", "workloadRef", "mountPath"}
	default:
		return nil
	}
}

func secretKeysByID(resources []spec.Resource) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, res := range resources {
		if res.APIVersion != api.PlatformV1Alpha1 || res.Kind != "SecretReference" {
			continue
		}
		body, ok := specBodyOK(res)
		if !ok {
			continue
		}
		keys := map[string]bool{}
		for _, key := range stringSlice(body["keys"]) {
			keys[key] = true
		}
		out[res.Identity("", "").CanonicalString()] = keys
	}
	return out
}

func validateRequiredSecretKeys(res spec.Resource, body map[string]any, secretKeys map[string]map[string]bool) []domain.Diagnostic {
	refValue := stringValue(body["credentialsRef"])
	if refValue == "" {
		return nil
	}
	ref := parseRef(refValue, res, "SecretReference")
	keys, ok := secretKeys[ref.CanonicalString()]
	if !ok {
		return nil
	}
	required := requiredSecretKeys(res.Kind)
	diags := make([]domain.Diagnostic, 0)
	for _, key := range required {
		if !keys[key] {
			diags = append(diags, diag(res, "DCONN004", "spec.credentialsRef", "referenced SecretReference is missing key "+key, "add "+key+" to the referenced SecretReference spec.keys"))
		}
	}
	return diags
}

func requiredSecretKeys(kind string) []string {
	switch kind {
	case "DatabaseConnection":
		return []string{"username", "password"}
	case "ObjectStoreConnection":
		return []string{"accessKey", "secretKey"}
	case "EventStreamConnection":
		return []string{"token"}
	default:
		return nil
	}
}

func isBuiltinProvider(ref domain.ResourceIdentity) bool {
	return ref.APIVersion == api.PlatformV1Alpha1 && ref.Kind == "Provider" && strings.HasPrefix(ref.Name, "local-")
}

func isBuiltinProviderInstance(ref domain.ResourceIdentity) bool {
	return ref.APIVersion == api.PlatformV1Alpha1 && ref.Kind == "ProviderInstance" && strings.HasPrefix(ref.Name, "local-")
}

func matchesJSONType(value any, typ string) bool {
	switch typ {
	case "string":
		_, ok := value.(string)
		return ok
	case "object":
		_, ok := value.(map[string]any)
		return ok
	case "array":
		_, ok := value.([]any)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "integer":
		switch value.(type) {
		case int, int64, json.Number:
			return true
		default:
			return false
		}
	case "number":
		switch value.(type) {
		case int, int64, float64, json.Number:
			return true
		default:
			return false
		}
	default:
		return true
	}
}

func plannedOrDisabled(body map[string]any) bool {
	state := stringValue(body["state"])
	ownership := stringValue(body["ownership"])
	return state == "planned" || state == "deferred" || state == "disabled" || ownership == "planned" || ownership == "disabled"
}

func externallyOwned(body map[string]any) bool {
	ownership := stringValue(body["ownership"])
	return ownership == "external" || ownership == "imported" || boolValue(body["external"], false)
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

func specBodyOK(res spec.Resource) (map[string]any, bool) {
	body, err := specBody(res)
	return body, err == nil
}

func findSecretValues(res spec.Resource, path string, value any, diags *[]domain.Diagnostic) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			nextPath := key
			if path != "" {
				nextPath = path + "." + key
			}
			if strings.Contains(nextPath, "mapping.") || strings.HasSuffix(nextPath, "configMapping."+key) {
				findSecretValues(res, nextPath, typed[key], diags)
				continue
			}
			lower := strings.ToLower(key)
			if isSecretKey(lower) {
				if s, ok := typed[key].(string); ok && s != "" && !looksLikeReference(s) {
					*diags = append(*diags, diag(res, "DVAL005", "spec."+nextPath, "secret-like field contains an inline value", "use a SecretReference or external secret reference rather than embedding secret material"))
				}
			}
			findSecretValues(res, nextPath, typed[key], diags)
		}
	case []any:
		for i, item := range typed {
			findSecretValues(res, fmt.Sprintf("%s[%d]", path, i), item, diags)
		}
	}
}

func isSecretKey(key string) bool {
	if strings.Contains(key, "secret_manager") || strings.Contains(key, "secrets_manager") || strings.HasSuffix(key, "secretbackend") {
		return false
	}
	return strings.Contains(key, "password") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "token") ||
		strings.Contains(key, "privatekey")
}

func looksLikeReference(value string) bool {
	return strings.HasPrefix(value, "ref:") ||
		strings.HasPrefix(value, "secretRef:") ||
		strings.HasPrefix(value, "${") ||
		strings.HasPrefix(value, "vault:")
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func stringLikeEmpty(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == ""
	case nil:
		return true
	default:
		return false
	}
}

func boolValue(value any, fallback bool) bool {
	b, ok := value.(bool)
	if !ok {
		return fallback
	}
	return b
}

func stringSlice(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func defaultNamespace(namespace string) string {
	if namespace == "" {
		return api.DefaultNamespace
	}
	return namespace
}

func diag(res spec.Resource, code, fieldPath, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{
		Severity:    domain.SeverityError,
		Code:        code,
		Resource:    res.Identity("", "").Display(),
		FieldPath:   fieldPath,
		Message:     message,
		Remediation: remediation,
		Location:    res.Location,
	}
}

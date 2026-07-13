package compiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/binding"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/provider"
	"datascape.dev/platformctl/internal/spec"
)

func buildCDCPlan(resources []spec.Resource, bindings []binding.Resolved, providers *provider.Registry, target string) (ir.CDCPlan, []domain.Diagnostic) {
	byID := resourceIndex(resources, target)
	classes := cdcClassPlans(resources, target)
	instances := cdcInstancePlans(resources, classes, target)
	hasCDCBinding := false
	for _, resolved := range bindings {
		if resolved.Kind == "CDCBinding" && resolved.State != "disabled" && resolved.State != "deferred" {
			hasCDCBinding = true
			break
		}
	}
	if hasCDCBinding && len(classes) == 0 {
		classes = append(classes, defaultCDCClass(target))
	}
	if hasCDCBinding && len(instances) == 0 {
		instances = append(instances, defaultCDCInstance(target))
	}
	classByID := map[string]ir.CDCClassPlan{}
	for _, class := range classes {
		classByID[class.Identity.CanonicalString()] = class
	}
	instanceByID := map[string]ir.CDCInstancePlan{}
	for _, instance := range instances {
		instanceByID[instance.Identity.CanonicalString()] = instance
	}
	connectors := make([]ir.CDCConnectorPlan, 0)
	diags := make([]domain.Diagnostic, 0)
	for _, resolved := range bindings {
		if resolved.Kind != "CDCBinding" || resolved.State == "disabled" || resolved.State == "deferred" {
			continue
		}
		connector, connectorDiags := buildCDCConnectorPlan(resolved, byID, classByID, instanceByID, providers, target)
		diags = append(diags, connectorDiags...)
		if connector.Binding.Name == "" {
			continue
		}
		connectors = append(connectors, connector)
	}
	sort.SliceStable(classes, func(i, j int) bool {
		return classes[i].Identity.CanonicalString() < classes[j].Identity.CanonicalString()
	})
	sort.SliceStable(instances, func(i, j int) bool {
		return instances[i].Identity.CanonicalString() < instances[j].Identity.CanonicalString()
	})
	sort.SliceStable(connectors, func(i, j int) bool {
		return connectors[i].Identity.CanonicalString() < connectors[j].Identity.CanonicalString()
	})
	diags = append(diags, validateCDCPlan(classes, instances, connectors, target)...)
	return ir.CDCPlan{Classes: classes, Instances: instances, Connectors: connectors}, diags
}

func cdcClassPlans(resources []spec.Resource, target string) []ir.CDCClassPlan {
	out := make([]ir.CDCClassPlan, 0)
	for _, res := range resources {
		if res.Kind != "CDCClass" {
			continue
		}
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		supported := make([]domain.ResourceIdentity, 0)
		for _, value := range stringSlice(body["supportedConnectorClasses"]) {
			refValue := value
			if !strings.Contains(refValue, "/") {
				refValue = "ConnectorClass/" + refValue
			}
			ref := parseCDCRef(refValue, res, "ConnectorClass", target)
			if ref.Name != "" {
				supported = append(supported, ref)
			}
		}
		sort.SliceStable(supported, func(i, j int) bool { return supported[i].CanonicalString() < supported[j].CanonicalString() })
		out = append(out, ir.CDCClassPlan{
			Identity:                  res.Identity(target, ""),
			Engine:                    stringValue(body["engine"]),
			ProviderInstance:          parseCDCRef(stringValue(body["providerInstanceRef"]), res, "ProviderInstance", target),
			SupportedConnectorClasses: supported,
			TargetCompatibility:       stringSlice(body["targetCompatibility"]),
			Parameters:                cloneMap(anyMap(body["parameters"])),
			WorkerConfiguration:       cloneMap(anyMap(body["workerConfiguration"])),
		})
	}
	return out
}

func cdcInstancePlans(resources []spec.Resource, classes []ir.CDCClassPlan, target string) []ir.CDCInstancePlan {
	classByID := map[string]ir.CDCClassPlan{}
	for _, class := range classes {
		classByID[class.Identity.CanonicalString()] = class
	}
	out := make([]ir.CDCInstancePlan, 0)
	for _, res := range resources {
		if res.Kind != "CDCInstance" {
			continue
		}
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		classID := parseCDCRef(stringValue(body["classRef"]), res, "CDCClass", target)
		class := classByID[classID.CanonicalString()]
		providerInstance := parseCDCRef(stringValue(body["providerInstanceRef"]), res, "ProviderInstance", target)
		if providerInstance.Name == "" {
			providerInstance = class.ProviderInstance
		}
		ownership := resourceOwnership(res, body)
		policy := stringValue(body["managementPolicy"])
		if policy == "" {
			if ownership == "external" || ownership == "imported" {
				policy = "ObserveOnly"
			} else {
				policy = "ManagedConnectors"
			}
		}
		out = append(out, ir.CDCInstancePlan{
			Identity:            res.Identity(target, ""),
			Class:               classID,
			Engine:              stringDefault(class.Engine, "kafka-connect"),
			Ownership:           ownership,
			ManagementPolicy:    policy,
			ProviderInstance:    providerInstance,
			Replicas:            intDefault(intValue(body["replicas"]), 1),
			Resources:           resourcesFromBody(anyMap(body["resources"])),
			Endpoint:            endpointFromBody(anyMap(body["endpoint"])),
			CredentialsRef:      parseCDCRef(stringValue(body["credentialsRef"]), res, "SecretReference", target),
			Parameters:          mergeAnyMaps(class.Parameters, anyMap(body["parameters"])),
			WorkerConfiguration: mergeAnyMaps(class.WorkerConfiguration, anyMap(body["workerConfiguration"])),
			Verification:        verificationChecks(body["verification"]),
			State:               graphState(ownership, resourceLifecycle(body)),
		})
	}
	return out
}

func buildCDCConnectorPlan(resolved binding.Resolved, byID map[string]spec.Resource, classes map[string]ir.CDCClassPlan, instances map[string]ir.CDCInstancePlan, providers *provider.Registry, target string) (ir.CDCConnectorPlan, []domain.Diagnostic) {
	diags := make([]domain.Diagnostic, 0)
	bindingRes, ok := byID[resolved.Identity.CanonicalString()]
	if !ok {
		return ir.CDCConnectorPlan{}, diags
	}
	body, _ := resourceBody(bindingRes)
	cdcID := resolved.CDCInstance
	if cdcID.Name == "" {
		cdcID = inferCDCInstance(resolved, instances)
	}
	cdcInstance, ok := instances[cdcID.CanonicalString()]
	if !ok {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCDC012", Resource: resolved.Identity.Display(), FieldPath: "spec.cdcRef", Message: "CDC binding does not resolve to a CDCInstance", Remediation: "set spec.cdcRef or declare exactly one compatible default CDCInstance"})
		return ir.CDCConnectorPlan{}, diags
	}
	sourceRes, ok := byID[resolved.Source.CanonicalString()]
	if !ok {
		return ir.CDCConnectorPlan{}, diags
	}
	sourceBody, _ := resourceBody(sourceRes)
	connectionID := parseCDCRef(stringValue(sourceBody["connectionRef"]), sourceRes, "DatabaseConnection", target)
	connection, ok := byID[connectionID.CanonicalString()]
	if !ok {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCDC014", Resource: resolved.Identity.Display(), FieldPath: "spec.sourceRef", Message: "CDC source does not resolve to a DatabaseConnection", Remediation: "set RelationalSource.spec.connectionRef"})
		return ir.CDCConnectorPlan{}, diags
	}
	connectionInfo := resolveDatabaseConnection(connection, byID, target)
	connectorClassID := resolved.ConnectorClass
	if connectorClassID.Name == "" {
		connectorClassID = connectionInfo.ConnectionConnectorClass
	}
	if connectorClassID.Name == "" {
		connectorClassID = defaultConnectorClassIdentity(target)
	}
	connectorClassBody := map[string]any{}
	if connectorClassRes, ok := byID[connectorClassID.CanonicalString()]; ok {
		connectorClassBody, _ = resourceBody(connectorClassRes)
	}
	connectorName := stringValue(body["connectorName"])
	if connectorName == "" {
		connectorName = sanitizeName(resolved.Identity.Namespace + "-" + resolved.Identity.Name)
	}
	tables := stringSlice(body["tables"])
	if len(tables) == 0 {
		tables = stringSlice(sourceBody["tables"])
	}
	topic := firstStringValue(body, "topic", "topicName")
	if topic == "" {
		topic = resolved.Target.Name
	}
	snapshot := stringDefault(stringValue(body["snapshot"]), "initial")
	config := cdcProviderConfiguration(connectorClassBody, body, connectionInfo, connectorName, tables, snapshot, topic)
	providerInstance := cdcInstance.ProviderInstance
	if providerInstance.Name == "" {
		providerInstance = resolved.ProviderInstance
	}
	if providerInstance.Name != "" {
		if _, _, ok := providers.Instance(providerInstance); !ok {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCDC015", Resource: resolved.Identity.Display(), FieldPath: "spec.cdcRef", Message: "CDC provider instance does not exist: " + providerInstance.Display(), Remediation: "declare the ProviderInstance or correct the CDCInstance providerInstanceRef"})
		}
	}
	deps := []domain.ResourceIdentity{resolved.Source, connectionID, resolved.Target, cdcID, connectorClassID}
	if connectionInfo.DatabaseInstance.Name != "" {
		deps = append(deps, connectionInfo.DatabaseInstance)
	}
	if connectionInfo.DatabaseClass.Name != "" {
		deps = append(deps, connectionInfo.DatabaseClass)
	}
	sort.SliceStable(deps, func(i, j int) bool { return deps[i].CanonicalString() < deps[j].CanonicalString() })
	return ir.CDCConnectorPlan{
		Identity:              domain.ResourceIdentity{APIVersion: "cdc.datascape.dev/v1alpha1", Kind: "CDCConnector", Namespace: resolved.Identity.Namespace, Name: resolved.Identity.Name, Target: target},
		Binding:               resolved.Identity,
		Source:                resolved.Source,
		DatabaseConnection:    connectionID,
		DatabaseInstance:      connectionInfo.DatabaseInstance,
		DatabaseClass:         connectionInfo.DatabaseClass,
		DatabaseEngine:        connectionInfo.Engine,
		DatabaseEndpoint:      connectionInfo.Endpoint,
		CredentialsRef:        connectionInfo.CredentialsRef,
		CredentialEnvironment: connectionInfo.CredentialEnv,
		DestinationStream:     resolved.Target,
		CDCInstance:           cdcID,
		ConnectorClass:        connectorClassID,
		ConnectorName:         connectorName,
		ConfigPath:            "configuration/cdc/" + resolved.Identity.Namespace + "/" + cdcID.Name + "/" + resolved.Identity.Name + ".json",
		Tables:                tables,
		SnapshotMode:          snapshot,
		Topic:                 topic,
		ProviderConfiguration: config,
		Lifecycle:             stringDefault(stringValue(body["lifecycle"]), "steady-state"),
		Ownership:             cdcInstance.Ownership,
		State:                 connectorState(cdcInstance),
		Dependencies:          deps,
		Verification: []ir.VerificationCheck{
			{ID: "CDC-CONNECTOR-" + strings.ToUpper(sanitizeName(resolved.Identity.Namespace+"-"+resolved.Identity.Name)), Description: "CDC connector " + connectorName + " reaches a healthy task state"},
		},
	}, diags
}

type databaseConnectionInfo struct {
	Engine                   string
	Endpoint                 ir.EndpointPlan
	Database                 string
	CredentialsRef           domain.ResourceIdentity
	CredentialEnv            map[string]string
	DatabaseInstance         domain.ResourceIdentity
	DatabaseClass            domain.ResourceIdentity
	ConnectionConnectorClass domain.ResourceIdentity
}

func resolveDatabaseConnection(connection spec.Resource, byID map[string]spec.Resource, target string) databaseConnectionInfo {
	body, _ := resourceBody(connection)
	info := databaseConnectionInfo{
		Engine:                   normalizeDatabaseEngine(stringValue(body["engine"])),
		Endpoint:                 endpointFromConnection(body),
		Database:                 stringValue(body["database"]),
		CredentialsRef:           parseCDCRef(stringValue(body["credentialsRef"]), connection, "SecretReference", target),
		ConnectionConnectorClass: parseCDCRef(stringValue(body["connectorClassRef"]), connection, "ConnectorClass", target),
	}
	instanceID := parseCDCRef(stringValue(body["instanceRef"]), connection, "DatabaseInstance", target)
	if instanceID.Name != "" {
		info.DatabaseInstance = instanceID
		if instance, ok := byID[instanceID.CanonicalString()]; ok {
			instanceBody, _ := resourceBody(instance)
			if info.CredentialsRef.Name == "" {
				info.CredentialsRef = parseCDCRef(stringValue(instanceBody["credentialsRef"]), instance, "SecretReference", target)
			}
			if info.Database == "" {
				info.Database = stringValue(instanceBody["database"])
			}
			classID := parseCDCRef(stringValue(instanceBody["classRef"]), instance, "DatabaseClass", target)
			info.DatabaseClass = classID
			if class, ok := byID[classID.CanonicalString()]; ok {
				classBody, _ := resourceBody(class)
				if info.Engine == "" {
					info.Engine = normalizeDatabaseEngine(stringValue(classBody["engine"]))
				}
			}
		}
	}
	if info.Endpoint.Host == "" && info.DatabaseInstance.Name != "" {
		info.Endpoint.Host = info.DatabaseInstance.Name
	}
	if info.Endpoint.Port == "" {
		info.Endpoint.Port = defaultDatabasePort(info.Engine)
	}
	if info.Database == "" {
		info.Database = connection.Metadata.Name
	}
	info.CredentialEnv = credentialEnvNames(info.CredentialsRef)
	return info
}

func cdcProviderConfiguration(connectorClassBody, bindingBody map[string]any, connection databaseConnectionInfo, connectorName string, tables []string, snapshot, topic string) map[string]any {
	defaults := map[string]any{}
	if configuration := anyMap(connectorClassBody["configuration"]); len(configuration) > 0 {
		defaults = mergeAnyMaps(defaults, anyMap(configuration["defaults"]))
	}
	if parameters := anyMap(connectorClassBody["parameters"]); len(parameters) > 0 {
		defaults = mergeAnyMaps(defaults, anyMap(parameters["defaults"]))
	}
	mapping := map[string]string{}
	for key, value := range defaultCDCConfigMapping() {
		mapping[key] = value
	}
	for key, value := range anyMap(connectorClassBody["configMapping"]) {
		if s, ok := value.(string); ok {
			mapping[key] = s
		}
	}
	if configuration := anyMap(connectorClassBody["configuration"]); len(configuration) > 0 {
		for key, value := range anyMap(configuration["mapping"]) {
			if s, ok := value.(string); ok {
				mapping[key] = s
			}
		}
	}
	normalized := map[string]any{
		"connector.class":           stringDefault(stringValue(connectorClassBody["driver"]), "io.debezium.connector.postgresql.PostgresConnector"),
		"connection.host":           connection.Endpoint.Host,
		"connection.port":           connection.Endpoint.Port,
		"database.name":             connection.Database,
		"credentials.usernameEnv":   envReference(connection.CredentialEnv["username"]),
		"credentials.passwordEnv":   envReference(connection.CredentialEnv["password"]),
		"tables.includeList":        strings.Join(tables, ","),
		"snapshot.mode":             snapshot,
		"stream.topic":              topic,
		"stream.topicPrefix":        sanitizeName(connectorName),
		"connector.name":            connectorName,
		"connector.publicationName": postgresIdentifier(connectorName + "_publication"),
		"connector.slotName":        postgresIdentifier(connectorName + "_slot"),
		"plugin.name":               "pgoutput",
	}
	config := mergeAnyMaps(defaults, map[string]any{})
	for nativeKey, normalizedKey := range mapping {
		value, ok := normalized[normalizedKey]
		if !ok || emptyConfigValue(value) {
			continue
		}
		config[nativeKey] = value
	}
	config = mergeAnyMaps(config, defaultCDCTransformConfig(topic))
	config = mergeAnyMaps(config, anyMap(bindingBody["configuration"]))
	return config
}

func defaultCDCConfigMapping() map[string]string {
	return map[string]string{
		"connector.class":    "connector.class",
		"database.hostname":  "connection.host",
		"database.port":      "connection.port",
		"database.user":      "credentials.usernameEnv",
		"database.password":  "credentials.passwordEnv",
		"database.dbname":    "database.name",
		"topic.prefix":       "stream.topicPrefix",
		"table.include.list": "tables.includeList",
		"snapshot.mode":      "snapshot.mode",
		"publication.name":   "connector.publicationName",
		"slot.name":          "connector.slotName",
		"plugin.name":        "plugin.name",
	}
}

func postgresIdentifier(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastUnderscore := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	identifier := strings.Trim(b.String(), "_")
	if identifier == "" {
		return "datascape_cdc"
	}
	if identifier[0] >= '0' && identifier[0] <= '9' {
		identifier = "cdc_" + identifier
	}
	if len(identifier) > 63 {
		identifier = identifier[:63]
		identifier = strings.TrimRight(identifier, "_")
	}
	return identifier
}

func defaultCDCTransformConfig(topic string) map[string]any {
	if topic == "" {
		return map[string]any{}
	}
	return map[string]any{
		"transforms":                         "route",
		"transforms.route.type":              "io.debezium.transforms.ByLogicalTableRouter",
		"transforms.route.topic.regex":       ".*",
		"transforms.route.topic.replacement": topic,
		"key.converter":                      "org.apache.kafka.connect.json.JsonConverter",
		"key.converter.schemas.enable":       "true",
		"value.converter":                    "org.apache.kafka.connect.json.JsonConverter",
		"value.converter.schemas.enable":     "true",
		"publication.autocreate.mode":        "filtered",
	}
}

func validateCDCPlan(classes []ir.CDCClassPlan, instances []ir.CDCInstancePlan, connectors []ir.CDCConnectorPlan, target string) []domain.Diagnostic {
	diags := make([]domain.Diagnostic, 0)
	connectorNames := map[string]ir.CDCConnectorPlan{}
	for _, connector := range connectors {
		key := connector.CDCInstance.CanonicalString() + "|" + connector.ConnectorName
		if prior, ok := connectorNames[key]; ok {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCDC016", Resource: connector.Binding.Display(), FieldPath: "spec.connectorName", Message: "CDC connector name collides with " + prior.Binding.Display(), Remediation: "set a unique connectorName or rename one CDCBinding"})
		}
		connectorNames[key] = connector
	}
	serviceNames := map[string]domain.ResourceIdentity{}
	for _, instance := range instances {
		if target == "compose" && instance.Ownership == "managed" && instance.Replicas > 1 {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCDC017", Resource: instance.Identity.Display(), FieldPath: "spec.replicas", Message: "Compose CDC runtime replicas above 1 are not safely supported in this release", Remediation: "split connectors across separate CDCInstance resources or use a target with safe horizontal scaling"})
		}
		if instance.Ownership != "managed" {
			continue
		}
		name := cdcWorkerServiceName(instance.Identity)
		if prior, ok := serviceNames[name]; ok {
			diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DCOMPOSE004", Resource: instance.Identity.Display(), FieldPath: "metadata.name", Message: "CDC worker service name collides with " + prior.Display(), Remediation: "rename one CDCInstance"})
		}
		serviceNames[name] = instance.Identity
	}
	_ = classes
	return diags
}

func inferCDCInstance(resolved binding.Resolved, instances map[string]ir.CDCInstancePlan) domain.ResourceIdentity {
	if len(instances) == 1 {
		for _, instance := range instances {
			return instance.Identity
		}
	}
	defaultID := domain.ResourceIdentity{APIVersion: "cdc.datascape.dev/v1alpha1", Kind: "CDCInstance", Namespace: resolved.Identity.Namespace, Name: "default-cdc", Target: resolved.Identity.Target}
	if _, ok := instances[defaultID.CanonicalString()]; ok {
		return defaultID
	}
	return domain.ResourceIdentity{}
}

func defaultCDCClass(target string) ir.CDCClassPlan {
	id := domain.ResourceIdentity{APIVersion: "cdc.datascape.dev/v1alpha1", Kind: "CDCClass", Namespace: api.DefaultNamespace, Name: "debezium-kafka-connect", Target: target}
	return ir.CDCClassPlan{
		Identity:            id,
		Engine:              "kafka-connect",
		ProviderInstance:    domain.ResourceIdentity{APIVersion: api.PlatformV1Alpha1, Kind: "ProviderInstance", Namespace: api.DefaultNamespace, Name: "local-cdc", Target: target},
		TargetCompatibility: []string{"compose"},
		Parameters:          map[string]any{"image": "quay.io/debezium/connect:3.6.0.Final"},
		WorkerConfiguration: map[string]any{},
	}
}

func defaultCDCInstance(target string) ir.CDCInstancePlan {
	class := defaultCDCClass(target)
	return ir.CDCInstancePlan{
		Identity:         domain.ResourceIdentity{APIVersion: "cdc.datascape.dev/v1alpha1", Kind: "CDCInstance", Namespace: api.DefaultNamespace, Name: "default-cdc", Target: target},
		Class:            class.Identity,
		Engine:           class.Engine,
		Ownership:        "managed",
		ManagementPolicy: "ManagedConnectors",
		ProviderInstance: class.ProviderInstance,
		Replicas:         1,
		Resources:        ir.ResourceRequirements{Memory: "1g", PidsLimit: 256},
		Parameters:       cloneMap(class.Parameters),
		State:            "satisfied",
	}
}

func defaultConnectorClassIdentity(target string) domain.ResourceIdentity {
	return domain.ResourceIdentity{APIVersion: "connections.datascape.dev/v1alpha1", Kind: "ConnectorClass", Namespace: api.DefaultNamespace, Name: "postgres-debezium", Target: target}
}

func relationalSourceExternal(source spec.Resource, byID map[string]spec.Resource, target string) bool {
	body, ok := resourceBody(source)
	if !ok {
		return false
	}
	connectionID := parseCDCRef(stringValue(body["connectionRef"]), source, "DatabaseConnection", target)
	connection, ok := byID[connectionID.CanonicalString()]
	if !ok {
		return false
	}
	connectionBody, _ := resourceBody(connection)
	if resourceOwnership(connection, connectionBody) == "external" || resourceOwnership(connection, connectionBody) == "imported" {
		return true
	}
	instanceID := parseCDCRef(stringValue(connectionBody["instanceRef"]), connection, "DatabaseInstance", target)
	if instanceID.Name == "" {
		return false
	}
	instance, ok := byID[instanceID.CanonicalString()]
	if !ok {
		return false
	}
	instanceBody, _ := resourceBody(instance)
	ownership := resourceOwnership(instance, instanceBody)
	return ownership == "external" || ownership == "imported"
}

func connectorState(instance ir.CDCInstancePlan) string {
	if instance.Ownership == "external" || instance.Ownership == "imported" {
		if instance.ManagementPolicy == "ObserveOnly" {
			return "externallyAppliedOrPending"
		}
		return "externallyManaged"
	}
	return "planned"
}

func parseCDCRef(value string, owner spec.Resource, expectedKind, target string) domain.ResourceIdentity {
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
	if kind == "StorageClass" || kind == "PersistentVolume" || kind == "DatabaseClass" || kind == "ConnectorClass" || kind == "CDCClass" {
		ns = api.DefaultNamespace
	}
	return domain.ResourceIdentity{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name, Target: target}
}

func endpointFromBody(body map[string]any) ir.EndpointPlan {
	return ir.EndpointPlan{Host: stringValue(body["host"]), Port: scalarString(body["port"]), URL: stringValue(body["url"])}
}

func endpointFromConnection(body map[string]any) ir.EndpointPlan {
	endpoint := endpointFromBody(anyMap(body["endpoint"]))
	if endpoint.Host == "" {
		endpoint.Host = stringValue(body["host"])
	}
	if endpoint.Port == "" {
		endpoint.Port = scalarString(body["port"])
	}
	if endpoint.URL == "" {
		endpoint.URL = stringValue(body["url"])
	}
	return endpoint
}

func resourcesFromBody(body map[string]any) ir.ResourceRequirements {
	return ir.ResourceRequirements{CPUs: stringValue(body["cpus"]), Memory: stringValue(body["memory"]), PidsLimit: intValue(body["pidsLimit"])}
}

func credentialEnvNames(ref domain.ResourceIdentity) map[string]string {
	if ref.Name == "" {
		return map[string]string{}
	}
	return map[string]string{
		"username": secretEnvNameForPlan(ref, "username"),
		"password": secretEnvNameForPlan(ref, "password"),
	}
}

func secretEnvNameForPlan(identity domain.ResourceIdentity, key string) string {
	return envNamePartForPlan(identity.Namespace) + "_" + envNamePartForPlan(identity.Name) + "_" + envNamePartForPlan(key)
}

func envNamePartForPlan(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r - ('a' - 'A'))
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func envReference(name string) string {
	if name == "" {
		return ""
	}
	return "${" + name + "}"
}

func cdcWorkerServiceName(id domain.ResourceIdentity) string {
	if id.Namespace == "" || id.Namespace == api.DefaultNamespace {
		return sanitizeName("cdc-" + id.Name)
	}
	return sanitizeName("cdc-" + id.Namespace + "-" + id.Name)
}

func cdcRegisterServiceName(connector ir.CDCConnectorPlan) string {
	return sanitizeName("cdc-register-" + connector.Binding.Namespace + "-" + connector.CDCInstance.Name + "-" + connector.Binding.Name)
}

func normalizeDatabaseEngine(value string) string {
	switch strings.ToLower(value) {
	case "postgres", "postgresql":
		return "postgresql"
	default:
		return strings.ToLower(value)
	}
}

func defaultDatabasePort(engine string) string {
	switch normalizeDatabaseEngine(engine) {
	case "postgresql":
		return "5432"
	default:
		return ""
	}
}

func anyMap(value any) map[string]any {
	body, _ := value.(map[string]any)
	if body == nil {
		return map[string]any{}
	}
	return body
}

func mergeAnyMaps(left, right map[string]any) map[string]any {
	out := cloneMap(left)
	for key, value := range right {
		out[key] = value
	}
	return out
}

func intValue(value any) int {
	switch typed := value.(type) {
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return 0
	}
}

func intDefault(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func scalarString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case int:
		return fmt.Sprint(typed)
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return ""
	}
}

func firstStringValue(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(body[key]); value != "" {
			return value
		}
	}
	return ""
}

func emptyConfigValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return typed == ""
	case []string:
		return len(typed) == 0
	default:
		return value == nil
	}
}

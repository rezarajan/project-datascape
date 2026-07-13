package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"datascape.dev/platformctl/internal/artifact"
	"datascape.dev/platformctl/internal/canonical"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
)

// Render generates the Compose target bundle from provider-owned planned resources.
func Render(ctx context.Context, plan ir.PlatformPlan) ([]artifact.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	planJSON, err := canonical.JSON(plan)
	if err != nil {
		return nil, err
	}
	resourcesJSON, err := canonical.JSON(plan.Resources)
	if err != nil {
		return nil, err
	}
	files := []artifact.File{
		text("README.md", readme(plan)),
		text(".env.example", envExample(plan)),
		text("compose.yaml", composeYAML(plan)),
		jsonFile("plan.json", planJSON),
		jsonFile("resources.json", resourcesJSON),
		jsonValue("resource-graph.json", plan.ResourceGraph),
		jsonValue("definitions/resources.json", plan.Definitions),
		jsonValue("definitions/bindings.json", bindingDefinitionProjection(plan)),
		jsonValue("providers/providers.json", plan.Providers),
		jsonValue("providers/instances.json", plan.ProviderInstances),
		jsonValue("providers/planned-resources.json", plan.PlannedResources),
		jsonValue("configuration/bindings/bindings.json", plan.Bindings),
		jsonValue("configuration/cdc/plan.json", plan.CDC),
		jsonValue("operations/plan.json", plan.Operations),
		jsonValue("operations/catalog.json", operationCatalog()),
		jsonValue("verification/checks.json", verificationChecks(plan)),
		text("verification/README.md", "# Verification\n\nRun `platformctl verify --bundle dist/local --runtime` after starting the generated Compose project.\n"),
		jsonValue("recovery/dependency-graph.json", plan.Recovery),
		jsonValue("storage/plan.json", plan.Storage),
	}
	files = append(files, bindingFiles(plan)...)
	files = append(files, cdcFiles(plan)...)
	files = append(files, operationFiles(plan)...)
	files = append(files, providerArtifactFiles(plan)...)
	return artifact.Normalize(files), nil
}

func composeYAML(plan ir.PlatformPlan) string {
	services := collectServices(plan)
	healthy := map[string]bool{}
	for _, item := range services {
		healthy[item.Name] = len(item.Healthcheck) > 0
	}
	var b strings.Builder
	b.WriteString("name: datascape\n")
	b.WriteString("services:\n")
	if len(services) == 0 {
		service(&b, "noop", ir.TargetServicePlan{Image: "alpine:3.20", Command: []string{"sh", "-c", "true"}}, healthy)
	} else {
		for _, servicePlan := range services {
			service(&b, servicePlan.Name, servicePlan, healthy)
		}
	}
	if len(plan.Verification.Checks) > 0 {
		verification := ir.TargetServicePlan{
			Name:    "verification",
			Image:   utilityImage(plan),
			Command: []string{"sh", "-c", "cat /verification/checks.json"},
			Volumes: []string{
				"./verification:/verification:ro",
			},
		}
		service(&b, "verification", verification, healthy)
	}
	volumes := collectVolumes(plan, services)
	if len(volumes) > 0 {
		b.WriteString("volumes:\n")
		for _, volume := range volumes {
			b.WriteString("  ")
			b.WriteString(volume.ComposeName)
			b.WriteString(":\n")
			if volume.External {
				b.WriteString("    external: true\n")
			}
			if volume.Driver != "" {
				b.WriteString("    driver: ")
				b.WriteString(quote(volume.Driver))
				b.WriteString("\n")
			}
			if len(volume.DriverOpts) > 0 {
				b.WriteString("    driver_opts:\n")
				keys := make([]string, 0, len(volume.DriverOpts))
				for key := range volume.DriverOpts {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					b.WriteString("      ")
					b.WriteString(key)
					b.WriteString(": ")
					b.WriteString(quote(volume.DriverOpts[key]))
					b.WriteString("\n")
				}
			}
		}
	}
	renderNamedFiles(&b, "secrets", services, func(service ir.TargetServicePlan) []string { return service.Secrets }, "./secrets/")
	renderNamedFiles(&b, "configs", services, func(service ir.TargetServicePlan) []string { return service.Configs }, "./configs/")
	b.WriteString("networks:\n  default:\n    name: datascape\n")
	return b.String()
}

func renderNamedFiles(b *strings.Builder, section string, services []ir.TargetServicePlan, values func(ir.TargetServicePlan) []string, prefix string) {
	names := make([]string, 0)
	for _, service := range services {
		names = append(names, values(service)...)
	}
	names = sortedUnique(names)
	if len(names) == 0 {
		return
	}
	b.WriteString(section)
	b.WriteString(":\n")
	for _, name := range names {
		b.WriteString("  ")
		b.WriteString(name)
		b.WriteString(":\n    file: ")
		b.WriteString(quote(prefix + name))
		b.WriteString("\n")
	}
}

func service(b *strings.Builder, name string, service ir.TargetServicePlan, healthy map[string]bool) {
	if service.Image == "" {
		service.Image = "alpine:3.20"
	}
	b.WriteString("  ")
	b.WriteString(name)
	b.WriteString(":\n")
	b.WriteString("    image: ")
	b.WriteString(service.Image)
	b.WriteString("\n")
	if service.Restart != "" {
		b.WriteString("    restart: ")
		b.WriteString(service.Restart)
		b.WriteString("\n")
	}
	if service.User != "" {
		b.WriteString("    user: ")
		b.WriteString(quote(service.User))
		b.WriteString("\n")
	}
	if service.Init {
		b.WriteString("    init: true\n")
	}
	if service.ReadOnly {
		b.WriteString("    read_only: true\n")
	}
	if service.StopGracePeriod != "" {
		b.WriteString("    stop_grace_period: ")
		b.WriteString(service.StopGracePeriod)
		b.WriteString("\n")
	}
	if service.CPUs != "" {
		b.WriteString("    cpus: ")
		b.WriteString(quote(service.CPUs))
		b.WriteString("\n")
	}
	if service.Memory != "" {
		b.WriteString("    mem_limit: ")
		b.WriteString(quote(service.Memory))
		b.WriteString("\n")
	}
	if service.PidsLimit > 0 {
		b.WriteString("    pids_limit: ")
		b.WriteString(strconv.Itoa(service.PidsLimit))
		b.WriteString("\n")
	}
	b.WriteString("    logging:\n      driver: json-file\n      options:\n        max-size: \"10m\"\n        max-file: \"3\"\n")
	if len(service.Entrypoint) > 0 {
		b.WriteString("    entrypoint: ")
		b.WriteString(jsonList(service.Entrypoint))
		b.WriteString("\n")
	}
	if len(service.Command) > 0 {
		b.WriteString("    command: ")
		b.WriteString(jsonList(service.Command))
		b.WriteString("\n")
	}
	if len(service.Environment) > 0 {
		b.WriteString("    environment:\n")
		keys := make([]string, 0, len(service.Environment))
		for key := range service.Environment {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			b.WriteString("      ")
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(quote(service.Environment[key]))
			b.WriteString("\n")
		}
	}
	if len(service.Ports) > 0 {
		b.WriteString("    ports:\n")
		for _, port := range service.Ports {
			b.WriteString("      - ")
			b.WriteString(quote(port))
			b.WriteString("\n")
		}
	}
	if len(service.Volumes) > 0 {
		b.WriteString("    volumes:\n")
		for _, volume := range service.Volumes {
			b.WriteString("      - ")
			b.WriteString(quote(volume))
			b.WriteString("\n")
		}
	}
	if len(service.DependsOn) > 0 {
		b.WriteString("    depends_on:\n")
		for _, dep := range service.DependsOn {
			b.WriteString("      ")
			b.WriteString(dep)
			if containsString(service.DependsOnCompleted, dep) {
				b.WriteString(":\n        condition: service_completed_successfully\n")
			} else if healthy[dep] {
				b.WriteString(":\n        condition: service_healthy\n")
			} else {
				b.WriteString(":\n        condition: service_started\n")
			}
		}
	}
	if len(service.Healthcheck) > 0 {
		b.WriteString("    healthcheck:\n")
		b.WriteString("      test: ")
		b.WriteString(jsonList(service.Healthcheck))
		b.WriteString("\n")
		b.WriteString("      interval: 10s\n")
		b.WriteString("      timeout: 5s\n")
		b.WriteString("      retries: 30\n")
	}
	listField(b, "cap_drop", service.CapDrop)
	listField(b, "security_opt", service.SecurityOpt)
	listField(b, "tmpfs", service.Tmpfs)
	listField(b, "secrets", service.Secrets)
	listField(b, "configs", service.Configs)
	listField(b, "profiles", service.Profiles)
}

func collectServices(plan ir.PlatformPlan) []ir.TargetServicePlan {
	byName := map[string]ir.TargetServicePlan{}
	for _, planned := range plan.PlannedResources {
		for _, service := range planned.Services {
			if service.Name == "" {
				continue
			}
			if existing, ok := byName[service.Name]; ok {
				existing.DependsOn = sortedUnique(append(existing.DependsOn, service.DependsOn...))
				existing.DependsOnCompleted = sortedUnique(append(existing.DependsOnCompleted, service.DependsOnCompleted...))
				existing.Volumes = sortedUnique(append(existing.Volumes, service.Volumes...))
				byName[service.Name] = existing
				continue
			}
			service.DependsOn = sortedUnique(service.DependsOn)
			service.DependsOnCompleted = sortedUnique(service.DependsOnCompleted)
			service.Volumes = sortedUnique(service.Volumes)
			byName[service.Name] = service
		}
	}
	for _, service := range cdcServices(plan, byName) {
		if existing, ok := byName[service.Name]; ok {
			existing.DependsOn = sortedUnique(append(existing.DependsOn, service.DependsOn...))
			existing.DependsOnCompleted = sortedUnique(append(existing.DependsOnCompleted, service.DependsOnCompleted...))
			existing.Volumes = sortedUnique(append(existing.Volumes, service.Volumes...))
			byName[service.Name] = existing
			continue
		}
		service.DependsOn = sortedUnique(service.DependsOn)
		service.DependsOnCompleted = sortedUnique(service.DependsOnCompleted)
		service.Volumes = sortedUnique(service.Volumes)
		byName[service.Name] = service
	}
	for _, mount := range plan.Storage.Mounts {
		service, ok := byName[mount.Workload.Name]
		if !ok {
			continue
		}
		volume, ok := storageVolume(plan, mount.Volume)
		if !ok {
			continue
		}
		source := volume.ComposeName
		if volume.HostPath != "" {
			source = volume.HostPath
		}
		entry := source + ":" + mount.Path
		if mount.ReadOnly {
			entry += ":ro"
		}
		service.Volumes = sortedUnique(append(service.Volumes, entry))
		byName[mount.Workload.Name] = service
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		if name == "runtime-utility" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]ir.TargetServicePlan, 0, len(names))
	for _, name := range names {
		out = append(out, byName[name])
	}
	return out
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func collectVolumes(plan ir.PlatformPlan, services []ir.TargetServicePlan) []ir.VolumePlan {
	byName := map[string]ir.VolumePlan{}
	for _, volume := range plan.Storage.Volumes {
		if volume.HostPath == "" {
			byName[volume.ComposeName] = volume
		}
	}
	for _, service := range services {
		for _, mount := range service.Volumes {
			left := strings.SplitN(mount, ":", 2)[0]
			if strings.HasPrefix(left, ".") || strings.HasPrefix(left, "/") {
				continue
			}
			if _, ok := byName[left]; !ok {
				byName[left] = ir.VolumePlan{ComposeName: left, Ownership: "managed"}
			}
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	volumes := make([]ir.VolumePlan, 0, len(names))
	for _, name := range names {
		volumes = append(volumes, byName[name])
	}
	return volumes
}

func storageVolume(plan ir.PlatformPlan, id domain.ResourceIdentity) (ir.VolumePlan, bool) {
	for _, volume := range plan.Storage.Volumes {
		if volume.Identity.CanonicalString() == id.CanonicalString() {
			return volume, true
		}
	}
	return ir.VolumePlan{}, false
}

func listField(b *strings.Builder, name string, values []string) {
	if len(values) == 0 {
		return
	}
	b.WriteString("    ")
	b.WriteString(name)
	b.WriteString(":\n")
	for _, value := range values {
		b.WriteString("      - ")
		b.WriteString(quote(value))
		b.WriteString("\n")
	}
}

func bindingFiles(plan ir.PlatformPlan) []artifact.File {
	files := make([]artifact.File, 0, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		files = append(files, jsonValue("configuration/bindings/"+binding.Identity.Name+".json", binding))
	}
	return files
}

func cdcFiles(plan ir.PlatformPlan) []artifact.File {
	files := make([]artifact.File, 0, len(plan.CDC.Connectors)+2)
	for _, connector := range plan.CDC.Connectors {
		files = append(files, jsonValue(connector.ConfigPath, connector.ProviderConfiguration))
		files = append(files, jsonValue("verification/cdc/"+connector.Binding.Namespace+"-"+connector.Binding.Name+".json", map[string]any{
			"binding":       connector.Binding.CanonicalString(),
			"cdcInstance":   connector.CDCInstance.CanonicalString(),
			"connectorName": connector.ConnectorName,
			"state":         connector.State,
			"checks":        connector.Verification,
		}))
	}
	if len(plan.CDC.Connectors) > 0 {
		files = append(files, text("operations/cdc/register_connector.py", registerConnectorScript))
	}
	return files
}

func operationFiles(plan ir.PlatformPlan) []artifact.File {
	files := make([]artifact.File, 0, len(plan.Operations))
	for _, operation := range plan.Operations {
		files = append(files, jsonValue("operations/requests/"+operation.Identity.Namespace+"-"+operation.Identity.Name+".json", operation))
	}
	return files
}

func cdcServices(plan ir.PlatformPlan, providerServices map[string]ir.TargetServicePlan) []ir.TargetServicePlan {
	byInstance := map[string][]ir.CDCConnectorPlan{}
	for _, connector := range plan.CDC.Connectors {
		byInstance[connector.CDCInstance.CanonicalString()] = append(byInstance[connector.CDCInstance.CanonicalString()], connector)
	}
	out := make([]ir.TargetServicePlan, 0)
	for _, instance := range plan.CDC.Instances {
		connectors := byInstance[instance.Identity.CanonicalString()]
		if len(connectors) == 0 {
			continue
		}
		workerName := cdcWorkerServiceName(instance.Identity)
		if instance.Ownership == "managed" {
			service := ir.TargetServicePlan{
				Name:            workerName,
				Capability:      "datascape.dev/source.change-stream",
				Image:           stringParam(instance.Parameters, "image", "quay.io/debezium/connect:3.6.0.Final"),
				Restart:         stringParam(instance.Parameters, "restart", "unless-stopped"),
				StopGracePeriod: stringParam(instance.Parameters, "stopGracePeriod", "30s"),
				Environment: map[string]string{
					"BOOTSTRAP_SERVERS":                 stringParam(instance.Parameters, "bootstrapServers", defaultBootstrapServers(providerServices)),
					"GROUP_ID":                          sanitizeName("datascape-" + instance.Identity.Namespace + "-" + instance.Identity.Name),
					"CONFIG_STORAGE_TOPIC":              sanitizeName("datascape-" + instance.Identity.Namespace + "-" + instance.Identity.Name + "-configs"),
					"OFFSET_STORAGE_TOPIC":              sanitizeName("datascape-" + instance.Identity.Namespace + "-" + instance.Identity.Name + "-offsets"),
					"STATUS_STORAGE_TOPIC":              sanitizeName("datascape-" + instance.Identity.Namespace + "-" + instance.Identity.Name + "-status"),
					"CONFIG_STORAGE_REPLICATION_FACTOR": "1",
					"OFFSET_STORAGE_REPLICATION_FACTOR": "1",
					"STATUS_STORAGE_REPLICATION_FACTOR": "1",
				},
				DependsOn:   cdcWorkerDependencies(connectors, providerServices),
				Healthcheck: []string{"CMD-SHELL", "curl -fsS http://localhost:8083/connectors"},
				SecurityOpt: []string{"no-new-privileges:true"},
				CPUs:        instance.Resources.CPUs,
				Memory:      instance.Resources.Memory,
				PidsLimit:   instance.Resources.PidsLimit,
			}
			if ports, ok := stringSliceAny(instance.Parameters["ports"]); ok {
				service.Ports = ports
			}
			out = append(out, service)
		}
		for _, connector := range connectors {
			if instance.Ownership != "managed" && instance.ManagementPolicy != "ManagedConnectors" {
				continue
			}
			endpoint := connectEndpoint(instance, workerName, connector.ConnectorName)
			env := map[string]string{
				"CONNECTOR_CONFIG": "/" + connector.ConfigPath,
				"CONNECTOR_NAME":   connector.ConnectorName,
				"CONNECT_URL":      endpoint,
			}
			for _, envName := range connector.CredentialEnvironment {
				if envName != "" {
					env[envName] = "${" + envName + ":?set " + envName + "}"
				}
			}
			out = append(out, ir.TargetServicePlan{
				Name:        cdcRegisterServiceName(connector),
				Capability:  "datascape.dev/source.change-stream",
				Image:       stringParam(instance.Parameters, "registrationImage", "python:3.13-alpine"),
				Command:     []string{"python", "/operations/cdc/register_connector.py"},
				DependsOn:   registerDependsOn(instance, workerName),
				Volumes:     []string{"./configuration/cdc:/configuration/cdc:ro", "./operations:/operations:ro"},
				Environment: env,
				SecurityOpt: []string{"no-new-privileges:true"},
				Memory:      "128m",
				PidsLimit:   32,
			})
		}
	}
	return out
}

func cdcWorkerDependencies(connectors []ir.CDCConnectorPlan, providerServices map[string]ir.TargetServicePlan) []string {
	values := make([]string, 0)
	for _, connector := range connectors {
		if connector.DatabaseEndpoint.Host != "" {
			if _, ok := providerServices[connector.DatabaseEndpoint.Host]; ok {
				values = append(values, connector.DatabaseEndpoint.Host)
			}
		}
		if stream := streamServiceName(connector.DestinationStream, providerServices); stream != "" {
			values = append(values, stream)
		}
	}
	return sortedUnique(values)
}

func registerDependsOn(instance ir.CDCInstancePlan, workerName string) []string {
	if instance.Ownership == "managed" {
		return []string{workerName}
	}
	return nil
}

func connectEndpoint(instance ir.CDCInstancePlan, workerName, connectorName string) string {
	if instance.Ownership == "managed" {
		return "http://" + workerName + ":8083/connectors/" + connectorName + "/config"
	}
	if instance.Endpoint.URL != "" {
		return strings.TrimRight(instance.Endpoint.URL, "/") + "/connectors/" + connectorName + "/config"
	}
	if instance.Endpoint.Host != "" {
		port := instance.Endpoint.Port
		if port == "" {
			port = "8083"
		}
		return "http://" + instance.Endpoint.Host + ":" + port + "/connectors/" + connectorName + "/config"
	}
	return "http://external-cdc:8083/connectors/" + connectorName + "/config"
}

func streamServiceName(stream domain.ResourceIdentity, providerServices map[string]ir.TargetServicePlan) string {
	if _, ok := providerServices[stream.Name]; ok {
		return stream.Name
	}
	if _, ok := providerServices["event-stream"]; ok {
		return "event-stream"
	}
	for name, service := range providerServices {
		if service.Capability == "datascape.dev/stream.publish" {
			return name
		}
	}
	return ""
}

func defaultBootstrapServers(providerServices map[string]ir.TargetServicePlan) string {
	if _, ok := providerServices["event-stream"]; ok {
		return "event-stream:9092"
	}
	for name, service := range providerServices {
		if service.Capability == "datascape.dev/stream.publish" {
			return name + ":9092"
		}
	}
	return "event-stream:9092"
}

func providerArtifactFiles(plan ir.PlatformPlan) []artifact.File {
	files := make([]artifact.File, 0)
	for _, planned := range plan.PlannedResources {
		for _, providerArtifact := range planned.Artifacts {
			if providerArtifact.Path == "" {
				continue
			}
			content := providerArtifact.Content
			if len(content) == 0 {
				content = map[string]any{"capability": providerArtifact.Capability, "providerInstance": planned.ProviderInstance.CanonicalString()}
			}
			files = append(files, jsonValue(providerArtifact.Path, content))
		}
	}
	return files
}

func verificationChecks(plan ir.PlatformPlan) []map[string]any {
	out := make([]map[string]any, 0, len(plan.Verification.Checks))
	for _, check := range plan.Verification.Checks {
		out = append(out, map[string]any{
			"id":          check.ID,
			"status":      "not-run",
			"message":     check.Description,
			"evidenceRef": "execution-evidence/" + check.ID + ".json",
			"duration":    "0s",
			"remediation": "run platformctl verify --bundle dist/local --runtime",
		})
	}
	return out
}

func bindingDefinitionProjection(plan ir.PlatformPlan) []map[string]any {
	out := make([]map[string]any, 0, len(plan.Bindings))
	seen := map[string]bool{}
	for _, binding := range plan.Bindings {
		key := binding.Definition.CanonicalString()
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, map[string]any{"identity": key, "capability": binding.Capability})
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, _ := out[i]["identity"].(string)
		right, _ := out[j]["identity"].(string)
		return left < right
	})
	return out
}

func utilityImage(plan ir.PlatformPlan) string {
	for _, planned := range plan.PlannedResources {
		if planned.Capability != "datascape.dev/runtime.utility" {
			continue
		}
		for _, service := range planned.Services {
			if service.Image != "" {
				return service.Image
			}
		}
	}
	return "alpine:3.20"
}

func readme(plan ir.PlatformPlan) string {
	var b strings.Builder
	b.WriteString("# Datascape Compose Bundle\n\nRun:\n\n```sh\ncp .env.example .env\ndocker compose -f compose.yaml up --wait\nplatformctl verify --bundle . --runtime\n```\n\n")
	b.WriteString("This bundle is deterministic. Services are selected by provider instances that satisfy declared resource and binding capabilities.\n")
	if len(plan.PlannedResources) > 0 {
		b.WriteString("\n## Planned Provider Resources\n\n")
		for _, item := range plan.PlannedResources {
			b.WriteString("- `")
			b.WriteString(item.Capability)
			b.WriteString("` via ")
			b.WriteString(item.ProviderInstance.Display())
			b.WriteString("\n")
		}
	}
	return b.String()
}

func envExample(plan ir.PlatformPlan) string {
	values := map[string]string{
		"DATASCAPE_SOURCE_PASSWORD": "change-me-local",
		"MINIO_ROOT_PASSWORD":       "change-me-local",
		"MINIO_ROOT_USER":           "datascape",
	}
	for _, resource := range plan.Resources {
		if resource.Kind != "SecretReference" || resource.SecretBackend != "env" {
			continue
		}
		for _, key := range resource.SecretKeys {
			values[secretEnvName(resource.Identity, key)] = "change-me-local"
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteString("=")
		b.WriteString(values[key])
		b.WriteString("\n")
	}
	return b.String()
}

func secretEnvName(identity domain.ResourceIdentity, key string) string {
	parts := []string{identity.Namespace, identity.Name, key}
	for i, part := range parts {
		parts[i] = envNamePart(part)
	}
	return strings.Join(parts, "_")
}

func envNamePart(value string) string {
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

func stringParam(values map[string]any, key, fallback string) string {
	if s, ok := values[key].(string); ok && s != "" {
		return s
	}
	return fallback
}

func stringSliceAny(value any) ([]string, bool) {
	values, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if s, ok := value.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return sortedUnique(out), len(out) > 0
}

func cdcWorkerServiceName(id domain.ResourceIdentity) string {
	if id.Namespace == "" || id.Namespace == "default" {
		return sanitizeName("cdc-" + id.Name)
	}
	return sanitizeName("cdc-" + id.Namespace + "-" + id.Name)
}

func cdcRegisterServiceName(connector ir.CDCConnectorPlan) string {
	return sanitizeName("cdc-register-" + connector.Binding.Namespace + "-" + connector.CDCInstance.Name + "-" + connector.Binding.Name)
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

func operationCatalog() []map[string]any {
	actions := []string{
		"CreateOrReconcileConnector", "UpdateConnectorConfig", "PauseConnector", "ResumeConnector", "RestartConnector",
		"RestartFailedTasks", "DeleteConnector", "MoveConnector", "ChangeTableFilters", "IncrementalSnapshot",
		"Resnapshot", "ValidateConnectivity", "VerifyConnector", "InspectOffsets", "ExportOffsets", "ImportOffsets",
		"ResetOffsets", "ScaleWorker", "UpgradeWorker", "BackupPlan", "RestorePlan", "RotateDatabaseCredentials",
		"RotateCDCControlCredentials", "RotateStreamCredentials", "AdoptConnector", "DetachConnector", "DeleteCDCInstance",
	}
	out := make([]map[string]any, 0, len(actions))
	for _, action := range actions {
		out = append(out, map[string]any{
			"name":                 action,
			"applicableKinds":      []string{"CDCBinding", "CDCInstance"},
			"parameterSchema":      map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "additionalProperties": true},
			"mutatesExternalState": action != "ValidateConnectivity" && action != "VerifyConnector" && action != "InspectOffsets" && action != "BackupPlan" && action != "RestorePlan",
			"destructive":          action == "ResetOffsets" || action == "DeleteConnector" || action == "DeleteCDCInstance",
			"idempotent":           true,
			"targetCompatibility":  []string{"compose"},
			"executionContract":    "provider-adapter/v1alpha1",
			"verificationContract": "verification-checks/v1alpha1",
			"rollbackContract":     "recovery-plan/v1alpha1",
		})
	}
	return out
}

func jsonList(values []string) string {
	content, err := json.Marshal(values)
	if err != nil {
		return "[]"
	}
	return string(content)
}

func quote(value string) string {
	content, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(content)
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

func text(path, content string) artifact.File {
	return artifact.File{Path: path, Mode: 0o644, Content: []byte(content), Deterministic: true}
}

func jsonValue(path string, value any) artifact.File {
	content, err := canonical.JSON(value)
	if err != nil {
		content = []byte(`{"error":"canonicalization failed"}`)
	}
	return jsonFile(path, content)
}

func jsonFile(path string, content []byte) artifact.File {
	content = append(bytes.TrimSpace(content), '\n')
	return artifact.File{Path: path, Mode: 0o644, Content: content, Deterministic: true}
}

const registerConnectorScript = `"""Idempotently register a generated CDC connector configuration."""

import json
import os
import time
import urllib.error
import urllib.request


def expanded(value):
    if isinstance(value, str):
        return os.path.expandvars(value)
    if isinstance(value, dict):
        return {k: expanded(v) for k, v in value.items()}
    if isinstance(value, list):
        return [expanded(v) for v in value]
    return value


def main() -> None:
    connector_url = os.environ["CONNECT_URL"]
    config_path = os.environ["CONNECTOR_CONFIG"]
    with open(config_path, "r", encoding="utf-8") as handle:
        body = json.dumps(expanded(json.load(handle))).encode("utf-8")
    headers = {"Content-Type": "application/json"}
    token = os.environ.get("CDC_CONTROL_TOKEN")
    if token:
        headers["Authorization"] = "Bearer " + token
    for attempt in range(60):
        request = urllib.request.Request(connector_url, data=body, headers=headers, method="PUT")
        try:
            with urllib.request.urlopen(request, timeout=5) as response:
                if response.status < 300:
                    return
        except (urllib.error.URLError, TimeoutError):
            if attempt == 59:
                raise
            time.sleep(2)
    raise RuntimeError("CDC runtime did not accept the connector configuration")


if __name__ == "__main__":
    main()
`

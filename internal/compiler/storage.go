package compiler

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"datascape.dev/platformctl/internal/api"
	"datascape.dev/platformctl/internal/domain"
	"datascape.dev/platformctl/internal/ir"
	"datascape.dev/platformctl/internal/spec"
)

func buildStoragePlan(resources []spec.Resource, target string) (ir.StoragePlan, []domain.Diagnostic) {
	plan := ir.StoragePlan{}
	diags := make([]domain.Diagnostic, 0)
	classes := map[string]ir.StorageClassPlan{}
	volumes := map[string]ir.VolumePlan{}
	claims := map[string]ir.VolumeClaimPlan{}
	resourceByID := map[string]spec.Resource{}
	defaultClasses := make([]domain.ResourceIdentity, 0)

	for _, res := range resources {
		resourceByID[res.Identity(target, "").CanonicalString()] = res
		body, ok := resourceBody(res)
		if !ok {
			continue
		}
		switch res.Kind {
		case "StorageClass":
			compatibility := stringSlice(body["targetCompatibility"])
			if len(compatibility) > 0 && !containsValue(compatibility, target) {
				diags = append(diags, storageDiag(res, "DSTOR007", "spec.targetCompatibility", "storage class is not compatible with target "+target, "choose a compatible StorageClass or target"))
			}
			parameters, _ := body["parameters"].(map[string]any)
			class := ir.StorageClassPlan{
				Identity: res.Identity(target, ""), Provisioner: stringValue(body["provisioner"]),
				TargetCompatibility: compatibility, Parameters: cloneMap(parameters),
				ReclaimPolicy:        stringDefault(stringValue(body["reclaimPolicy"]), "Retain"),
				VolumeBindingMode:    stringDefault(stringValue(body["volumeBindingMode"]), "Immediate"),
				AllowVolumeExpansion: boolValue(body["allowVolumeExpansion"], false),
				Default:              boolValue(body["default"], false),
			}
			classes[class.Identity.CanonicalString()] = class
			plan.Classes = append(plan.Classes, class)
			if class.Default {
				defaultClasses = append(defaultClasses, class.Identity)
			}
		case "PersistentVolume":
			classID := parseStorageRef(stringValue(body["storageClassRef"]), res, "StorageClass", target)
			source, _ := body["source"].(map[string]any)
			composeName := stringValue(source["volumeName"])
			if composeName == "" {
				composeName = sanitizeName(res.Metadata.Name)
			}
			volume := ir.VolumePlan{
				Identity: res.Identity(target, ""), Class: classID, Capacity: stringValue(body["capacity"]),
				AccessModes: stringSlice(body["accessModes"]), Ownership: stringDefault(stringValue(body["ownership"]), "managed"),
				ComposeName: composeName, External: boolValue(source["external"], false),
				Driver: stringValue(source["driver"]), DriverOpts: stringMap(source["driverOpts"]),
				HostPath: stringValue(source["hostPath"]),
			}
			if target == "compose" && volume.HostPath != "" && (!strings.HasPrefix(volume.HostPath, "./") || strings.Contains(filepath.Clean(volume.HostPath), "..")) {
				diags = append(diags, storageDiag(res, "DSTOR009", "spec.source.hostPath", "Compose bind path must be a bundle-relative path without traversal", "use a path below the generated bundle such as ./state/data"))
			}
			volumes[volume.Identity.CanonicalString()] = volume
			plan.Volumes = append(plan.Volumes, volume)
		}
	}

	if len(defaultClasses) > 1 {
		diags = append(diags, domain.Diagnostic{Severity: domain.SeverityError, Code: "DSTOR005", FieldPath: "spec.default", Message: "multiple StorageClass resources are marked as default", Remediation: "mark exactly one StorageClass as default"})
	}
	claimed := map[string]bool{}
	for _, res := range resources {
		if res.Kind != "PersistentVolumeClaim" {
			continue
		}
		body, _ := resourceBody(res)
		classID := parseStorageRef(stringValue(body["storageClassRef"]), res, "StorageClass", target)
		if classID.Name == "" && len(defaultClasses) == 1 {
			classID = defaultClasses[0]
		}
		if classID.Name == "" {
			diags = append(diags, storageDiag(res, "DSTOR006", "spec.storageClassRef", "claim has no StorageClass and there is no unique default", "set storageClassRef or mark one StorageClass as default"))
			continue
		}
		class, classOK := classes[classID.CanonicalString()]
		if !classOK {
			diags = append(diags, storageDiag(res, "DREF002", "spec.storageClassRef", "referenced StorageClass does not exist", "declare the StorageClass or correct the reference"))
			continue
		}
		claim := ir.VolumeClaimPlan{Identity: res.Identity(target, ""), Class: classID, Capacity: stringValue(body["capacity"]), AccessModes: stringSlice(body["accessModes"])}
		explicit := parseStorageRef(stringValue(body["volumeRef"]), res, "PersistentVolume", target)
		if explicit.Name != "" {
			volume, ok := volumes[explicit.CanonicalString()]
			if !ok {
				diags = append(diags, storageDiag(res, "DREF002", "spec.volumeRef", "referenced PersistentVolume does not exist", "declare the volume or correct volumeRef"))
				continue
			}
			if claimed[volume.Identity.CanonicalString()] {
				diags = append(diags, storageDiag(res, "DSTOR014", "spec.volumeRef", "PersistentVolume is already bound to another claim", "bind each PersistentVolume to exactly one PersistentVolumeClaim"))
				continue
			}
			if volume.Class.CanonicalString() != classID.CanonicalString() {
				diags = append(diags, storageDiag(res, "DSTOR011", "spec.volumeRef", "PersistentVolume and claim use different StorageClass resources", "select a volume from the claim's StorageClass"))
				continue
			}
			if !capacityAtLeast(volume.Capacity, claim.Capacity) {
				diags = append(diags, storageDiag(res, "DSTOR012", "spec.capacity", "PersistentVolume capacity is smaller than the claim request", "select a larger volume or reduce the requested capacity"))
				continue
			}
			if !modesInclude(volume.AccessModes, claim.AccessModes) {
				diags = append(diags, storageDiag(res, "DSTOR013", "spec.accessModes", "PersistentVolume does not provide every requested access mode", "align the claim access modes with the selected volume"))
				continue
			}
			claim.BoundVolume = volume.Identity
			claimed[volume.Identity.CanonicalString()] = true
		} else {
			for _, candidate := range plan.Volumes {
				if claimed[candidate.Identity.CanonicalString()] || candidate.Class.CanonicalString() != classID.CanonicalString() || !capacityAtLeast(candidate.Capacity, claim.Capacity) || !modesInclude(candidate.AccessModes, claim.AccessModes) {
					continue
				}
				claim.BoundVolume = candidate.Identity
				claimed[candidate.Identity.CanonicalString()] = true
				break
			}
		}
		if claim.BoundVolume.Name == "" {
			if class.Provisioner != "compose.named" {
				diags = append(diags, storageDiag(res, "DSTOR008", "spec.storageClassRef", "storage class cannot dynamically provision a Compose volume", "declare a matching PersistentVolume or use the compose.named provisioner"))
				continue
			}
			volume := ir.VolumePlan{
				Identity: domain.ResourceIdentity{APIVersion: "storage.datascape.dev/v1alpha1", Kind: "PersistentVolume", Namespace: api.DefaultNamespace, Name: "pvc-" + defaultNamespace(res.Metadata.Namespace) + "-" + res.Metadata.Name, Target: target},
				Class:    classID, Capacity: claim.Capacity, AccessModes: append([]string{}, claim.AccessModes...), Ownership: "managed",
				ComposeName: sanitizeName("pvc-" + defaultNamespace(res.Metadata.Namespace) + "-" + res.Metadata.Name), Dynamic: true,
			}
			claim.BoundVolume = volume.Identity
			volumes[volume.Identity.CanonicalString()] = volume
			plan.Volumes = append(plan.Volumes, volume)
			claimed[volume.Identity.CanonicalString()] = true
		}
		claims[claim.Identity.CanonicalString()] = claim
		plan.Claims = append(plan.Claims, claim)
	}

	for _, res := range resources {
		if res.Kind != "VolumeMountBinding" {
			continue
		}
		body, _ := resourceBody(res)
		claimID := parseStorageRef(stringValue(body["claimRef"]), res, "PersistentVolumeClaim", target)
		claim, ok := claims[claimID.CanonicalString()]
		if !ok {
			continue
		}
		workload := parseStorageRef(stringValue(body["workloadRef"]), res, "", target)
		if _, ok := resourceByID[workload.CanonicalString()]; !ok {
			continue
		}
		mountPath := stringValue(body["mountPath"])
		if !filepath.IsAbs(mountPath) || strings.Contains(filepath.Clean(mountPath), "..") {
			diags = append(diags, storageDiag(res, "DSTOR010", "spec.mountPath", "mountPath must be an absolute container path without traversal", "use a path such as /var/lib/data"))
			continue
		}
		plan.Mounts = append(plan.Mounts, ir.VolumeMountPlan{Identity: res.Identity(target, ""), Claim: claimID, Workload: workload, Volume: claim.BoundVolume, Path: mountPath, ReadOnly: boolValue(body["readOnly"], false)})
	}

	sort.SliceStable(plan.Classes, func(i, j int) bool {
		return plan.Classes[i].Identity.CanonicalString() < plan.Classes[j].Identity.CanonicalString()
	})
	sort.SliceStable(plan.Volumes, func(i, j int) bool {
		return plan.Volumes[i].Identity.CanonicalString() < plan.Volumes[j].Identity.CanonicalString()
	})
	sort.SliceStable(plan.Claims, func(i, j int) bool {
		return plan.Claims[i].Identity.CanonicalString() < plan.Claims[j].Identity.CanonicalString()
	})
	sort.SliceStable(plan.Mounts, func(i, j int) bool {
		return plan.Mounts[i].Identity.CanonicalString() < plan.Mounts[j].Identity.CanonicalString()
	})
	return plan, diags
}

func parseStorageRef(value string, owner spec.Resource, expectedKind, target string) domain.ResourceIdentity {
	if value == "" {
		return domain.ResourceIdentity{}
	}
	parts := strings.Split(value, "/")
	ns := defaultNamespace(owner.Metadata.Namespace)
	kind, name, apiVersion := expectedKind, "", apiVersionForStorageKind(expectedKind)
	switch len(parts) {
	case 2:
		kind, name = parts[0], parts[1]
		apiVersion = apiVersionForStorageKind(kind)
	case 3:
		kind, ns, name = parts[0], parts[1], parts[2]
		apiVersion = apiVersionForStorageKind(kind)
	case 5:
		apiVersion, kind, ns, name = parts[0]+"/"+parts[1], parts[2], parts[3], parts[4]
	default:
		return domain.ResourceIdentity{}
	}
	if clusterStorageKind(kind) {
		ns = api.DefaultNamespace
	}
	return domain.ResourceIdentity{APIVersion: apiVersion, Kind: kind, Namespace: ns, Name: name, Target: target}
}

func apiVersionForStorageKind(kind string) string {
	switch kind {
	case "StorageClass", "PersistentVolume", "PersistentVolumeClaim":
		return "storage.datascape.dev/v1alpha1"
	case "DatabaseClass", "DatabaseInstance":
		return "databases.datascape.dev/v1alpha1"
	case "ConnectorClass", "DatabaseConnection":
		return "connections.datascape.dev/v1alpha1"
	case "RelationalSource":
		return "sources.datascape.dev/v1alpha1"
	case "EventStream":
		return "streams.datascape.dev/v1alpha1"
	case "ObjectStore", "Warehouse":
		return "stores.datascape.dev/v1alpha1"
	case "Pipeline":
		return "pipelines.datascape.dev/v1alpha1"
	case "TableCatalog", "MetadataCatalog":
		return "catalogs.datascape.dev/v1alpha1"
	case "QueryEngine":
		return "compute.datascape.dev/v1alpha1"
	default:
		return api.PlatformV1Alpha1
	}
}

func clusterStorageKind(kind string) bool {
	return kind == "StorageClass" || kind == "PersistentVolume" || kind == "DatabaseClass" || kind == "ConnectorClass"
}

func storageDiag(res spec.Resource, code, field, message, remediation string) domain.Diagnostic {
	return domain.Diagnostic{Severity: domain.SeverityError, Code: code, Resource: res.Identity("", "").Display(), FieldPath: field, Message: message, Remediation: remediation, Location: res.Location}
}

func containsValue(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func stringMap(value any) map[string]string {
	body, _ := value.(map[string]any)
	out := map[string]string{}
	for key, item := range body {
		if text, ok := item.(string); ok {
			out[key] = text
		}
	}
	return out
}

func parseQuantity(value string) int64 {
	units := map[string]int64{"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40, "Pi": 1 << 50}
	for suffix, multiplier := range units {
		if strings.HasSuffix(value, suffix) {
			n, _ := strconv.ParseInt(strings.TrimSuffix(value, suffix), 10, 64)
			return n * multiplier
		}
	}
	return 0
}

func capacityAtLeast(volume, claim string) bool { return parseQuantity(volume) >= parseQuantity(claim) }

func modesInclude(available, requested []string) bool {
	for _, want := range requested {
		if !containsValue(available, want) {
			return false
		}
	}
	return true
}

func volumeByIdentity(plan ir.StoragePlan, id domain.ResourceIdentity) (ir.VolumePlan, bool) {
	for _, volume := range plan.Volumes {
		if volume.Identity.CanonicalString() == id.CanonicalString() {
			return volume, true
		}
	}
	return ir.VolumePlan{}, false
}

func storageSummary(plan ir.StoragePlan) string {
	return fmt.Sprintf("%d classes, %d volumes, %d claims, %d mounts", len(plan.Classes), len(plan.Volumes), len(plan.Claims), len(plan.Mounts))
}

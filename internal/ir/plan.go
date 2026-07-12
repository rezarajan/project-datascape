package ir

import "datascape.dev/platformctl/internal/domain"

// PlatformPlan is the normalized target-neutral intermediate representation.
type PlatformPlan struct {
	APIVersion        string                  `json:"apiVersion"`
	Identity          domain.ResourceIdentity `json:"identity,omitempty"`
	Target            string                  `json:"target"`
	TargetPlan        TargetPlan              `json:"targetPlan"`
	Definitions       []DefinitionPlan        `json:"definitions,omitempty"`
	Providers         []ProviderPlan          `json:"providers,omitempty"`
	ProviderInstances []ProviderInstancePlan  `json:"providerInstances,omitempty"`
	PlannedResources  []ProviderResourcePlan  `json:"plannedResources,omitempty"`
	ResourceGraph     ResourceGraphPlan       `json:"resourceGraph,omitempty"`
	Bindings          []BindingPlan           `json:"bindings,omitempty"`
	External          []ExternalResourcePlan  `json:"externalResources,omitempty"`
	Policies          []PolicyPlan            `json:"policies,omitempty"`
	Overrides         []OverridePlan          `json:"overrides,omitempty"`
	Verification      VerificationPlan        `json:"verification,omitempty"`
	Recovery          RecoveryPlan            `json:"recovery,omitempty"`
	Storage           StoragePlan             `json:"storage,omitempty"`
	Resources         []ResourcePlan          `json:"resources"`
}

type StoragePlan struct {
	Classes []StorageClassPlan `json:"classes,omitempty"`
	Volumes []VolumePlan       `json:"volumes,omitempty"`
	Claims  []VolumeClaimPlan  `json:"claims,omitempty"`
	Mounts  []VolumeMountPlan  `json:"mounts,omitempty"`
}

type StorageClassPlan struct {
	Identity             domain.ResourceIdentity `json:"identity"`
	Provisioner          string                  `json:"provisioner"`
	TargetCompatibility  []string                `json:"targetCompatibility,omitempty"`
	Parameters           map[string]any          `json:"parameters,omitempty"`
	ReclaimPolicy        string                  `json:"reclaimPolicy"`
	VolumeBindingMode    string                  `json:"volumeBindingMode"`
	AllowVolumeExpansion bool                    `json:"allowVolumeExpansion,omitempty"`
	Default              bool                    `json:"default,omitempty"`
}

type VolumePlan struct {
	Identity    domain.ResourceIdentity `json:"identity"`
	Class       domain.ResourceIdentity `json:"storageClass"`
	Capacity    string                  `json:"capacity"`
	AccessModes []string                `json:"accessModes,omitempty"`
	Ownership   string                  `json:"ownership"`
	ComposeName string                  `json:"composeName"`
	External    bool                    `json:"external,omitempty"`
	Driver      string                  `json:"driver,omitempty"`
	DriverOpts  map[string]string       `json:"driverOpts,omitempty"`
	HostPath    string                  `json:"hostPath,omitempty"`
	Dynamic     bool                    `json:"dynamic,omitempty"`
}

type VolumeClaimPlan struct {
	Identity    domain.ResourceIdentity `json:"identity"`
	Class       domain.ResourceIdentity `json:"storageClass"`
	Capacity    string                  `json:"capacity"`
	AccessModes []string                `json:"accessModes,omitempty"`
	BoundVolume domain.ResourceIdentity `json:"boundVolume"`
}

type VolumeMountPlan struct {
	Identity domain.ResourceIdentity `json:"identity"`
	Claim    domain.ResourceIdentity `json:"claim"`
	Workload domain.ResourceIdentity `json:"workload"`
	Volume   domain.ResourceIdentity `json:"volume"`
	Path     string                  `json:"path"`
	ReadOnly bool                    `json:"readOnly,omitempty"`
}

type TargetPlan struct {
	Type                string `json:"type"`
	Profile             string `json:"profile,omitempty"`
	AvailabilityClass   string `json:"availabilityClass,omitempty"`
	DevelopmentMode     bool   `json:"developmentMode,omitempty"`
	AllowUnpinnedImages bool   `json:"allowUnpinnedImages,omitempty"`
}

type DefinitionPlan struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	APIVersion   string                  `json:"apiVersion"`
	Kind         string                  `json:"kind"`
	Scope        string                  `json:"scope"`
	Category     string                  `json:"category,omitempty"`
	ProviderType string                  `json:"providerType,omitempty"`
	Capabilities []string                `json:"capabilities,omitempty"`
	BindingRoles []string                `json:"bindingRoles,omitempty"`
	Core         bool                    `json:"core,omitempty"`
	Extension    bool                    `json:"extension,omitempty"`
}

type ProviderPlan struct {
	Identity            domain.ResourceIdentity `json:"identity"`
	Type                string                  `json:"type"`
	Capabilities        []string                `json:"capabilities"`
	BindingKinds        []string                `json:"bindingKinds,omitempty"`
	TargetCompatibility []string                `json:"targetCompatibility,omitempty"`
	RuntimeDependencies []string                `json:"runtimeDependencies,omitempty"`
	Services            []TargetServicePlan     `json:"services,omitempty"`
	Artifacts           []ProviderArtifactPlan  `json:"artifacts,omitempty"`
	RendererContract    string                  `json:"rendererContract,omitempty"`
	Conformance         []string                `json:"conformance,omitempty"`
	PackageVersion      string                  `json:"packageVersion,omitempty"`
	ContractVersion     string                  `json:"contractVersion,omitempty"`
	PackageDigest       string                  `json:"packageDigest,omitempty"`
	Provenance          string                  `json:"provenance,omitempty"`
}

type ProviderInstancePlan struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	Provider     domain.ResourceIdentity `json:"provider"`
	Type         string                  `json:"type"`
	Target       string                  `json:"target,omitempty"`
	Capabilities []string                `json:"capabilities"`
	Parameters   map[string]any          `json:"parameters,omitempty"`
}

type ProviderResourcePlan struct {
	Identity         domain.ResourceIdentity   `json:"identity"`
	Capability       string                    `json:"capability"`
	ProviderInstance domain.ResourceIdentity   `json:"providerInstance"`
	Provider         domain.ResourceIdentity   `json:"provider"`
	Reason           string                    `json:"reason,omitempty"`
	Services         []TargetServicePlan       `json:"services,omitempty"`
	Artifacts        []ProviderArtifactPlan    `json:"artifacts,omitempty"`
	DependsOn        []domain.ResourceIdentity `json:"dependsOn,omitempty"`
}

type TargetServicePlan struct {
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

type ProviderArtifactPlan struct {
	Path       string         `json:"path"`
	Capability string         `json:"capability,omitempty"`
	Content    map[string]any `json:"content,omitempty"`
}

type ResourceGraphPlan struct {
	ValidationMode string                 `json:"validationMode"`
	Nodes          []ResourceGraphNode    `json:"nodes,omitempty"`
	Bindings       []BindingPlan          `json:"bindings,omitempty"`
	External       []ExternalResourcePlan `json:"externalResources,omitempty"`
	Policies       []PolicyPlan           `json:"policies,omitempty"`
	Overrides      []OverridePlan         `json:"overrides,omitempty"`
}

type ResourceGraphNode struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	Kind         string                  `json:"kind"`
	Ownership    string                  `json:"ownership"`
	Lifecycle    string                  `json:"lifecycle,omitempty"`
	State        string                  `json:"state"`
	Capabilities []string                `json:"capabilities,omitempty"`
	Verification []VerificationCheck     `json:"verification,omitempty"`
}

type BindingPlan struct {
	Identity          domain.ResourceIdentity   `json:"identity"`
	Kind              string                    `json:"kind"`
	Definition        domain.ResourceIdentity   `json:"definition,omitempty"`
	Capability        string                    `json:"capability,omitempty"`
	Source            domain.ResourceIdentity   `json:"source,omitempty"`
	Target            domain.ResourceIdentity   `json:"target,omitempty"`
	ProviderInstance  domain.ResourceIdentity   `json:"providerInstance,omitempty"`
	Mode              string                    `json:"mode,omitempty"`
	Ownership         string                    `json:"ownership,omitempty"`
	State             string                    `json:"state"`
	Dependencies      []domain.ResourceIdentity `json:"dependencies,omitempty"`
	DependencyClosure []string                  `json:"dependencyClosure,omitempty"`
	Digest            string                    `json:"digest,omitempty"`
	Generated         bool                      `json:"generated,omitempty"`
}

type ExternalResourcePlan struct {
	Identity     domain.ResourceIdentity `json:"identity"`
	Kind         string                  `json:"kind"`
	Capability   string                  `json:"capability"`
	Interface    string                  `json:"interface,omitempty"`
	TrustPolicy  string                  `json:"trustPolicy,omitempty"`
	State        string                  `json:"state"`
	Verification []VerificationCheck     `json:"verification,omitempty"`
}

type PolicyPlan struct {
	Identity                    domain.ResourceIdentity `json:"identity"`
	ValidationMode              string                  `json:"validationMode"`
	RequireLineage              bool                    `json:"requireLineage,omitempty"`
	RequireAudit                bool                    `json:"requireAudit,omitempty"`
	RequireIdempotency          bool                    `json:"requireIdempotency,omitempty"`
	AllowExternalOwnership      bool                    `json:"allowExternalOwnership,omitempty"`
	AllowDeferrals              bool                    `json:"allowDeferrals,omitempty"`
	AllowOverrides              bool                    `json:"allowOverrides,omitempty"`
	AllowExternalTrustOverrides bool                    `json:"allowExternalTrustOverrides,omitempty"`
}

type OverridePlan struct {
	Name        string                  `json:"name"`
	Scope       string                  `json:"scope"`
	Resource    domain.ResourceIdentity `json:"resource,omitempty"`
	Policy      string                  `json:"policy,omitempty"`
	Reason      string                  `json:"reason,omitempty"`
	Remediation string                  `json:"remediation,omitempty"`
}

type VerificationPlan struct {
	PolicyRef domain.ResourceIdentity `json:"policyRef,omitempty"`
	Checks    []VerificationCheck     `json:"checks,omitempty"`
}

type VerificationCheck struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type RecoveryPlan struct {
	Steps []RecoveryStep `json:"steps,omitempty"`
}

type RecoveryStep struct {
	Order       int      `json:"order"`
	Name        string   `json:"name"`
	Requires    []string `json:"requires,omitempty"`
	Description string   `json:"description"`
}

type ResourcePlan struct {
	Identity               domain.ResourceIdentity   `json:"identity"`
	Kind                   string                    `json:"kind"`
	Dependencies           []domain.ResourceIdentity `json:"dependencies,omitempty"`
	SourceDeclarations     []string                  `json:"sourceDeclarations"`
	Adapter                string                    `json:"adapter,omitempty"`
	GeneratedFiles         []string                  `json:"generatedFiles"`
	CanonicalDigest        string                    `json:"canonicalDigest"`
	RolloutSensitiveDigest string                    `json:"rolloutSensitiveDigest"`
	RecoveryClassification string                    `json:"recoveryClassification"`
	SecretBackend          string                    `json:"secretBackend,omitempty"`
	SecretKeys             []string                  `json:"secretKeys,omitempty"`
	Ownership              string                    `json:"ownership,omitempty"`
	Lifecycle              string                    `json:"lifecycle,omitempty"`
	GraphState             string                    `json:"graphState,omitempty"`
	Overrides              []OverridePlan            `json:"overrides,omitempty"`
}

type ChangeAction struct {
	Operation string                  `json:"operation"`
	Identity  domain.ResourceIdentity `json:"identity"`
	Message   string                  `json:"message"`
}

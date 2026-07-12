package provenance

type Record struct {
	PlatformSpecificationDigest string            `json:"platformSpecificationDigest"`
	RuntimeProfileDigest        string            `json:"runtimeProfileDigest,omitempty"`
	ProviderSetDigest           string            `json:"providerSetDigest,omitempty"`
	CompilerVersion             string            `json:"compilerVersion"`
	CompilerBinaryDigest        string            `json:"compilerBinaryDigest,omitempty"`
	SourceCommit                string            `json:"sourceCommit,omitempty"`
	Target                      string            `json:"target"`
	AdapterVersions             map[string]string `json:"adapterVersions"`
	BundleDigest                string            `json:"bundleDigest"`
	SourceDateEpoch             string            `json:"sourceDateEpoch,omitempty"`
}

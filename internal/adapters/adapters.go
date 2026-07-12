package adapters

type Capability struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	Adapter   string `json:"adapter"`
	Supported bool   `json:"supported"`
}

func FoundationCapabilities() []Capability {
	return []Capability{
		{Name: "supportsReplicas", Target: "compose", Adapter: "foundation", Supported: false},
		{Name: "supportsReplicas", Target: "kubernetes", Adapter: "foundation", Supported: true},
		{Name: "supportsPodDisruptionBudget", Target: "kubernetes", Adapter: "foundation", Supported: true},
	}
}

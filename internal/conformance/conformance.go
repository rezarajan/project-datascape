package conformance

type Suite struct {
	Name       string   `json:"name"`
	Capability string   `json:"capability"`
	Checks     []string `json:"checks"`
}

func FoundationSuites() []Suite {
	return []Suite{
		{Name: "stream-foundation", Capability: "datascape.dev/stream.publish", Checks: []string{"publish", "subscribe", "replay", "retention", "authn", "authz", "metrics"}},
		{Name: "compiler-foundation", Capability: "datascape.dev/compiler.deterministic", Checks: []string{"parse", "stable-names", "health-checks", "no-secrets", "deterministic-ordering", "change-isolation"}},
	}
}

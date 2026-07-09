package settings

// KnownModel describes a model the service is allowed to pass to `claude`.
// Name is the canonical value handed to `claude --model`; Aliases are the
// shorthands the claude CLI also accepts and that SetModel will honour.
type KnownModel struct {
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
}

// KnownModels is the curated set of models selectable via the API. Order is
// most-capable-first so it doubles as a sensible display order in /models.
// Keep this in sync with the models the deployed claude CLI actually supports.
var KnownModels = []KnownModel{
	{Name: "claude-opus-4-8", Aliases: []string{"opus"}, Description: "Most capable Opus-tier model; best for long-horizon agentic work (default recommendation)."},
	{Name: "claude-opus-4-7", Description: "Previous-generation Opus; highly autonomous."},
	{Name: "claude-sonnet-5", Aliases: []string{"sonnet"}, Description: "Best balance of speed and intelligence; near-Opus quality on coding."},
	{Name: "claude-sonnet-4-6", Description: "Previous-generation Sonnet."},
	{Name: "claude-haiku-4-5", Aliases: []string{"haiku"}, Description: "Fastest and most cost-effective; best for simple tasks."},
}

// IsKnownModel reports whether name matches a known model's canonical name or
// one of its aliases.
func IsKnownModel(name string) bool {
	for _, m := range KnownModels {
		if m.Name == name {
			return true
		}
		for _, a := range m.Aliases {
			if a == name {
				return true
			}
		}
	}
	return false
}

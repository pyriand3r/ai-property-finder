package llm

import "strings"

// ProviderInfo describes one selectable LLM provider. The Settings UI is
// driven by this list (GET /api/config), so adding a provider means adding
// an entry here plus its implementation — no frontend change.
type ProviderInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Models are suggestions for the model picker, not an allow-list: model
	// IDs change faster than releases, so the backend accepts any non-empty
	// model string and the UI lets the user type one.
	Models []string `json:"models"`
	// RequiresBaseURL is true when the provider has no built-in endpoint.
	// For claude the Base URL is optional (proxy/gateway override).
	RequiresBaseURL bool `json:"requires_base_url"`
	// DefaultBaseURL is a typical endpoint, shown as the Base URL
	// placeholder. It is never applied implicitly: a switch of provider
	// must not silently talk to an address the user did not enter.
	DefaultBaseURL string `json:"default_base_url"`
	// RequiresAPIKey is false for providers that run without a key (a local
	// LM Studio server). The key is still sent when one is set.
	RequiresAPIKey bool `json:"requires_api_key"`
}

// providers is the catalog of implemented providers: Claude, plus the two
// OpenAI-compatible backends of ADR 0030.
var providers = []ProviderInfo{
	{
		ID:    "claude",
		Label: "Claude (Anthropic)",
		Models: []string{
			"claude-sonnet-5",
			"claude-opus-4-5",
			"claude-sonnet-4-5",
			"claude-haiku-4-5",
		},
		RequiresAPIKey: true,
	},
	{
		ID:              "lmstudio",
		Label:           "LM Studio (lokal)",
		Models:          []string{},
		RequiresBaseURL: true,
		DefaultBaseURL:  "http://localhost:1234/v1",
	},
	{
		ID:              "opencode",
		Label:           "OpenCode",
		Models:          []string{},
		RequiresBaseURL: true,
		DefaultBaseURL:  "https://opencode.ai/zen/v1",
		RequiresAPIKey:  true,
	},
}

// Providers returns a copy of the implemented-provider catalog.
func Providers() []ProviderInfo {
	result := make([]ProviderInfo, len(providers))
	for index, info := range providers {
		// Never nil: the Settings UI reads this as an array.
		info.Models = append(make([]string, 0, len(info.Models)), info.Models...)
		result[index] = info
	}
	return result
}

// FindProvider looks id up case-insensitively in the catalog.
func FindProvider(id string) (ProviderInfo, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, info := range providers {
		if info.ID == id {
			return info, true
		}
	}
	return ProviderInfo{}, false
}

func providerIDs() []string {
	ids := make([]string, len(providers))
	for index, info := range providers {
		ids[index] = info.ID
	}
	return ids
}

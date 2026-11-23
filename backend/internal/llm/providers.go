package llm

import "strings"

// ProviderInfo describes one selectable LLM provider. The Settings UI is
// driven by this list (GET /api/config), so adding a provider (LM Studio /
// OpenCode, ADR 0006) means adding an entry here plus its implementation —
// no frontend change.
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
}

// providers is the catalog of implemented providers (Claude only until the
// LM Studio/OpenCode work lands, ADR 0006).
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
	},
}

// Providers returns a copy of the implemented-provider catalog.
func Providers() []ProviderInfo {
	result := make([]ProviderInfo, len(providers))
	for index, info := range providers {
		info.Models = append([]string(nil), info.Models...)
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

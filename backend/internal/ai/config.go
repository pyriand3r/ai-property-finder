package ai

import (
	"strings"
	"time"
)

// Config is the subset of llm.* settings a Provider needs. main.go maps
// config.LLM onto this at startup; the ai package deliberately does not
// import internal/config so the provider abstraction stays dependency-free
// of the bootstrap layer.
type Config struct {
	Provider          string // "claude" | "lmstudio" | "opencode" | "" (unconfigured); the last two are OpenAI-compatible (ADR 0030)
	APIKey            string
	BaseURL           string
	Model             string
	TimeoutSeconds    int
	MaxRetries        int // corrective retries on a parse/shape failure
	ReasoningLanguage string
}

// timeout returns the per-call deadline, defaulting to 30s when unset.
func (c Config) timeout() time.Duration {
	seconds := c.TimeoutSeconds
	if seconds <= 0 {
		seconds = defaultTimeoutSeconds
	}
	return time.Duration(seconds) * time.Second
}

// reasoningLanguage returns the configured reasoning language, defaulting
// to German to match the UI.
func (c Config) reasoningLanguage() string {
	if strings.TrimSpace(c.ReasoningLanguage) == "" {
		return defaultReasoningLanguage
	}
	return c.ReasoningLanguage
}

const (
	providerClaude   = "claude"
	providerLMStudio = "lmstudio"
	providerOpenCode = "opencode"

	defaultTimeoutSeconds    = 30
	defaultReasoningLanguage = "de"
)

// NewProvider selects and constructs the active Provider from cfg. An empty
// provider, an unconfigured Claude provider (no API key), or an
// OpenAI-compatible provider (lmstudio/opencode, ADR 0030) without a base URL
// yields the unconfigured provider, whose operations fail fast with
// ErrNotConfigured instead of crashing. An unknown provider string is an
// error so a typo surfaces at startup. LM Studio and OpenCode need no API
// key here: a local server has none, and the key is sent only when set.
func NewProvider(cfg Config) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "":
		return &unconfiguredProvider{}, nil
	case providerClaude:
		if strings.TrimSpace(cfg.APIKey) == "" {
			return &unconfiguredProvider{}, nil
		}
		return newClaudeProvider(cfg), nil
	case providerLMStudio, providerOpenCode:
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return &unconfiguredProvider{}, nil
		}
		return newOpenAIProvider(cfg), nil
	default:
		return nil, &UnknownProviderError{Provider: cfg.Provider}
	}
}

// UnknownProviderError signals an llm.provider value that is neither a
// known implementation nor a reserved-for-later value.
type UnknownProviderError struct {
	Provider string
}

func (err *UnknownProviderError) Error() string {
	return "unknown llm provider: " + err.Provider
}

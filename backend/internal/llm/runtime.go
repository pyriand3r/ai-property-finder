// Package llm owns the live LLM configuration of the running process: the
// resolved Tier-2 settings plus the two providers built from them — the
// single-shot internal/ai.Provider and the multi-turn internal/agent.Provider
// (ADR 0007). Both are rebuilt and swapped atomically when the settings
// change (PATCH /api/config), so a new model or API key takes effect on the
// next chat turn / evaluation without a restart.
package llm

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/agent/provider/claude"
	"propertyfinder/internal/agent/provider/openai"
	"propertyfinder/internal/ai"
	"propertyfinder/internal/config"
	"propertyfinder/internal/domain"
	"strings"
	"sync"
	"sync/atomic"
)

// Store persists and reloads the Tier-2 llm.* settings. The production
// implementation is PoolStore; tests inject a fake.
type Store interface {
	Load(ctx context.Context) (config.LLM, error)
	// Save persists cfg. The API key row is written only when
	// includeAPIKey is true.
	Save(ctx context.Context, cfg config.LLM, includeAPIKey bool) error
}

// Snapshot is one immutable generation of the live configuration. A caller
// that needs a consistent view across several reads (one Agent turn: the
// provider, its message formatting, and the roundtrip budget) takes one
// Snapshot and uses it throughout, so a concurrent Update can never mix two
// generations inside a turn.
type Snapshot struct {
	Config config.LLM
	AI     ai.Provider
	Agent  agent.Provider
}

// Update is a partial change to the LLM settings (PATCH semantics): nil
// fields are left unchanged. APIKey is write-only — there is no way to read
// it back, and an Update with a nil APIKey keeps the stored key.
type Update struct {
	Provider            *string `json:"provider"`
	APIKey              *string `json:"api_key"`
	BaseURL             *string `json:"base_url"`
	Model               *string `json:"model"`
	TimeoutSeconds      *int    `json:"timeout_seconds"`
	MaxRetries          *int    `json:"max_retries"`
	MaxToolCallsPerTurn *int    `json:"max_tool_calls_per_turn"`
	MaxLLMRoundtrips    *int    `json:"max_llm_roundtrips"`
	ReasoningLanguage   *string `json:"reasoning_language"`
}

// Runtime holds the current Snapshot behind an atomic pointer, so readers
// (every chat turn, every evaluation) are lock-free, and serializes
// writers. It implements ai.Provider by delegating each call to the
// snapshot current at call time — single-shot operations are one request
// each, so they need no cross-call consistency.
type Runtime struct {
	store    Store
	updateMu sync.Mutex
	current  atomic.Pointer[Snapshot]
}

// New builds the initial Snapshot from cfg. An error means cfg names an
// unknown provider (a typo in app_config) — fatal at boot.
func New(cfg config.LLM, store Store) (*Runtime, error) {
	snapshot, err := buildSnapshot(cfg)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{store: store}
	runtime.current.Store(snapshot)
	return runtime, nil
}

// Snapshot returns the current generation. Never nil.
func (runtime *Runtime) Snapshot() *Snapshot {
	return runtime.current.Load()
}

// Config returns the current resolved settings (including the API key —
// callers must not serialize it).
func (runtime *Runtime) Config() config.LLM {
	return runtime.Snapshot().Config
}

// EvaluateListing implements ai.Provider against the current snapshot.
func (runtime *Runtime) EvaluateListing(ctx context.Context, listing domain.Listing, criteria []domain.Criterion) ([]domain.CriterionScore, error) {
	return runtime.Snapshot().AI.EvaluateListing(ctx, listing, criteria)
}

// GenerateRanking implements ai.Provider against the current snapshot.
func (runtime *Runtime) GenerateRanking(ctx context.Context, listings []domain.Listing) (ai.RankingResult, error) {
	return runtime.Snapshot().AI.GenerateRanking(ctx, listings)
}

// AnalyzeRejection implements ai.Provider against the current snapshot.
func (runtime *Runtime) AnalyzeRejection(ctx context.Context, listing domain.Listing, rejectionComment string, criteria []domain.Criterion, contextText string) (ai.RejectionAnalysisResult, error) {
	return runtime.Snapshot().AI.AnalyzeRejection(ctx, listing, rejectionComment, criteria, contextText)
}

// Apply merges update into the stored settings, validates the result,
// persists it, and swaps in new providers. The merge base is reloaded from
// the store (the DB is authoritative, see internal/config), not the
// in-memory copy, so fields not named in update keep their stored value.
// Nothing is persisted or swapped unless the whole update is valid. An
// Agent turn already running keeps the Snapshot it started with.
func (runtime *Runtime) Apply(ctx context.Context, update Update) (config.LLM, error) {
	runtime.updateMu.Lock()
	defer runtime.updateMu.Unlock()

	stored, err := runtime.store.Load(ctx)
	if err != nil {
		return config.LLM{}, err
	}
	merged, err := merge(stored, update)
	if err != nil {
		return config.LLM{}, err
	}
	snapshot, err := buildSnapshot(merged)
	if err != nil {
		return config.LLM{}, &domain.ValidationError{Field: "provider", Message: err.Error()}
	}
	if saveError := runtime.store.Save(ctx, merged, update.APIKey != nil); saveError != nil {
		return config.LLM{}, saveError
	}
	runtime.current.Store(snapshot)
	log.Printf("llm config updated: provider=%s model=%q api_key_changed=%t api_key_configured=%t",
		merged.Provider, merged.Model, update.APIKey != nil, merged.APIKey != "")
	return merged, nil
}

// Bounds for the numeric settings. They reject typos (0 or a negative
// timeout, a roundtrip budget of 10000) rather than encode policy.
const (
	maxTimeoutSeconds      = 600
	maxRetriesLimit        = 10
	maxToolCallsLimit      = 100
	maxLLMRoundtripsLimit  = 50
	maxReasoningLangLength = 32
)

func merge(base config.LLM, update Update) (config.LLM, error) {
	merged := base
	if update.Provider != nil {
		merged.Provider = strings.ToLower(strings.TrimSpace(*update.Provider))
	}
	if update.APIKey != nil {
		merged.APIKey = strings.TrimSpace(*update.APIKey)
	}
	if update.BaseURL != nil {
		merged.BaseURL = strings.TrimSpace(*update.BaseURL)
	}
	if update.Model != nil {
		merged.Model = strings.TrimSpace(*update.Model)
	}
	if update.TimeoutSeconds != nil {
		merged.TimeoutSeconds = *update.TimeoutSeconds
	}
	if update.MaxRetries != nil {
		merged.MaxRetries = *update.MaxRetries
	}
	if update.MaxToolCallsPerTurn != nil {
		merged.MaxToolCallsPerTurn = *update.MaxToolCallsPerTurn
	}
	if update.MaxLLMRoundtrips != nil {
		merged.MaxLLMRoundtrips = *update.MaxLLMRoundtrips
	}
	if update.ReasoningLanguage != nil {
		merged.ReasoningLanguage = strings.TrimSpace(*update.ReasoningLanguage)
	}
	return merged, validate(merged, update)
}

// validate checks the merged result. Only fields named in update are
// reported as the cause of a failure's Field — but the whole result is
// checked, so a stored-but-invalid value (hand-edited DB row) cannot be
// kept alive by an unrelated PATCH.
func validate(cfg config.LLM, update Update) error {
	if update.APIKey != nil && cfg.APIKey == "" {
		return &domain.ValidationError{Field: "api_key", Message: "API-Key darf nicht leer sein (zum Beibehalten das Feld weglassen)"}
	}
	info, ok := FindProvider(cfg.Provider)
	if !ok {
		return &domain.ValidationError{Field: "provider", Message: fmt.Sprintf("Unbekannter Provider %q (verfügbar: %s)", cfg.Provider, strings.Join(providerIDs(), ", "))}
	}
	if info.RequiresBaseURL && cfg.BaseURL == "" {
		return &domain.ValidationError{Field: "base_url", Message: fmt.Sprintf("Base URL ist für %s erforderlich (z. B. %s)", info.Label, info.DefaultBaseURL)}
	}
	if cfg.Model == "" {
		return &domain.ValidationError{Field: "model", Message: "Modell darf nicht leer sein"}
	}
	if cfg.BaseURL != "" {
		parsed, err := url.Parse(cfg.BaseURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return &domain.ValidationError{Field: "base_url", Message: "Base URL muss eine gültige http(s)-URL sein (oder leer)"}
		}
	}
	if err := checkRange("timeout_seconds", "Timeout (Sekunden)", cfg.TimeoutSeconds, 1, maxTimeoutSeconds); err != nil {
		return err
	}
	if err := checkRange("max_retries", "Max. Wiederholungen", cfg.MaxRetries, 0, maxRetriesLimit); err != nil {
		return err
	}
	if err := checkRange("max_tool_calls_per_turn", "Max. Tool-Aufrufe pro Turn", cfg.MaxToolCallsPerTurn, 1, maxToolCallsLimit); err != nil {
		return err
	}
	if err := checkRange("max_llm_roundtrips", "Max. LLM-Roundtrips pro Turn", cfg.MaxLLMRoundtrips, 1, maxLLMRoundtripsLimit); err != nil {
		return err
	}
	if cfg.ReasoningLanguage == "" || len(cfg.ReasoningLanguage) > maxReasoningLangLength {
		return &domain.ValidationError{Field: "reasoning_language", Message: "Antwortsprache darf nicht leer sein (max. 32 Zeichen)"}
	}
	return nil
}

func checkRange(field, label string, value, lower, upper int) error {
	if value < lower || value > upper {
		return &domain.ValidationError{Field: field, Message: fmt.Sprintf("%s muss zwischen %d und %d liegen", label, lower, upper)}
	}
	return nil
}

// buildSnapshot constructs both providers from cfg. ai.NewProvider owns
// the "unconfigured" gating (empty key, reserved provider values); the
// agent-side gating lives in newAgentProvider and mirrors it.
func buildSnapshot(cfg config.LLM) (*Snapshot, error) {
	aiProvider, err := ai.NewProvider(ai.Config{
		Provider:          cfg.Provider,
		APIKey:            cfg.APIKey,
		BaseURL:           cfg.BaseURL,
		Model:             cfg.Model,
		TimeoutSeconds:    cfg.TimeoutSeconds,
		MaxRetries:        cfg.MaxRetries,
		ReasoningLanguage: cfg.ReasoningLanguage,
	})
	if err != nil {
		return nil, err
	}
	return &Snapshot{Config: cfg, AI: aiProvider, Agent: newAgentProvider(cfg)}, nil
}

// newAgentProvider resolves the multi-turn agent.Provider (ADR 0007) from
// cfg, mirroring ai.NewProvider's gating: Claude without an API key and an
// OpenAI-compatible provider (lmstudio/opencode, ADR 0030) without a base
// URL yield the unconfigured stand-in instead of a Provider that would fail
// on every call anyway.
func newAgentProvider(cfg config.LLM) agent.Provider {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "claude":
		if strings.TrimSpace(cfg.APIKey) == "" {
			return agent.NewUnconfiguredProvider()
		}
		return claude.New(claude.Config{
			APIKey:         cfg.APIKey,
			BaseURL:        cfg.BaseURL,
			Model:          cfg.Model,
			TimeoutSeconds: cfg.TimeoutSeconds,
		})
	case "lmstudio", "opencode":
		if strings.TrimSpace(cfg.BaseURL) == "" {
			return agent.NewUnconfiguredProvider()
		}
		return openai.New(openai.Config{
			BaseURL:        cfg.BaseURL,
			APIKey:         cfg.APIKey,
			Model:          cfg.Model,
			TimeoutSeconds: cfg.TimeoutSeconds,
		})
	default:
		return agent.NewUnconfiguredProvider()
	}
}

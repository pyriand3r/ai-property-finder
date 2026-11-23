package api

import (
	"net/http"
	"propertyfinder/internal/config"
	"propertyfinder/internal/llm"
)

// configResponse is the GET/PATCH /api/config body. APIKey is deliberately
// not echoed back — api_key_configured reports whether one is set without
// ever exposing the value over HTTP. providers is the backend-owned list of
// selectable providers and their model suggestions; the Settings UI renders
// its pickers from it.
type configResponse struct {
	LLM       llmConfigResponse  `json:"llm"`
	Providers []llm.ProviderInfo `json:"providers"`
}

type llmConfigResponse struct {
	Provider            string `json:"provider"`
	BaseURL             string `json:"base_url"`
	Model               string `json:"model"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	MaxRetries          int    `json:"max_retries"`
	MaxToolCallsPerTurn int    `json:"max_tool_calls_per_turn"`
	MaxLLMRoundtrips    int    `json:"max_llm_roundtrips"`
	ReasoningLanguage   string `json:"reasoning_language"`
	APIKeyConfigured    bool   `json:"api_key_configured"`
}

// ConfigHandler serves and edits the live Tier-2 LLM configuration.
type ConfigHandler struct {
	runtime *llm.Runtime
}

func (handler *ConfigHandler) get(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, newConfigResponse(handler.runtime.Config()))
}

// patch applies a partial llm.* update (docs/spec 00-03 "Config"): the
// request fields are all optional, api_key is write-only, and the change is
// live for the next chat turn / evaluation without a restart.
func (handler *ConfigHandler) patch(writer http.ResponseWriter, request *http.Request) {
	var body struct {
		LLM llm.Update `json:"llm"`
	}
	if !decodeJSON(writer, request, &body) {
		return
	}
	updated, err := handler.runtime.Apply(request.Context(), body.LLM)
	if err != nil {
		writeError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, newConfigResponse(updated))
}

func newConfigResponse(cfg config.LLM) configResponse {
	return configResponse{
		LLM: llmConfigResponse{
			Provider:            cfg.Provider,
			BaseURL:             cfg.BaseURL,
			Model:               cfg.Model,
			TimeoutSeconds:      cfg.TimeoutSeconds,
			MaxRetries:          cfg.MaxRetries,
			MaxToolCallsPerTurn: cfg.MaxToolCallsPerTurn,
			MaxLLMRoundtrips:    cfg.MaxLLMRoundtrips,
			ReasoningLanguage:   cfg.ReasoningLanguage,
			APIKeyConfigured:    cfg.APIKey != "",
		},
		Providers: llm.Providers(),
	}
}

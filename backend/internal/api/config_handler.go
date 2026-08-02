package api

import (
	"encoding/json"
	"net/http"
	"propertyfinder/internal/config"
)

// configResponse is the GET /api/config body. APIKey is deliberately not
// echoed back — apiKeyConfigured reports whether one is set without ever
// exposing the value over HTTP.
type configResponse struct {
	LLM llmConfigResponse `json:"llm"`
}

type llmConfigResponse struct {
	Provider            string `json:"provider"`
	BaseURL             string `json:"baseUrl"`
	Model               string `json:"model"`
	TimeoutSeconds      int    `json:"timeoutSeconds"`
	MaxRetries          int    `json:"maxRetries"`
	MaxToolCallsPerTurn int    `json:"maxToolCallsPerTurn"`
	MaxLLMRoundtrips    int    `json:"maxLlmRoundtrips"`
	ReasoningLanguage   string `json:"reasoningLanguage"`
	APIKeyConfigured    bool   `json:"apiKeyConfigured"`
}

// ConfigHandler serves the resolved Tier-2 app configuration.
type ConfigHandler struct {
	llm config.LLM
}

func (handler *ConfigHandler) get(writer http.ResponseWriter, _ *http.Request) {
	body := configResponse{
		LLM: llmConfigResponse{
			Provider:            handler.llm.Provider,
			BaseURL:             handler.llm.BaseURL,
			Model:               handler.llm.Model,
			TimeoutSeconds:      handler.llm.TimeoutSeconds,
			MaxRetries:          handler.llm.MaxRetries,
			MaxToolCallsPerTurn: handler.llm.MaxToolCallsPerTurn,
			MaxLLMRoundtrips:    handler.llm.MaxLLMRoundtrips,
			ReasoningLanguage:   handler.llm.ReasoningLanguage,
			APIKeyConfigured:    handler.llm.APIKey != "",
		},
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(writer).Encode(body)
}

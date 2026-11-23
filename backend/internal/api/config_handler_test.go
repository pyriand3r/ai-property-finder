package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/config"
	"propertyfinder/internal/llm"
	"strings"
	"testing"
)

type memoryStore struct{ cfg config.LLM }

func (store *memoryStore) Load(context.Context) (config.LLM, error) { return store.cfg, nil }

func (store *memoryStore) Save(_ context.Context, cfg config.LLM, _ bool) error {
	store.cfg = cfg
	return nil
}

func newConfigTestHandler(t *testing.T) *ConfigHandler {
	t.Helper()
	store := &memoryStore{cfg: config.LLM{
		Provider: "claude", APIKey: "sk-secret-value", Model: "claude-sonnet-5",
		TimeoutSeconds: 30, MaxRetries: 2, MaxToolCallsPerTurn: 15, MaxLLMRoundtrips: 5, ReasoningLanguage: "de",
	}}
	runtime, err := llm.New(store.cfg, store)
	if err != nil {
		t.Fatalf("llm.New: %v", err)
	}
	return &ConfigHandler{runtime: runtime}
}

// TestConfigHandler_NeverEchoesAPIKey: issue #23 acceptance — the key must not
// appear in GET or in the PATCH response, even right after it was set.
func TestConfigHandler_NeverEchoesAPIKey(t *testing.T) {
	handler := newConfigTestHandler(t)

	getRecorder := httptest.NewRecorder()
	handler.get(getRecorder, httptest.NewRequest(http.MethodGet, "/api/config", http.NoBody))

	patchRecorder := httptest.NewRecorder()
	handler.patch(patchRecorder, httptest.NewRequest(http.MethodPatch, "/api/config", strings.NewReader(`{"llm":{"api_key":"sk-brand-new","model":"claude-opus-4-5"}}`)))

	if patchRecorder.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, body %s", patchRecorder.Code, patchRecorder.Body)
	}
	for name, body := range map[string]string{"GET": getRecorder.Body.String(), "PATCH": patchRecorder.Body.String()} {
		if strings.Contains(body, "sk-secret-value") || strings.Contains(body, "sk-brand-new") {
			t.Errorf("%s response leaks the API key: %s", name, body)
		}
		if !strings.Contains(body, `"api_key_configured":true`) {
			t.Errorf("%s response lacks api_key_configured=true: %s", name, body)
		}
	}
	if !strings.Contains(patchRecorder.Body.String(), `"model":"claude-opus-4-5"`) {
		t.Errorf("PATCH response does not show the new model: %s", patchRecorder.Body)
	}
}

func TestConfigHandler_Patch_InvalidValueIs400(t *testing.T) {
	handler := newConfigTestHandler(t)
	recorder := httptest.NewRecorder()

	handler.patch(recorder, httptest.NewRequest(http.MethodPatch, "/api/config", strings.NewReader(`{"llm":{"provider":"openai"}}`)))

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body %s", recorder.Code, recorder.Body)
	}
}

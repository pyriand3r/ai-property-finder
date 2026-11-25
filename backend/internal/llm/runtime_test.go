package llm

import (
	"context"
	"errors"
	"fmt"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/config"
	"propertyfinder/internal/domain"
	"strings"
	"testing"
)

type fakeStore struct {
	stored        config.LLM
	saveCalls     int
	savedWithKey  bool
	saveError     error
	loadCallCount int
}

func (store *fakeStore) Load(context.Context) (config.LLM, error) {
	store.loadCallCount++
	return store.stored, nil
}

func (store *fakeStore) Save(_ context.Context, cfg config.LLM, includeAPIKey bool) error {
	if store.saveError != nil {
		return store.saveError
	}
	store.saveCalls++
	store.savedWithKey = includeAPIKey
	store.stored = cfg
	return nil
}

func baseConfig() config.LLM {
	return config.LLM{
		Provider: "claude", APIKey: "sk-old", Model: "claude-sonnet-5",
		TimeoutSeconds: 30, MaxRetries: 2, MaxToolCallsPerTurn: 15, MaxLLMRoundtrips: 5, ReasoningLanguage: "de",
	}
}

func newRuntime(t *testing.T, store *fakeStore) *Runtime {
	t.Helper()
	runtime, err := New(store.stored, store)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return runtime
}

// TestRuntime_Apply_SwapsSnapshotWithoutRestart is the issue #23 acceptance
// criterion: a model change must be visible to the very next consumer of the
// runtime (next chat turn / evaluation), with the roundtrip budget and the
// unconfigured-gating rebuilt too.
func TestRuntime_Apply_SwapsSnapshotWithoutRestart(t *testing.T) {
	store := &fakeStore{stored: baseConfig()}
	runtime := newRuntime(t, store)
	before := runtime.Snapshot()

	if _, err := runtime.Apply(context.Background(), Update{Model: new("claude-opus-4-5"), MaxLLMRoundtrips: new(9)}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	after := runtime.Snapshot()
	if after == before {
		t.Fatal("snapshot was not replaced")
	}
	if after.Config.Model != "claude-opus-4-5" || after.Config.MaxLLMRoundtrips != 9 {
		t.Errorf("got model=%q roundtrips=%d, want claude-opus-4-5 / 9", after.Config.Model, after.Config.MaxLLMRoundtrips)
	}
	if after.Config.APIKey != "sk-old" {
		t.Errorf("api key changed to %q without being sent", after.Config.APIKey)
	}
	if store.savedWithKey {
		t.Error("API key row was written although the update carried no key")
	}
	if before.Config.Model != "claude-sonnet-5" {
		t.Error("the old snapshot was mutated; in-flight turns must keep their generation")
	}
}

func TestRuntime_Apply_NewAPIKeyIsPersistedAndMakesProviderConfigured(t *testing.T) {
	cfg := baseConfig()
	cfg.APIKey = ""
	store := &fakeStore{stored: cfg}
	runtime := newRuntime(t, store)
	if _, err := runtime.Snapshot().Agent.Chat(context.Background(), agent.ChatRequest{}); !errors.Is(err, agent.ErrNotConfigured) {
		t.Fatalf("pre-update Chat error = %v, want ErrNotConfigured", err)
	}

	if _, err := runtime.Apply(context.Background(), Update{APIKey: new("  sk-new  ")}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if !store.savedWithKey || store.stored.APIKey != "sk-new" {
		t.Errorf("key not persisted trimmed: savedWithKey=%t stored=%q", store.savedWithKey, store.stored.APIKey)
	}
	if _, err := runtime.Snapshot().Agent.Chat(context.Background(), agent.ChatRequest{}); errors.Is(err, agent.ErrNotConfigured) {
		t.Error("agent provider is still the unconfigured stand-in after setting a key")
	}
}

func TestRuntime_Apply_InvalidUpdateChangesNothing(t *testing.T) {
	cases := map[string]Update{
		"unknown provider":  {Provider: new("openai")},
		"empty model":       {Model: new("  ")},
		"empty api key":     {APIKey: new("")},
		"bad base url":      {BaseURL: new("ftp://x")},
		"zero timeout":      {TimeoutSeconds: new(0)},
		"huge roundtrips":   {MaxLLMRoundtrips: new(1000)},
		"zero roundtrips":   {MaxLLMRoundtrips: new(0)},
		"negative retries":  {MaxRetries: new(-1)},
		"empty language":    {ReasoningLanguage: new("")},
		"zero tool budget":  {MaxToolCallsPerTurn: new(0)},
		"mixed valid+wrong": {Model: new("claude-opus-4-5"), MaxLLMRoundtrips: new(0)},
		// base_url is stored empty for claude: switching to a provider with
		// no built-in endpoint must be rejected, not saved as a dead config.
		"lmstudio without base url": {Provider: new("lmstudio"), Model: new("qwen3")},
	}
	for name, update := range cases {
		t.Run(name, func(t *testing.T) {
			store := &fakeStore{stored: baseConfig()}
			runtime := newRuntime(t, store)
			before := runtime.Snapshot()

			_, err := runtime.Apply(context.Background(), update)

			if _, ok := errors.AsType[*domain.ValidationError](err); !ok {
				t.Fatalf("error = %v, want *domain.ValidationError", err)
			}
			if store.saveCalls != 0 {
				t.Error("invalid update was persisted")
			}
			if runtime.Snapshot() != before {
				t.Error("invalid update swapped the snapshot")
			}
		})
	}
}

// TestRuntime_Apply_LMStudioNeedsNoAPIKey: a local LM Studio server has no
// key, so switching to it with an empty key must yield a working (not
// unconfigured) agent provider, with the key sent only if one is set.
func TestRuntime_Apply_LMStudioNeedsNoAPIKey(t *testing.T) {
	cfg := baseConfig()
	cfg.APIKey = ""
	store := &fakeStore{stored: cfg}
	runtime := newRuntime(t, store)

	if _, err := runtime.Apply(context.Background(), Update{Provider: new("lmstudio"), BaseURL: new("http://127.0.0.1:1/v1"), Model: new("qwen3")}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	events, err := runtime.Snapshot().Agent.Chat(context.Background(), agent.ChatRequest{})
	if errors.Is(err, agent.ErrNotConfigured) {
		t.Fatal("agent provider is the unconfigured stand-in for a configured LM Studio")
	}
	if err == nil {
		for range events { // nothing listens on port 1: drains one error event
		}
	}
}

// TestRuntime_New_OpenAICompatibleWithoutBaseURLIsUnconfigured: a hand-edited
// app_config row (provider=opencode, no base_url) must boot into the
// unconfigured stand-in rather than fail or dial an empty URL.
func TestRuntime_New_OpenAICompatibleWithoutBaseURLIsUnconfigured(t *testing.T) {
	cfg := baseConfig()
	cfg.Provider = "opencode"
	runtime := newRuntime(t, &fakeStore{stored: cfg})

	if _, err := runtime.Snapshot().Agent.Chat(context.Background(), agent.ChatRequest{}); !errors.Is(err, agent.ErrNotConfigured) {
		t.Fatalf("Chat error = %v, want ErrNotConfigured", err)
	}
}

// TestRuntime_Apply_PersistFailureKeepsOldSnapshot: the swap happens only
// after a successful save, so memory never runs ahead of the DB.
func TestRuntime_Apply_PersistFailureKeepsOldSnapshot(t *testing.T) {
	store := &fakeStore{stored: baseConfig(), saveError: errors.New("db down")}
	runtime := newRuntime(t, store)
	before := runtime.Snapshot()

	if _, err := runtime.Apply(context.Background(), Update{Model: new("claude-opus-4-5")}); err == nil {
		t.Fatal("expected error")
	}
	if runtime.Snapshot() != before {
		t.Error("snapshot swapped despite failed save")
	}
}

// TestLLM_String_RedactsAPIKey guards the "API key never appears in any log"
// acceptance criterion against an accidental %v / %+v / %#v of the struct.
func TestLLM_String_RedactsAPIKey(t *testing.T) {
	cfg := baseConfig()
	cfg.APIKey = "sk-super-secret"
	for _, rendered := range []string{cfg.String(), fmt.Sprintf("%v", []config.LLM{cfg}), fmt.Sprintf("%+v", cfg), fmt.Sprintf("%#v", cfg)} {
		if strings.Contains(rendered, "sk-super-secret") {
			t.Errorf("api key leaked: %s", rendered)
		}
	}
}

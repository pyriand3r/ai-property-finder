package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/domain"
	"strings"
	"testing"
)

// sseBody is a canned Chat Completions stream: text, one tool call spread
// over several deltas (the arguments split mid-string, as local servers
// do), a second parameterless call without any argument chunk, then the
// trailing usage-only chunk and [DONE].
const sseBody = `data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"Ich prüfe "}}]}

data: {"choices":[{"index":0,"delta":{"content":"das Inserat."}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"fetch_url","arguments":""}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"url\":\""}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"https://x\"}"}}]}}]}

data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":1,"function":{"name":"compute_ranking","arguments":""}}]}}]}

data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":42,"completion_tokens":17}}

data: [DONE]

`

func collect(t *testing.T, events <-chan agent.ChatEvent) []agent.ChatEvent {
	t.Helper()
	var all []agent.ChatEvent
	for event := range events {
		all = append(all, event)
	}
	return all
}

func TestChat_StreamsTextThenToolCallsThenDone(t *testing.T) {
	var gotAuth string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s, want /v1/chat/completions", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseBody))
	}))
	defer server.Close()

	provider := New(Config{BaseURL: server.URL + "/v1/", Model: "qwen3"})
	events, err := provider.Chat(context.Background(), agent.ChatRequest{
		System:   []agent.ContentBlock{{Type: agent.ContentBlockText, Text: "sys"}},
		Messages: []agent.Message{{Role: "user", Content: []agent.ContentBlock{{Type: agent.ContentBlockText, Text: "hi"}}}},
		Tools:    []agent.Tool{{Name: "fetch_url", Description: "d", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}},
		Stream:   true,
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	all := collect(t, events)

	var text string
	var calls []*agent.ToolCall
	var done *agent.ChatEvent
	for i := range all {
		switch all[i].Type {
		case agent.ChatEventTextDelta:
			text += all[i].Text
		case agent.ChatEventToolCallRequest:
			calls = append(calls, all[i].ToolCall)
		case agent.ChatEventDone:
			done = &all[i]
		}
		if all[i].Error != nil {
			t.Fatalf("unexpected error event: %v", all[i].Error)
		}
	}
	if text != "Ich prüfe das Inserat." {
		t.Errorf("text = %q", text)
	}
	if len(calls) != 2 {
		t.Fatalf("got %d tool calls, want 2", len(calls))
	}
	if calls[0].ID != "call_a" || calls[0].Name != "fetch_url" || string(calls[0].Arguments) != `{"url":"https://x"}` {
		t.Errorf("call 0 = %+v args=%s", calls[0], calls[0].Arguments)
	}
	// No argument chunk and no id: the call still has to replay validly.
	if calls[1].Name != "compute_ranking" || string(calls[1].Arguments) != "{}" || !strings.HasPrefix(calls[1].ID, "call_") {
		t.Errorf("call 1 = %+v args=%s", calls[1], calls[1].Arguments)
	}
	if done == nil || done.Usage == nil || done.Usage.InputTokens != 42 || done.Usage.OutputTokens != 17 {
		t.Errorf("done = %+v", done)
	}
	if all[len(all)-1].Type != agent.ChatEventDone {
		t.Error("tool calls must be emitted before done, done last")
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want none without an API key", gotAuth)
	}
	if gotBody["stream"] != true || gotBody["model"] != "qwen3" {
		t.Errorf("request body = %v", gotBody)
	}
}

func TestChat_SendsBearerTokenWhenKeyIsSet(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"x\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()

	events, _ := New(Config{BaseURL: server.URL, APIKey: "k", Model: "m"}).Chat(context.Background(), agent.ChatRequest{})
	collect(t, events)

	if gotAuth != "Bearer k" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}

// TestChat_TruncatedStreamIsAnError: a connection cut before [DONE] or a
// finish_reason must not be reported as a finished reply, or the loop would
// store half an answer as complete.
func TestChat_TruncatedStreamIsAnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"halb\"}}]}\n\n"))
	}))
	defer server.Close()

	events, _ := New(Config{BaseURL: server.URL, Model: "m"}).Chat(context.Background(), agent.ChatRequest{})
	all := collect(t, events)

	last := all[len(all)-1]
	if !errors.Is(last.Error, io.ErrUnexpectedEOF) {
		t.Fatalf("last event = %+v, want unexpected-EOF error", last)
	}
}

func TestChat_HTTPErrorCarriesServerMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"model not loaded"}}`))
	}))
	defer server.Close()

	events, _ := New(Config{BaseURL: server.URL, Model: "m"}).Chat(context.Background(), agent.ChatRequest{})
	all := collect(t, events)

	if len(all) != 1 || all[0].Error == nil || !strings.Contains(all[0].Error.Error(), "model not loaded") || !strings.Contains(all[0].Error.Error(), "400") {
		t.Fatalf("events = %+v", all)
	}
}

// TestChat_CancelledContextYieldsContextCanceled: Loop.Run tells a Stop
// click from a failure with errors.Is(err, context.Canceled).
func TestChat_CancelledContextYieldsContextCanceled(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	events, _ := New(Config{BaseURL: server.URL, Model: "m"}).Chat(ctx, agent.ChatRequest{})
	cancel()
	all := collect(t, events)

	if len(all) != 1 || !errors.Is(all[0].Error, context.Canceled) {
		t.Fatalf("events = %+v, want one context.Canceled error", all)
	}
}

// TestBuildMessages_SplitsToolResultsAndKeepsPairing covers the OpenAI
// shape: an assistant turn with tool_calls (content null when it has no
// text), then one role=tool message per result, then the follow-up user text.
func TestBuildMessages_SplitsToolResultsAndKeepsPairing(t *testing.T) {
	rows := []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "Schau dir das an"},
		{Role: domain.ChatRoleAssistant, ToolCalls: json.RawMessage(`[{"id":"c1","name":"fetch_url","input":{"url":"https://x"}},{"id":"c2","name":"compute_ranking","input":null}]`)},
		{Role: domain.ChatRoleTool, ToolResult: json.RawMessage(`{"tool_call_id":"c1","result":{"ok":true}}`)},
		{Role: domain.ChatRoleTool, ToolResult: json.RawMessage(`{"tool_call_id":"c2","result":"done"}`)},
		{Role: domain.ChatRoleAssistant, Content: "Fertig."},
	}
	provider := New(Config{BaseURL: "http://x", Model: "m"})

	wire := buildMessages(
		[]agent.ContentBlock{{Type: agent.ContentBlockText, Text: "sys"}},
		agent.RepairToolPairing(provider.FormatMessages(rows)),
	)

	roles := make([]string, len(wire))
	for i, m := range wire {
		roles[i] = m.Role
	}
	if got := strings.Join(roles, ","); got != "system,user,assistant,tool,tool,assistant" {
		t.Fatalf("roles = %s", got)
	}
	assistant := wire[2]
	if assistant.Content != nil || len(assistant.ToolCalls) != 2 {
		t.Errorf("assistant = %+v, want null content and 2 tool_calls", assistant)
	}
	if assistant.ToolCalls[1].Function.Arguments != "{}" {
		t.Errorf("parameterless call arguments = %q, want {}", assistant.ToolCalls[1].Function.Arguments)
	}
	if wire[3].ToolCallID != "c1" || *wire[3].Content != `{"ok":true}` || wire[4].ToolCallID != "c2" {
		t.Errorf("tool messages = %+v %+v", wire[3], wire[4])
	}
}

func TestCapabilities_NoNativeDocumentOrImageInput(t *testing.T) {
	provider := New(Config{})
	if provider.SupportsDocumentInput() || provider.SupportsImageInput() {
		t.Error("openai-compatible provider must not advertise document/image input")
	}
}

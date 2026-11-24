package claude

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/domain"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// sseBody is a canned Anthropic streaming response: a text delta followed
// by one complete tool_use call, matching the shape a Claude tool-use turn
// produces (spec 00-03 test: "Claude-Tool-Call → ChatEvent korrekt").
const sseBody = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":42,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Checking that "}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"listing now."}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"fetch_url","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"url\":\""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"https://x\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":42,"output_tokens":17}}

event: message_stop
data: {"type":"message_stop"}

`

func TestChat_StreamsTextThenToolCallThenDone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sseBody))
	}))
	defer server.Close()

	provider := New(Config{APIKey: "test-key", BaseURL: server.URL, Model: "claude-3-5-sonnet-20241022"})

	req := agent.ChatRequest{
		System:   []agent.ContentBlock{{Type: agent.ContentBlockText, Text: "You are a helpful agent."}},
		Messages: []agent.Message{{Role: "user", Content: []agent.ContentBlock{{Type: agent.ContentBlockText, Text: "Check this listing"}}}},
		Tools: []agent.Tool{{
			Name:        "fetch_url",
			Description: "Fetch a URL",
			InputSchema: json.RawMessage(`{"properties":{"url":{"type":"string"}},"required":["url"]}`),
		}},
		Stream: true,
	}

	events, err := provider.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	var text strings.Builder
	var toolCall *agent.ToolCall
	var done bool
	var usage *agent.Usage

	for event := range events {
		if event.Error != nil {
			t.Fatalf("unexpected event error: %v", event.Error)
		}
		switch event.Type {
		case agent.ChatEventTextDelta:
			text.WriteString(event.Text)
		case agent.ChatEventToolCallRequest:
			toolCall = event.ToolCall
		case agent.ChatEventDone:
			done = true
			usage = event.Usage
		}
	}

	if got, want := text.String(), "Checking that listing now."; got != want {
		t.Errorf("text = %q, want %q", got, want)
	}
	if toolCall == nil {
		t.Fatal("expected a tool call request, got none")
	}
	if toolCall.ID != "toolu_1" || toolCall.Name != "fetch_url" {
		t.Errorf("toolCall = %+v, want ID=toolu_1 Name=fetch_url", toolCall)
	}
	if string(toolCall.Arguments) != `{"url":"https://x"}` {
		t.Errorf("toolCall.Arguments = %q, want %q", toolCall.Arguments, `{"url":"https://x"}`)
	}
	if !done {
		t.Error("expected a ChatEventDone before channel close")
	}
	if usage == nil || usage.InputTokens != 42 || usage.OutputTokens != 17 {
		t.Errorf("usage = %+v, want InputTokens=42 OutputTokens=17", usage)
	}
}

// noArgToolSSEBody mirrors what Claude actually streams for a tool whose
// schema takes no parameters (e.g. compute_ranking): content_block_start
// with input:{} immediately followed by content_block_stop — zero
// input_json_delta events, since there's nothing to stream.
const noArgToolSSEBody = `event: message_start
data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","content":[],"model":"claude-3-5-sonnet-20241022","usage":{"input_tokens":10,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_2","name":"compute_ranking","input":{}}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"input_tokens":10,"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`

// TestChat_ZeroArgumentToolCall_DefaultsToEmptyObject guards against a
// regression: Arguments must never come out as "" for a no-parameter tool
// call — replaying an empty string as tool_use.input on the next
// roundtrip is rejected by Claude's API ("Field required"), even though
// the original streamed response is well-formed.
func TestChat_ZeroArgumentToolCall_DefaultsToEmptyObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(noArgToolSSEBody))
	}))
	defer server.Close()

	provider := New(Config{APIKey: "test-key", BaseURL: server.URL, Model: "claude-sonnet-5"})
	events, err := provider.Chat(context.Background(), agent.ChatRequest{Stream: true})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}

	var toolCall *agent.ToolCall
	for event := range events {
		if event.Error != nil {
			t.Fatalf("unexpected event error: %v", event.Error)
		}
		if event.Type == agent.ChatEventToolCallRequest {
			toolCall = event.ToolCall
		}
	}

	if toolCall == nil {
		t.Fatal("expected a tool call request, got none")
	}
	if string(toolCall.Arguments) != "{}" {
		t.Errorf("toolCall.Arguments = %q, want %q", toolCall.Arguments, "{}")
	}
}

func TestFormatMessages_MergesConsecutiveToolResults(t *testing.T) {
	provider := New(Config{APIKey: "test-key"})

	toolResult1, _ := json.Marshal(agent.StoredToolResult{ToolCallID: "call_1", Result: json.RawMessage(`{"ok":true}`)})
	toolResult2, _ := json.Marshal(agent.StoredToolResult{ToolCallID: "call_2", Result: json.RawMessage(`{"ok":false}`)})
	toolCalls, _ := json.Marshal([]agent.StoredToolCall{
		{ID: "call_1", Name: "fetch_url", Input: json.RawMessage(`{"url":"https://x"}`)},
		{ID: "call_2", Name: "fetch_url", Input: json.RawMessage(`{"url":"https://y"}`)},
	})

	rows := []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "Check these two listings"},
		{Role: domain.ChatRoleAssistant, ToolCalls: toolCalls},
		{Role: domain.ChatRoleTool, ToolResult: toolResult1},
		{Role: domain.ChatRoleTool, ToolResult: toolResult2},
		{Role: domain.ChatRoleAssistant, Content: "Both checked."},
	}

	messages := provider.FormatMessages(rows)

	if len(messages) != 4 {
		t.Fatalf("got %d messages, want 4 (user, assistant/tool_use, user/tool_results merged, assistant); messages=%+v", len(messages), messages)
	}
	toolResultMsg := messages[2]
	if toolResultMsg.Role != "user" {
		t.Fatalf("tool result message role = %q, want user", toolResultMsg.Role)
	}
	if len(toolResultMsg.Content) != 2 {
		t.Fatalf("tool result message has %d blocks, want 2 (merged), got %+v", len(toolResultMsg.Content), toolResultMsg.Content)
	}
	if toolResultMsg.Content[0].ToolResultForID != "call_1" || toolResultMsg.Content[1].ToolResultForID != "call_2" {
		t.Errorf("tool result blocks = %+v, want ToolResultForID call_1 then call_2", toolResultMsg.Content)
	}
}

func TestFormatMessages_TranslatesImageBlock(t *testing.T) {
	messages := []agent.Message{
		{Role: "user", Content: []agent.ContentBlock{
			{Type: agent.ContentBlockText, Text: "Hier ist ein Screenshot"},
			{Type: agent.ContentBlockImage, ImageData: "ZmFrZQ==", ImageMediaType: "image/png"},
		}},
	}

	params := formatMessages(messages)

	if len(params) != 1 {
		t.Fatalf("got %d messages, want 1", len(params))
	}
	blocks := params[0].Content
	if len(blocks) != 2 {
		t.Fatalf("got %d content blocks, want 2 (text + image), got %+v", len(blocks), blocks)
	}
	imageBlock := blocks[1].OfImage
	if imageBlock == nil {
		t.Fatalf("second block is not an image block: %+v", blocks[1])
	}
	if imageBlock.Source.OfBase64 == nil || imageBlock.Source.OfBase64.Data != "ZmFrZQ==" {
		t.Errorf("image data = %+v, want OfBase64.Data = %q", imageBlock.Source.OfBase64, "ZmFrZQ==")
	}
	if imageBlock.Source.OfBase64 != nil && imageBlock.Source.OfBase64.MediaType != anthropic.Base64ImageSourceMediaTypeImagePNG {
		t.Errorf("image media type = %q, want image/png", imageBlock.Source.OfBase64.MediaType)
	}
}

func TestFormatMessages_TranslatesLinkedPDFAfterToolResult(t *testing.T) {
	messages := []agent.Message{
		{Role: "user", Content: []agent.ContentBlock{
			{Type: agent.ContentBlockToolResult, ToolResultForID: "call_pdf", ToolResultText: `{"media_type":"application/pdf"}`},
			{Type: agent.ContentBlockDocument, DocumentData: "JVBERi0xLjQ="},
		}},
	}

	params := formatMessages(messages)

	if len(params) != 1 || len(params[0].Content) != 2 {
		t.Fatalf("formatMessages = %+v, want one message with tool_result and document", params)
	}
	if params[0].Content[0].OfToolResult == nil {
		t.Fatalf("first block = %+v, want tool_result", params[0].Content[0])
	}
	document := params[0].Content[1].OfDocument
	if document == nil || document.Source.OfBase64 == nil || document.Source.OfBase64.Data != "JVBERi0xLjQ=" {
		t.Errorf("second block = %+v, want base64 PDF document", params[0].Content[1])
	}
}

// TestFormatMessages_RepairedHistoryHasNoDanglingToolUse replays the
// buglist scenario end to end through the Claude formatter: stored rows
// with an assistant tool_use whose role=tool row never got written, then
// the next user message. After RepairToolPairing the wire messages pair up.
func TestFormatMessages_RepairedHistoryHasNoDanglingToolUse(t *testing.T) {
	provider := New(Config{APIKey: "test-key"})
	toolCalls, _ := json.Marshal([]agent.StoredToolCall{
		{ID: "call_1", Name: "fetch_url", Input: json.RawMessage(`{"url":"https://x"}`)},
	})
	rows := []domain.ChatMessage{
		{Role: domain.ChatRoleUser, Content: "Check this"},
		{Role: domain.ChatRoleAssistant, ToolCalls: toolCalls},
		{Role: domain.ChatRoleUser, Content: "Und jetzt?"},
	}

	wire := formatMessages(agent.RepairToolPairing(provider.FormatMessages(rows)))

	if len(wire) != 3 {
		t.Fatalf("got %d wire messages, want 3", len(wire))
	}
	results := wire[2].Content
	if len(results) != 2 || results[0].OfToolResult == nil || results[0].OfToolResult.ToolUseID != "call_1" {
		t.Fatalf("wire[2] = %+v, want tool_result for call_1 first, then the user text", results)
	}
	if results[1].OfText == nil {
		t.Errorf("user text block missing after the tool_result: %+v", results[1])
	}
}

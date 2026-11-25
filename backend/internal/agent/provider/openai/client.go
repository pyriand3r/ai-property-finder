// Package openai is the agent.Provider for OpenAI-compatible Chat
// Completions backends: LM Studio and OpenCode (ADR 0030, extends ADR 0006/
// 0007). It speaks plain HTTP + SSE (`POST {base_url}/chat/completions`,
// `stream: true`) and has no SDK dependency — every server of this family
// implements the same small slice of the API.
//
// Capabilities differ from Claude: no native PDF input (ADR 0005 — the loop
// reports a linked PDF as a tool error), and no image input, because whether
// an image block works depends on the loaded model, which the config cannot
// express.
package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/domain"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Config is the subset of llm.* settings the provider needs.
type Config struct {
	// BaseURL includes the API version prefix, e.g. http://localhost:1234/v1.
	BaseURL string
	// APIKey is optional: LM Studio needs none, OpenCode Zen does. Sent as a
	// Bearer token only when non-empty.
	APIKey         string
	Model          string
	TimeoutSeconds int
}

func (c Config) timeout() time.Duration {
	if c.TimeoutSeconds <= 0 {
		return defaultTimeoutSeconds * time.Second
	}
	return time.Duration(c.TimeoutSeconds) * time.Second
}

const (
	defaultTimeoutSeconds = 30
	// maxTokens caps one roundtrip's assistant response (same as the Claude
	// provider).
	maxTokens          = 4096
	eventChannelBuffer = 16
	// maxStreamLine bounds one SSE line. A tool call's arguments arrive in
	// small deltas, but some servers send a whole call in one chunk.
	maxStreamLine = 4 << 20
	// maxErrorBody bounds how much of a non-2xx response is put in the error.
	maxErrorBody = 2048
)

// Provider talks to an OpenAI-compatible Chat Completions endpoint in
// streaming mode. It holds no conversation state.
type Provider struct {
	cfg    Config
	client *http.Client
}

// New constructs a Provider. cfg.BaseURL must be non-empty — callers gate
// construction on that (internal/llm).
func New(cfg Config) *Provider {
	// No http.Client.Timeout: it would also cap reading the stream body.
	// The per-call deadline is a context in Chat.
	return &Provider{cfg: cfg, client: &http.Client{}}
}

// SupportsDocumentInput is false: there is no portable PDF content block in
// the Chat Completions API (ADR 0005/0006 capability flag).
func (p *Provider) SupportsDocumentInput() bool { return false }

// SupportsImageInput is false: only vision models accept image parts and the
// config cannot tell whether the loaded model is one.
func (p *Provider) SupportsImageInput() bool { return false }

// FormatTools converts app-level Tools into the OpenAI `tools` array. The
// concrete type is hidden behind `any` per the agent.Provider interface.
func (p *Provider) FormatTools(tools []agent.Tool) any {
	result := make([]wireTool, 0, len(tools))
	for _, tool := range tools {
		parameters := tool.InputSchema
		if len(bytes.TrimSpace(parameters)) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		result = append(result, wireTool{
			Type:     "function",
			Function: wireFunctionDef{Name: tool.Name, Description: tool.Description, Parameters: parameters},
		})
	}
	return result
}

// FormatMessages maps stored rows to normalized messages; the OpenAI
// role="tool" split happens in Chat (agent.StoredHistoryMessages).
func (p *Provider) FormatMessages(messages []domain.ChatMessage) []agent.Message {
	return agent.StoredHistoryMessages(messages)
}

type wireTool struct {
	Type     string          `json:"type"`
	Function wireFunctionDef `json:"function"`
}

type wireFunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireFunctionCall `json:"function"`
}

type wireFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireRequest struct {
	Model         string         `json:"model"`
	Messages      []wireMessage  `json:"messages"`
	Tools         []wireTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions map[string]any `json:"stream_options,omitempty"`
	MaxTokens     int            `json:"max_tokens"`
}

// buildMessages maps the normalized history onto OpenAI messages. The system
// blocks become one system message; a user Message holding tool_result
// blocks becomes one role="tool" message per result (followed by a user
// message for any remaining text); an assistant Message carries its
// tool_use blocks as tool_calls. Document and image blocks are dropped — the
// provider reports neither capability, so none should arrive.
func buildMessages(system []agent.ContentBlock, messages []agent.Message) []wireMessage {
	result := make([]wireMessage, 0, len(messages)+1)
	var systemText []string
	for _, block := range system {
		if block.Type == agent.ContentBlockText && block.Text != "" {
			systemText = append(systemText, block.Text)
		}
	}
	if len(systemText) > 0 {
		result = append(result, wireMessage{Role: "system", Content: new(strings.Join(systemText, "\n\n"))})
	}
	for _, m := range messages {
		if m.Role == "assistant" {
			result = append(result, assistantMessage(m))
			continue
		}
		var text []string
		for _, block := range m.Content {
			switch block.Type {
			case agent.ContentBlockToolResult:
				result = append(result, wireMessage{Role: "tool", ToolCallID: block.ToolResultForID, Content: new(block.ToolResultText)})
			case agent.ContentBlockText:
				text = append(text, block.Text)
			}
		}
		if len(text) > 0 {
			result = append(result, wireMessage{Role: "user", Content: new(strings.Join(text, "\n\n"))})
		}
	}
	return result
}

func assistantMessage(m agent.Message) wireMessage {
	wire := wireMessage{Role: "assistant"}
	var text []string
	for _, block := range m.Content {
		switch block.Type {
		case agent.ContentBlockText:
			text = append(text, block.Text)
		case agent.ContentBlockToolUse:
			wire.ToolCalls = append(wire.ToolCalls, wireToolCall{
				ID:   block.ToolUseID,
				Type: "function",
				Function: wireFunctionCall{
					Name:      block.ToolName,
					Arguments: argsOrEmptyObject(string(block.ToolInput)),
				},
			})
		}
	}
	// content must be present (not omitted) even when empty for strict
	// servers; null is only valid together with tool_calls.
	if len(text) > 0 {
		wire.Content = new(strings.Join(text, "\n\n"))
	} else if len(wire.ToolCalls) == 0 {
		wire.Content = new("")
	}
	return wire
}

// argsOrEmptyObject returns raw, or "{}" when empty or "null": a tool
// without parameters streams no argument chunks, and a replayed empty or
// null arguments string is rejected by strict servers.
func argsOrEmptyObject(raw string) string {
	if trimmed := strings.TrimSpace(raw); trimmed == "" || trimmed == "null" {
		return "{}"
	}
	return raw
}

// Chat sends req in streaming mode and translates the SSE stream into
// normalized ChatEvents. The request is made inside the stream goroutine so a
// cancellation during connect surfaces as an event carrying context.Canceled
// — the same shape the Claude provider has, which Loop.Run relies on to tell
// a Stop click from a failure. The channel is closed after a ChatEventDone or
// an event with a non-nil Error.
func (p *Provider) Chat(ctx context.Context, req agent.ChatRequest) (<-chan agent.ChatEvent, error) {
	payload := wireRequest{
		Model:         p.cfg.Model,
		Messages:      buildMessages(req.System, req.Messages),
		Stream:        true,
		StreamOptions: map[string]any{"include_usage": true},
		MaxTokens:     maxTokens,
	}
	if tools, ok := p.FormatTools(req.Tools).([]wireTool); ok && len(tools) > 0 {
		payload.Tools = tools
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode chat request: %w", err)
	}

	callCtx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	events := make(chan agent.ChatEvent, eventChannelBuffer)
	go p.stream(callCtx, body, events, cancel)
	return events, nil
}

func (p *Provider) endpoint() string {
	return strings.TrimRight(strings.TrimSpace(p.cfg.BaseURL), "/") + "/chat/completions"
}

func (p *Provider) stream(ctx context.Context, body []byte, events chan<- agent.ChatEvent, cancel context.CancelFunc) {
	defer cancel()
	defer close(events)

	fail := func(err error) { events <- agent.ChatEvent{Error: classifyError(ctx, err)} }

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint(), bytes.NewReader(body))
	if err != nil {
		fail(fmt.Errorf("build chat request: %w", err))
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if key := strings.TrimSpace(p.cfg.APIKey); key != "" {
		httpReq.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		fail(err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		fail(fmt.Errorf("llm endpoint returned %s: %s", resp.Status, strings.TrimSpace(string(snippet))))
		return
	}

	tracker := newToolCallTracker()
	usage := agent.Usage{}
	finished := false

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxStreamLine)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue // comments, event: lines, blanks
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			finished = true
			break
		}
		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			fail(fmt.Errorf("decode stream chunk: %w", err))
			return
		}
		if chunk.Error != nil {
			fail(errors.New(chunk.Error.Message))
			return
		}
		if chunk.Usage != nil {
			usage.InputTokens = chunk.Usage.PromptTokens
			usage.OutputTokens = chunk.Usage.CompletionTokens
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				events <- agent.ChatEvent{Type: agent.ChatEventTextDelta, Text: choice.Delta.Content}
			}
			for _, delta := range choice.Delta.ToolCalls {
				if fragment := tracker.add(delta); fragment != "" {
					events <- agent.ChatEvent{Type: agent.ChatEventToolCallDelta, Text: fragment}
				}
			}
			if choice.FinishReason != "" {
				finished = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		fail(err)
		return
	}
	if !finished {
		// A cut connection must not look like a finished reply: the loop
		// would store a truncated answer as complete.
		fail(io.ErrUnexpectedEOF)
		return
	}

	for _, call := range tracker.complete() {
		events <- agent.ChatEvent{Type: agent.ChatEventToolCallRequest, ToolCall: call}
	}
	events <- agent.ChatEvent{Type: agent.ChatEventDone, Usage: &usage}
}

// classifyError normalizes cancellation and deadline errors so Loop.Run can
// match them with errors.Is regardless of net/http's wrapping.
func classifyError(ctx context.Context, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return context.Canceled
	default:
		return err
	}
}

type streamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string            `json:"content"`
			ToolCalls []streamToolDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type streamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type pendingCall struct {
	id, name string
	args     strings.Builder
}

// toolCallTracker assembles tool calls from streamed deltas. Unlike Claude's
// content_block_stop, OpenAI has no per-call end marker, so calls are only
// complete once the stream ends.
type toolCallTracker struct {
	calls map[int]*pendingCall
}

func newToolCallTracker() *toolCallTracker {
	return &toolCallTracker{calls: map[int]*pendingCall{}}
}

// add folds one delta in and returns its argument fragment (for progress).
func (t *toolCallTracker) add(delta streamToolDelta) string {
	call, ok := t.calls[delta.Index]
	if !ok {
		call = &pendingCall{}
		t.calls[delta.Index] = call
	}
	if delta.ID != "" {
		call.id = delta.ID
	}
	if delta.Function.Name != "" {
		// Name arrives whole in the first delta; some servers repeat it in
		// later ones, so assign rather than append.
		call.name = delta.Function.Name
	}
	call.args.WriteString(delta.Function.Arguments)
	return delta.Function.Arguments
}

// complete returns the assembled calls in index order. A call without a
// name is dropped (nothing to dispatch). Servers that omit the ID get a
// generated one — the ID pairs the result on the next roundtrip.
func (t *toolCallTracker) complete() []*agent.ToolCall {
	indexes := make([]int, 0, len(t.calls))
	for index := range t.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	result := make([]*agent.ToolCall, 0, len(indexes))
	for _, index := range indexes {
		call := t.calls[index]
		if call.name == "" {
			continue
		}
		id := call.id
		if id == "" {
			id = "call_" + uuid.NewString()
		}
		result = append(result, &agent.ToolCall{
			ID:        id,
			Name:      call.name,
			Arguments: json.RawMessage(argsOrEmptyObject(call.args.String())),
		})
	}
	return result
}

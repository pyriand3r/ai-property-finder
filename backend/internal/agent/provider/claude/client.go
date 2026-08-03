// Package claude is the Claude API implementation of agent.Provider (spec
// 00-03, ADR 0007). It is the sole MVP provider (ADR 0006); LM Studio and
// OpenCode implementations will live as sibling packages behind the same
// agent.Provider interface.
package claude

import (
	"context"
	"encoding/json"
	"errors"
	"propertyfinder/internal/agent"
	"propertyfinder/internal/domain"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// Config is the subset of llm.* settings the Claude provider needs. main.go
// maps config.LLM onto this at startup.
type Config struct {
	APIKey         string
	BaseURL        string
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
	// maxTokens caps one turn's assistant response. Generous for a chat
	// reply plus a handful of tool calls in one roundtrip.
	maxTokens = 4096
	// eventChannelBuffer keeps the translator goroutine from blocking on a
	// slow consumer for the common case of a burst of text deltas.
	eventChannelBuffer = 16
)

// Provider talks to Anthropic's Messages API in streaming mode. It holds no
// conversation state — every field it needs comes in on ChatRequest.
type Provider struct {
	cfg    Config
	client anthropic.Client
}

// New constructs a Claude Provider. cfg.APIKey must be non-empty — callers
// gate construction on that the same way internal/ai.NewProvider does.
func New(cfg Config) *Provider {
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if strings.TrimSpace(cfg.BaseURL) != "" {
		opts = append(opts, option.WithBaseURL(cfg.BaseURL))
	}
	return &Provider{cfg: cfg, client: anthropic.NewClient(opts...)}
}

func (p *Provider) SupportsDocumentInput() bool { return true }

// FormatTools converts app-level Tool definitions into Anthropic's
// ToolUnionParam wire shape. The concrete type is hidden behind `any` per
// the agent.Provider interface; Chat type-asserts its own return value.
func (p *Provider) FormatTools(tools []agent.Tool) any {
	params := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		var schema struct {
			Properties any      `json:"properties"`
			Required   []string `json:"required"`
		}
		// A malformed schema constant would be a programming error caught
		// at startup (tools.go), not a runtime condition to recover from —
		// an empty schema is a safe zero value if unmarshal ever failed.
		_ = json.Unmarshal(tool.InputSchema, &schema)
		params = append(params, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: schema.Properties,
					Required:   schema.Required,
				},
			},
		})
	}
	return params
}

// FormatMessages converts DB-neutral chat_message rows into normalized
// agent.Message values, applying Claude's wire convention: tool results
// belong on a "user" turn as ToolResult content blocks, immediately
// following the assistant turn that requested them. Consecutive
// role=tool rows collapse into one user Message, matching how Claude
// expects all tool_results for one assistant turn to arrive together.
func (p *Provider) FormatMessages(messages []domain.ChatMessage) []agent.Message {
	result := make([]agent.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case domain.ChatRoleUser:
			result = append(result, agent.Message{
				Role:    "user",
				Content: []agent.ContentBlock{{Type: agent.ContentBlockText, Text: m.Content}},
			})
		case domain.ChatRoleAssistant:
			result = append(result, agent.Message{Role: "assistant", Content: assistantBlocks(m)})
		case domain.ChatRoleTool:
			block := toolResultBlock(m)
			if n := len(result); n > 0 && result[n-1].Role == "user" && isToolResultMessage(result[n-1]) {
				result[n-1].Content = append(result[n-1].Content, block)
			} else {
				result = append(result, agent.Message{Role: "user", Content: []agent.ContentBlock{block}})
			}
		}
	}
	return result
}

func assistantBlocks(m domain.ChatMessage) []agent.ContentBlock {
	var blocks []agent.ContentBlock
	if strings.TrimSpace(m.Content) != "" {
		blocks = append(blocks, agent.ContentBlock{Type: agent.ContentBlockText, Text: m.Content})
	}
	if len(m.ToolCalls) > 0 {
		var calls []agent.StoredToolCall
		if err := json.Unmarshal(m.ToolCalls, &calls); err == nil {
			for _, call := range calls {
				blocks = append(blocks, agent.ContentBlock{
					Type:      agent.ContentBlockToolUse,
					ToolUseID: call.ID,
					ToolName:  call.Name,
					ToolInput: call.Input,
				})
			}
		}
	}
	return blocks
}

func toolResultBlock(m domain.ChatMessage) agent.ContentBlock {
	var stored agent.StoredToolResult
	_ = json.Unmarshal(m.ToolResult, &stored)
	return agent.ContentBlock{
		Type:            agent.ContentBlockToolResult,
		ToolResultForID: stored.ToolCallID,
		ToolResultText:  string(stored.Result),
	}
}

func isToolResultMessage(m agent.Message) bool {
	for _, block := range m.Content {
		if block.Type != agent.ContentBlockToolResult {
			return false
		}
	}
	return len(m.Content) > 0
}

// Chat sends req to Claude in streaming mode and translates Anthropic's SSE
// event stream into the normalized ChatEvent channel. The channel is
// closed after a ChatEventDone or an event carrying a non-nil Error.
func (p *Provider) Chat(ctx context.Context, req agent.ChatRequest) (<-chan agent.ChatEvent, error) {
	params := anthropic.MessageNewParams{
		Model:     p.cfg.Model,
		MaxTokens: maxTokens,
		System:    formatSystem(req.System),
		Messages:  formatMessages(req.Messages),
	}
	if tools, ok := p.FormatTools(req.Tools).([]anthropic.ToolUnionParam); ok {
		params.Tools = tools
	}

	callCtx, cancel := context.WithTimeout(ctx, p.cfg.timeout())
	stream := p.client.Messages.NewStreaming(callCtx, params)

	events := make(chan agent.ChatEvent, eventChannelBuffer)
	go translateStream(stream, events, cancel)
	return events, nil
}

func formatSystem(blocks []agent.ContentBlock) []anthropic.TextBlockParam {
	params := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == agent.ContentBlockText {
			params = append(params, anthropic.TextBlockParam{Text: block.Text})
		}
	}
	return params
}

func formatMessages(messages []agent.Message) []anthropic.MessageParam {
	params := make([]anthropic.MessageParam, 0, len(messages))
	for _, m := range messages {
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(m.Content))
		for _, block := range m.Content {
			switch block.Type {
			case agent.ContentBlockText:
				blocks = append(blocks, anthropic.NewTextBlock(block.Text))
			case agent.ContentBlockToolUse:
				var input any
				_ = json.Unmarshal(block.ToolInput, &input)
				blocks = append(blocks, anthropic.NewToolUseBlock(block.ToolUseID, input, block.ToolName))
			case agent.ContentBlockToolResult:
				blocks = append(blocks, anthropic.NewToolResultBlock(block.ToolResultForID, block.ToolResultText, block.ToolResultError))
			}
		}
		role := anthropic.MessageParamRoleUser
		if m.Role == "assistant" {
			role = anthropic.MessageParamRoleAssistant
		}
		params = append(params, anthropic.MessageParam{Role: role, Content: blocks})
	}
	return params
}

// pendingToolCall accumulates one tool_use content block's streamed
// arguments across content_block_delta events until content_block_stop.
type pendingToolCall struct {
	id, name string
	args     strings.Builder
}

// translateStream drains an Anthropic SSE stream and emits normalized
// ChatEvents. It owns cancel — called once the stream (and any consumer of
// the returned channel) is done with callCtx.
func translateStream(stream *ssestream.Stream[anthropic.MessageStreamEventUnion], events chan<- agent.ChatEvent, cancel context.CancelFunc) {
	defer cancel()
	defer close(events)

	pending := map[int64]*pendingToolCall{}
	usage := agent.Usage{}

	for stream.Next() {
		event := stream.Current()
		switch variant := event.AsAny().(type) {
		case anthropic.MessageStartEvent:
			usage.InputTokens = int(variant.Message.Usage.InputTokens)
		case anthropic.ContentBlockStartEvent:
			if variant.ContentBlock.Type == "tool_use" {
				pending[variant.Index] = &pendingToolCall{id: variant.ContentBlock.ID, name: variant.ContentBlock.Name}
			}
		case anthropic.ContentBlockDeltaEvent:
			switch delta := variant.Delta.AsAny().(type) {
			case anthropic.TextDelta:
				events <- agent.ChatEvent{Type: agent.ChatEventTextDelta, Text: delta.Text}
			case anthropic.InputJSONDelta:
				if call, ok := pending[variant.Index]; ok {
					call.args.WriteString(delta.PartialJSON)
					events <- agent.ChatEvent{Type: agent.ChatEventToolCallDelta, Text: delta.PartialJSON}
				}
			}
		case anthropic.ContentBlockStopEvent:
			if call, ok := pending[variant.Index]; ok {
				delete(pending, variant.Index)
				events <- agent.ChatEvent{
					Type: agent.ChatEventToolCallRequest,
					ToolCall: &agent.ToolCall{
						ID:        call.id,
						Name:      call.name,
						Arguments: json.RawMessage(call.args.String()),
					},
				}
			}
		case anthropic.MessageDeltaEvent:
			usage.OutputTokens = int(variant.Usage.OutputTokens)
		}
	}

	if err := stream.Err(); err != nil {
		events <- agent.ChatEvent{Error: classifyStreamError(err)}
		return
	}
	events <- agent.ChatEvent{Type: agent.ChatEventDone, Usage: &usage}
}

func classifyStreamError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return err
}

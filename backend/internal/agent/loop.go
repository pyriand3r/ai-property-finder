package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// DefaultMaxLLMRoundtrips and DefaultMaxToolCallsPerTurn are the same
// fallbacks config.LoadAll applies to app_config — repeated here so a Loop
// built without going through config (e.g. in a test) still behaves
// sanely, and so BuildSystemPrompt's "Grenzen" text and Run's actual
// budget can never drift apart (spec 00-03, ADR 0013).
const (
	DefaultMaxLLMRoundtrips    = 5
	DefaultMaxToolCallsPerTurn = 15
)

// ErrNotConfigured means no LLM backend is configured (llm.provider is
// empty, or llm.api_key is missing). Not retryable.
var ErrNotConfigured = errors.New("agent_llm_not_configured")

// EffectiveMaxLLMRoundtrips normalizes an unset/invalid config value onto
// DefaultMaxLLMRoundtrips. Exported so callers assembling the system
// prompt's "Grenzen" text (which states the budget to the model) use
// exactly the same number Run() enforces.
func EffectiveMaxLLMRoundtrips(configured int) int {
	if configured <= 0 {
		return DefaultMaxLLMRoundtrips
	}
	return configured
}

func effectiveMaxToolCallsPerTurn(configured int) int {
	if configured <= 0 {
		return DefaultMaxToolCallsPerTurn
	}
	return configured
}

// TurnRequest is one call to Run — the assembled system prompt (already in
// preamble+Global Context+Profile Context order, see BuildSystemPrompt)
// and the full prior message history for one turn's roundtrip loop.
type TurnRequest struct {
	System   []ContentBlock
	Messages []Message
	Tools    []Tool
}

// TurnResult is what one Run call produced: the assistant's final text
// reply — the concatenation of every text_delta across every roundtrip
// spent on this turn — and the summed token usage across those roundtrips.
type TurnResult struct {
	Text  string
	Usage Usage
}

// DispatchFunc resolves one tool call to its result. Returns the result
// text to feed back to the model as a tool_result block, and whether it
// represents an error (spec 00-03 "Error Handling": tool failures are
// reported to the Agent, not raised as a turn failure).
type DispatchFunc func(ctx context.Context, call ToolCall) (result string, isError bool)

// OnEvent is called synchronously for every ChatEvent Run receives from
// the Provider, across every roundtrip, so the caller can bridge each one
// onto SSE as it happens rather than buffering until Run returns.
type OnEvent func(ChatEvent)

// Loop drives the multi-roundtrip conversation with a Provider until the
// model stops requesting tool calls, a limit is hit, or the Provider
// errors (ADR 0007). Layer 6.3 ships with no Tool catalog — TurnRequest
// .Tools is always empty and Dispatch is always nil, so every real turn is
// exactly one roundtrip ending in text only. The roundtrip/tool-call
// limits and the dispatch hook are wired now so Layer 6.4 only needs to
// populate the catalog, not touch loop structure.
type Loop struct {
	Provider            Provider
	MaxLLMRoundtrips    int
	MaxToolCallsPerTurn int
	Dispatch            DispatchFunc
}

// Run drives roundtrips until the model replies with no further tool
// calls, the roundtrip or tool-call budget is exhausted, or the Provider
// errors. On budget exhaustion it does not return an error — spec 00-03
// says hitting max_llm_roundtrips "returns a tool error to the Agent" so
// it can wrap up in the same turn; Run appends one instruction to that
// effect to the accumulated text instead of making the next roundtrip.
func (loop *Loop) Run(ctx context.Context, req TurnRequest, onEvent OnEvent) (TurnResult, error) {
	maxRoundtrips := EffectiveMaxLLMRoundtrips(loop.MaxLLMRoundtrips)
	maxToolCalls := effectiveMaxToolCallsPerTurn(loop.MaxToolCallsPerTurn)

	messages := append([]Message(nil), req.Messages...)
	var text strings.Builder
	var usage Usage
	toolCallsUsed := 0

	for roundtrip := 1; roundtrip <= maxRoundtrips; roundtrip++ {
		events, err := loop.Provider.Chat(ctx, ChatRequest{
			System:   req.System,
			Messages: messages,
			Tools:    req.Tools,
			Stream:   true,
		})
		if err != nil {
			return TurnResult{Text: text.String(), Usage: usage}, err
		}

		roundtripText, toolCalls, roundtripUsage, streamErr := drainChat(events, onEvent)
		usage.InputTokens += roundtripUsage.InputTokens
		usage.OutputTokens += roundtripUsage.OutputTokens
		text.WriteString(roundtripText)
		if streamErr != nil {
			return TurnResult{Text: text.String(), Usage: usage}, streamErr
		}

		if len(toolCalls) == 0 {
			return TurnResult{Text: text.String(), Usage: usage}, nil
		}

		messages = append(messages, Message{Role: "assistant", Content: assistantResponseBlocks(roundtripText, toolCalls)})

		budgetExhausted := roundtrip == maxRoundtrips || toolCallsUsed+len(toolCalls) > maxToolCalls
		if budgetExhausted {
			text.WriteString(limitReachedNote(maxRoundtrips))
			return TurnResult{Text: text.String(), Usage: usage}, nil
		}

		toolCallsUsed += len(toolCalls)
		messages = append(messages, Message{Role: "user", Content: loop.dispatchAll(ctx, toolCalls)})
	}

	return TurnResult{Text: text.String(), Usage: usage}, nil
}

// drainChat consumes one roundtrip's ChatEvent channel to completion,
// forwarding every event to onEvent as it arrives.
func drainChat(events <-chan ChatEvent, onEvent OnEvent) (text string, toolCalls []ToolCall, usage Usage, err error) {
	var builder strings.Builder
	for event := range events {
		onEvent(event)
		if event.Error != nil {
			err = event.Error
			continue
		}
		switch event.Type {
		case ChatEventTextDelta:
			builder.WriteString(event.Text)
		case ChatEventToolCallRequest:
			if event.ToolCall != nil {
				toolCalls = append(toolCalls, *event.ToolCall)
			}
		case ChatEventDone:
			if event.Usage != nil {
				usage = *event.Usage
			}
		}
	}
	return builder.String(), toolCalls, usage, err
}

func assistantResponseBlocks(text string, toolCalls []ToolCall) []ContentBlock {
	var blocks []ContentBlock
	if text != "" {
		blocks = append(blocks, ContentBlock{Type: ContentBlockText, Text: text})
	}
	for _, call := range toolCalls {
		blocks = append(blocks, ContentBlock{
			Type:      ContentBlockToolUse,
			ToolUseID: call.ID,
			ToolName:  call.Name,
			ToolInput: call.Arguments,
		})
	}
	return blocks
}

// dispatchAll resolves every tool call via Dispatch (or a stand-in error
// result when no dispatcher is wired — Layer 6.3 never reaches this path
// since it offers no Tools for the model to call) and returns the
// tool_result content blocks for the next roundtrip's user turn.
func (loop *Loop) dispatchAll(ctx context.Context, calls []ToolCall) []ContentBlock {
	blocks := make([]ContentBlock, 0, len(calls))
	for _, call := range calls {
		resultText, isError := loop.dispatch(ctx, call)
		blocks = append(blocks, ContentBlock{
			Type:            ContentBlockToolResult,
			ToolResultForID: call.ID,
			ToolResultText:  resultText,
			ToolResultError: isError,
		})
	}
	return blocks
}

func (loop *Loop) dispatch(ctx context.Context, call ToolCall) (string, bool) {
	if loop.Dispatch == nil {
		return `{"error":"tool not available"}`, true
	}
	return loop.Dispatch(ctx, call)
}

func limitReachedNote(maxRoundtrips int) string {
	return fmt.Sprintf("\n\n[Limit von %d LLM-Anfragen erreicht — fasse den Stand zusammen und frag, ob weitergemacht werden soll.]", maxRoundtrips)
}

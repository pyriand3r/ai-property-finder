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

// TurnResult is what one Run call produced: the concatenation of every
// roundtrip's text across the whole turn, and the summed token usage.
// Persistence is driven by OnRoundtrip, not this — TurnResult is a
// convenience summary for callers that don't need row-by-row detail (e.g.
// tests).
type TurnResult struct {
	Text  string
	Usage Usage
}

// ToolResult is one dispatched Tool call's outcome. Result is the JSON text
// fed back as a tool_result block. DocumentData optionally carries a
// base64-encoded PDF discovered by the Tool; Loop appends it as a native
// document block for this turn only and keeps it out of ToolOutcome, so chat
// history never persists or replays the payload.
type ToolResult struct {
	Result       string
	IsError      bool
	DocumentData string
}

// DispatchFunc resolves one tool call to its result. Tool failures are
// reported to the Agent, not raised as a turn failure (spec 00-03 "Error
// Handling").
type DispatchFunc func(ctx context.Context, call ToolCall) ToolResult

// OnEvent is called synchronously for every ChatEvent Run receives from
// the Provider, across every roundtrip, so the caller can bridge each one
// onto SSE as it happens rather than buffering until Run returns.
type OnEvent func(ChatEvent)

// ToolOutcome is one dispatched tool call's result, paired with the call
// that produced it.
type ToolOutcome struct {
	Call    ToolCall
	Result  string
	IsError bool
}

// RoundtripOutcome is what one LLM roundtrip produced: the assistant's
// message for that roundtrip (text and/or tool_use blocks) and — once
// dispatched — each tool call's outcome. Results is empty for the
// roundtrip that ends the turn (no tool calls requested, or the budget
// was hit before dispatching).
type RoundtripOutcome struct {
	Assistant Message
	Results   []ToolOutcome
}

// OnRoundtrip is called once per completed roundtrip, after any tool
// calls in it have been dispatched. The caller persists it as its own
// chat_message row(s) — spec 00-02/ADR 0010 model history as one
// assistant row per roundtrip (carrying tool_calls when present) plus one
// role=tool row per dispatched call, not a single row per turn. Without
// this, a second user message in the same session would replay a history
// with every intermediate tool call/result silently missing.
type OnRoundtrip func(RoundtripOutcome)

// Loop drives the multi-roundtrip conversation with a Provider until the
// model stops requesting tool calls, a limit is hit, or the Provider
// errors (ADR 0007).
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
// effect to the roundtrip's own text instead of making the next roundtrip.
func (loop *Loop) Run(ctx context.Context, req TurnRequest, onEvent OnEvent, onRoundtrip OnRoundtrip) (TurnResult, error) {
	maxRoundtrips := EffectiveMaxLLMRoundtrips(loop.MaxLLMRoundtrips)
	maxToolCalls := effectiveMaxToolCallsPerTurn(loop.MaxToolCallsPerTurn)

	messages := append([]Message(nil), req.Messages...)
	var text strings.Builder
	var usage Usage
	toolCallsUsed := 0

	for roundtrip := 1; roundtrip <= maxRoundtrips; roundtrip++ {
		// Checked before every roundtrip, not just once: this is the only
		// place a "stop" (spec 01-01) can take effect between roundtrips —
		// e.g. right after a tool call finished dispatching (dispatchAll
		// deliberately ignores ctx cancellation so an in-flight tool call
		// always runs to completion, see below).
		if ctx.Err() != nil {
			//nolint:nilerr // ctx.Err() here means "stop requested" (spec 01-01), not a turn failure — stopped() intentionally reports nil error so the caller's chat.message path runs instead of chat.turn_error.
			return loop.stopped(&text, usage, "", onRoundtrip), nil
		}

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
		if streamErr != nil {
			if errors.Is(streamErr, context.Canceled) {
				// The model was still streaming its reply (no tool_use
				// blocks committed to messages yet) when "stop" fired —
				// treat like any other mid-turn stop: persist what arrived
				// as a normal roundtrip, not a turn_error. Any tool_use
				// blocks drainChat did parse before the cancellation are
				// intentionally dropped by not passing toolCalls through —
				// they were never dispatched, so including them would leave
				// a dangling tool_use a later turn's history replay can't
				// satisfy (same reasoning as the budget-exhausted branch
				// below).
				return loop.stopped(&text, usage, roundtripText, onRoundtrip), nil
			}
			text.WriteString(roundtripText)
			return TurnResult{Text: text.String(), Usage: usage}, streamErr
		}

		if len(toolCalls) == 0 {
			text.WriteString(roundtripText)
			assistant := Message{Role: "assistant", Content: assistantResponseBlocks(roundtripText, nil)}
			onRoundtrip(RoundtripOutcome{Assistant: assistant})
			return TurnResult{Text: text.String(), Usage: usage}, nil
		}

		assistantBlocks := assistantResponseBlocks(roundtripText, toolCalls)
		messages = append(messages, Message{Role: "assistant", Content: assistantBlocks})

		budgetExhausted := roundtrip == maxRoundtrips || toolCallsUsed+len(toolCalls) > maxToolCalls
		if budgetExhausted {
			note := limitReachedNote(maxRoundtrips)
			text.WriteString(roundtripText)
			text.WriteString(note)
			// The cutoff note is surfaced as this roundtrip's own text —
			// visible in chat history exactly where the turn stopped —
			// rather than only appended to TurnResult.Text. Persisted as
			// text-only: toolCalls were requested but never dispatched (no
			// budget left to run dispatchAll), so there's no matching
			// tool_result. Including their tool_use blocks here would leave
			// an assistant message the next turn's history replay can't
			// satisfy — Claude requires every tool_use to have a result in
			// the immediately following message — breaking "weitermachen"
			// (spec 01-09 "weitermachen after budget-limit").
			assistant := Message{Role: "assistant", Content: assistantResponseBlocks(roundtripText+note, nil)}
			onRoundtrip(RoundtripOutcome{Assistant: assistant})
			return TurnResult{Text: text.String(), Usage: usage}, nil
		}

		text.WriteString(roundtripText)
		toolCallsUsed += len(toolCalls)

		resultBlocks, outcomes := loop.dispatchAll(ctx, toolCalls)
		messages = append(messages, Message{Role: "user", Content: resultBlocks})
		onRoundtrip(RoundtripOutcome{Assistant: Message{Role: "assistant", Content: assistantBlocks}, Results: outcomes})
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
// result when no dispatcher is wired) and returns both the tool_result
// content blocks for the next roundtrip's user turn and the raw outcomes
// for the caller to persist. Native PDF blocks follow all tool_result blocks
// because Claude rejects non-tool-result content interleaved between them.
func (loop *Loop) dispatchAll(ctx context.Context, calls []ToolCall) ([]ContentBlock, []ToolOutcome) {
	// context.WithoutCancel: a "stop" click (spec 01-01) must let any tool
	// call currently executing run to completion — Dispatch implementations
	// use ctx for real I/O (fetch_url's HTTP GET, save_listing's DB write),
	// so passing the cancellable ctx through here would abort a write
	// mid-flight instead of just skipping the *next* one. Run's own
	// cancellation check (top of the roundtrip loop) still stops further
	// LLM requests once this whole batch finishes.
	dispatchCtx := context.WithoutCancel(ctx)
	blocks := make([]ContentBlock, 0, 2*len(calls))
	documents := make([]ContentBlock, 0, len(calls))
	outcomes := make([]ToolOutcome, 0, len(calls))
	for _, call := range calls {
		result := loop.dispatch(dispatchCtx, call)
		if result.DocumentData != "" && !loop.Provider.SupportsDocumentInput() {
			result.Result = `{"error":"PDF geladen, aber der aktuelle LLM-Provider unterstützt keine PDF-Dokumente."}`
			result.IsError = true
			result.DocumentData = ""
		}
		blocks = append(blocks, ContentBlock{
			Type:            ContentBlockToolResult,
			ToolResultForID: call.ID,
			ToolResultText:  result.Result,
			ToolResultError: result.IsError,
		})
		if result.DocumentData != "" {
			documents = append(documents, ContentBlock{
				Type:         ContentBlockDocument,
				DocumentData: result.DocumentData,
			})
		}
		outcomes = append(outcomes, ToolOutcome{Call: call, Result: result.Result, IsError: result.IsError})
	}
	return append(blocks, documents...), outcomes
}

func (loop *Loop) dispatch(ctx context.Context, call ToolCall) ToolResult {
	if loop.Dispatch == nil {
		return ToolResult{Result: `{"error":"tool not available"}`, IsError: true}
	}
	return loop.Dispatch(ctx, call)
}

func limitReachedNote(maxRoundtrips int) string {
	return fmt.Sprintf("\n\n[Limit von %d LLM-Anfragen erreicht — fasse den Stand zusammen und frag, ob weitergemacht werden soll.]", maxRoundtrips)
}

// stopped finalizes a turn ended by the "stop" control (spec 01-01, ADR
// 0008: cancellation flag, not a rollback): persists one last synthetic
// roundtrip — pendingRoundtripText (whatever the current roundtrip had
// streamed before it was cut off, empty if cancellation was noticed between
// roundtrips) plus a short note, text only, never a tool_use block — and
// returns a nil error so the caller treats this exactly like a normal
// turn end (its own chat.message, not chat.turn_error): an intentional stop
// is not a failure.
func (loop *Loop) stopped(text *strings.Builder, usage Usage, pendingRoundtripText string, onRoundtrip OnRoundtrip) TurnResult {
	messageText := pendingRoundtripText + stoppedNote()
	text.WriteString(messageText)
	assistant := Message{Role: "assistant", Content: assistantResponseBlocks(messageText, nil)}
	onRoundtrip(RoundtripOutcome{Assistant: assistant})
	return TurnResult{Text: text.String(), Usage: usage}
}

func stoppedNote() string {
	return "\n\n[Abgebrochen durch Nutzer.]"
}

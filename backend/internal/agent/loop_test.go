package agent

import (
	"context"
	"encoding/json"
	"errors"
	"propertyfinder/internal/domain"
	"strings"
	"testing"
)

var errBoom = errors.New("boom")

// fakeProvider replays a fixed sequence of ChatEvent batches, one batch per
// Chat() call — enough to drive Loop.Run through multiple roundtrips
// without a real LLM.
type fakeProvider struct {
	batches  [][]ChatEvent
	err      error
	calls    int
	requests []ChatRequest
}

func (p *fakeProvider) SupportsDocumentInput() bool                   { return true }
func (p *fakeProvider) SupportsImageInput() bool                      { return true }
func (p *fakeProvider) FormatTools([]Tool) any                        { return nil }
func (p *fakeProvider) FormatMessages([]domain.ChatMessage) []Message { return nil }

func (p *fakeProvider) Chat(_ context.Context, req ChatRequest) (<-chan ChatEvent, error) {
	if p.err != nil {
		return nil, p.err
	}
	p.requests = append(p.requests, req)
	batch := p.batches[p.calls]
	p.calls++
	ch := make(chan ChatEvent, len(batch))
	for _, event := range batch {
		ch <- event
	}
	close(ch)
	return ch, nil
}

func TestLoop_Run_TextOnlyEndsAfterOneRoundtrip(t *testing.T) {
	provider := &fakeProvider{
		batches: [][]ChatEvent{
			{
				{Type: ChatEventTextDelta, Text: "Hallo, "},
				{Type: ChatEventTextDelta, Text: "wie kann ich helfen?"},
				{Type: ChatEventDone, Usage: &Usage{InputTokens: 10, OutputTokens: 5}},
			},
		},
	}
	loop := &Loop{Provider: provider}

	var seen []ChatEvent
	var roundtrips []RoundtripOutcome
	result, err := loop.Run(context.Background(), TurnRequest{},
		func(e ChatEvent) { seen = append(seen, e) },
		func(r RoundtripOutcome) { roundtrips = append(roundtrips, r) },
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Text != "Hallo, wie kann ich helfen?" {
		t.Errorf("Text = %q, want %q", result.Text, "Hallo, wie kann ich helfen?")
	}
	if result.Usage.InputTokens != 10 || result.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v, want InputTokens=10 OutputTokens=5", result.Usage)
	}
	if provider.calls != 1 {
		t.Errorf("Provider.Chat called %d times, want 1 (no tool calls requested)", provider.calls)
	}
	if len(seen) != 3 {
		t.Errorf("onEvent saw %d events, want 3 (forwarded every event)", len(seen))
	}
	if len(roundtrips) != 1 {
		t.Fatalf("onRoundtrip fired %d times, want 1", len(roundtrips))
	}
	if len(roundtrips[0].Results) != 0 {
		t.Errorf("final roundtrip has %d Results, want 0 (no tool calls)", len(roundtrips[0].Results))
	}
	if len(roundtrips[0].Assistant.Content) != 1 || roundtrips[0].Assistant.Content[0].Text != "Hallo, wie kann ich helfen?" {
		t.Errorf("Assistant.Content = %+v, want one text block with the reply", roundtrips[0].Assistant.Content)
	}
}

func TestLoop_Run_DispatchesToolCallAndContinues(t *testing.T) {
	provider := &fakeProvider{
		batches: [][]ChatEvent{
			{
				{Type: ChatEventToolCallRequest, ToolCall: &ToolCall{ID: "call_1", Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x"}`)}},
				{Type: ChatEventDone, Usage: &Usage{InputTokens: 5, OutputTokens: 2}},
			},
			{
				{Type: ChatEventTextDelta, Text: "Fertig."},
				{Type: ChatEventDone, Usage: &Usage{InputTokens: 8, OutputTokens: 3}},
			},
		},
	}
	dispatched := 0
	loop := &Loop{
		Provider: provider,
		Dispatch: func(_ context.Context, call ToolCall) ToolResult {
			dispatched++
			if call.Name != "fetch_url" {
				t.Errorf("dispatched tool = %q, want fetch_url", call.Name)
			}
			return ToolResult{Result: `{"text":"ok"}`}
		},
	}

	var roundtrips []RoundtripOutcome
	result, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {},
		func(r RoundtripOutcome) { roundtrips = append(roundtrips, r) },
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Text != "Fertig." {
		t.Errorf("Text = %q, want %q", result.Text, "Fertig.")
	}
	if dispatched != 1 {
		t.Errorf("dispatched %d tool calls, want 1", dispatched)
	}
	if provider.calls != 2 {
		t.Errorf("Provider.Chat called %d times, want 2 (one per roundtrip)", provider.calls)
	}
	if result.Usage.InputTokens != 13 || result.Usage.OutputTokens != 5 {
		t.Errorf("Usage = %+v, want summed across roundtrips (InputTokens=13 OutputTokens=5)", result.Usage)
	}

	if len(roundtrips) != 2 {
		t.Fatalf("onRoundtrip fired %d times, want 2 (one per roundtrip)", len(roundtrips))
	}
	if len(roundtrips[0].Results) != 1 || roundtrips[0].Results[0].Call.Name != "fetch_url" {
		t.Errorf("first roundtrip Results = %+v, want one fetch_url outcome", roundtrips[0].Results)
	}
	toolUseFound := false
	for _, block := range roundtrips[0].Assistant.Content {
		if block.Type == ContentBlockToolUse && block.ToolName == "fetch_url" {
			toolUseFound = true
		}
	}
	if !toolUseFound {
		t.Errorf("first roundtrip Assistant.Content = %+v, want a tool_use block for fetch_url", roundtrips[0].Assistant.Content)
	}
	if len(roundtrips[1].Results) != 0 {
		t.Errorf("second (final) roundtrip has %d Results, want 0", len(roundtrips[1].Results))
	}
}

func TestLoop_Run_AttachesLinkedPDFAfterAllToolResults(t *testing.T) {
	provider := &fakeProvider{
		batches: [][]ChatEvent{
			{
				{Type: ChatEventToolCallRequest, ToolCall: &ToolCall{ID: "call_pdf", Name: "fetch_url", Arguments: json.RawMessage(`{"url":"https://x/floorplan.pdf"}`)}},
				{Type: ChatEventToolCallRequest, ToolCall: &ToolCall{ID: "call_save", Name: "save_listing", Arguments: json.RawMessage(`{}`)}},
				{Type: ChatEventDone},
			},
			{
				{Type: ChatEventTextDelta, Text: "Grundriss berücksichtigt."},
				{Type: ChatEventDone},
			},
		},
	}
	loop := &Loop{
		Provider: provider,
		Dispatch: func(_ context.Context, call ToolCall) ToolResult {
			if call.ID == "call_pdf" {
				return ToolResult{Result: `{"media_type":"application/pdf"}`, DocumentData: "JVBERi0xLjQ="}
			}
			return ToolResult{Result: `{"status":"ok"}`}
		},
	}

	var roundtrips []RoundtripOutcome
	_, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {}, func(outcome RoundtripOutcome) {
		roundtrips = append(roundtrips, outcome)
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(provider.requests) != 2 {
		t.Fatalf("Provider.Chat requests = %d, want 2", len(provider.requests))
	}
	messages := provider.requests[1].Messages
	blocks := messages[len(messages)-1].Content
	if len(blocks) != 3 {
		t.Fatalf("tool-result message blocks = %+v, want two tool results followed by one PDF", blocks)
	}
	if blocks[0].Type != ContentBlockToolResult || blocks[0].ToolResultForID != "call_pdf" ||
		blocks[1].Type != ContentBlockToolResult || blocks[1].ToolResultForID != "call_save" {
		t.Errorf("first blocks = %+v, want both tool_result blocks before document content", blocks[:2])
	}
	if blocks[2].Type != ContentBlockDocument || blocks[2].DocumentData != "JVBERi0xLjQ=" {
		t.Errorf("last block = %+v, want the linked native PDF", blocks[2])
	}
	if len(roundtrips) != 2 || len(roundtrips[0].Results) != 2 {
		t.Fatalf("roundtrips = %+v, want two persisted tool outcomes", roundtrips)
	}
	if strings.Contains(roundtrips[0].Results[0].Result, "JVBERi0xLjQ=") {
		t.Error("persisted tool outcome contains base64 PDF data")
	}
}

func TestLoop_Run_StopsAtRoundtripLimitWithPendingToolCall(t *testing.T) {
	batch := []ChatEvent{
		{Type: ChatEventToolCallRequest, ToolCall: &ToolCall{ID: "call_1", Name: "fetch_url", Arguments: json.RawMessage(`{}`)}},
		{Type: ChatEventDone, Usage: &Usage{InputTokens: 1, OutputTokens: 1}},
	}
	provider := &fakeProvider{batches: [][]ChatEvent{batch, batch}}
	loop := &Loop{
		Provider:         provider,
		MaxLLMRoundtrips: 2,
		Dispatch:         func(context.Context, ToolCall) ToolResult { return ToolResult{Result: `{}`} },
	}

	var roundtrips []RoundtripOutcome
	result, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {},
		func(r RoundtripOutcome) { roundtrips = append(roundtrips, r) },
	)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.calls != 2 {
		t.Errorf("Provider.Chat called %d times, want exactly MaxLLMRoundtrips=2", provider.calls)
	}
	if want := "[Limit von 2 LLM-Anfragen erreicht"; !strings.Contains(result.Text, want) {
		t.Errorf("Text = %q, want it to contain %q", result.Text, want)
	}
	// Two onRoundtrip calls: roundtrip 1 dispatches its tool call normally
	// (budget not yet exhausted) and persists with a Results entry;
	// roundtrip 2 is the one that hits the limit and never dispatches.
	if len(roundtrips) != 2 {
		t.Fatalf("onRoundtrip fired %d times, want 2", len(roundtrips))
	}
	if len(roundtrips[0].Results) != 1 {
		t.Errorf("first roundtrip has %d Results, want 1 (dispatched before the limit)", len(roundtrips[0].Results))
	}
	if len(roundtrips[1].Results) != 0 {
		t.Errorf("second (budget-exhausted) roundtrip has %d Results, want 0 (never dispatched)", len(roundtrips[1].Results))
	}
	// Regression guard (spec 01-09 "weitermachen after budget-limit"): the
	// budget-exhausted roundtrip's persisted Assistant message must not
	// contain a tool_use block for the call that never got dispatched —
	// replaying that on the next turn (no matching tool_result exists) is
	// rejected by Claude's API and left the Agent unable to continue.
	for _, block := range roundtrips[1].Assistant.Content {
		if block.Type == ContentBlockToolUse {
			t.Errorf("budget-exhausted roundtrip's Assistant.Content contains a tool_use block (%+v), want text only", block)
		}
	}
}

func TestLoop_Run_PropagatesProviderError(t *testing.T) {
	provider := &fakeProvider{err: errBoom}
	loop := &Loop{Provider: provider}

	_, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {}, func(RoundtripOutcome) {})
	if !errors.Is(err, errBoom) {
		t.Errorf("Run error = %v, want errBoom", err)
	}
}

// TestLoop_Run_StopsBetweenRoundtripsOnCancel covers the "stop" control
// (spec 01-01) taking effect right after a tool call finished dispatching:
// Dispatch itself cancels ctx (standing in for a Stop click arriving while
// the tool call was running) and also asserts the ctx it received is NOT
// canceled — proving dispatchAll's context.WithoutCancel detached it, so
// the in-flight call ran to completion rather than observing the
// cancellation. Run must then notice ctx.Err() before starting a second
// roundtrip and wrap up cleanly (nil error, not a turn_error).
func TestLoop_Run_StopsBetweenRoundtripsOnCancel(t *testing.T) {
	provider := &fakeProvider{
		batches: [][]ChatEvent{
			{
				{Type: ChatEventToolCallRequest, ToolCall: &ToolCall{ID: "call_1", Name: "fetch_url", Arguments: json.RawMessage(`{}`)}},
				{Type: ChatEventDone, Usage: &Usage{InputTokens: 1, OutputTokens: 1}},
			},
			// A second batch would prove the bug if Run ever consumed it —
			// fakeProvider.Chat panics via index-out-of-range if called a
			// third time, but two batches lets us assert calls == 1 below
			// without a slice-bounds risk if the stop check fails to fire.
			{{Type: ChatEventDone}},
		},
	}
	var cancel context.CancelFunc
	dispatchSawCanceledCtx := false
	loop := &Loop{
		Provider: provider,
		Dispatch: func(ctx context.Context, _ ToolCall) ToolResult {
			cancel()
			dispatchSawCanceledCtx = ctx.Err() != nil
			return ToolResult{Result: `{}`}
		},
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	cancel = cancelFunc
	var roundtrips []RoundtripOutcome
	result, err := loop.Run(ctx, TurnRequest{}, func(ChatEvent) {},
		func(r RoundtripOutcome) { roundtrips = append(roundtrips, r) },
	)
	if err != nil {
		t.Fatalf("Run returned error: %v, want nil (a stop is not a turn error)", err)
	}
	if dispatchSawCanceledCtx {
		t.Error("Dispatch observed a canceled ctx — dispatchAll must detach via context.WithoutCancel so an in-flight tool call runs to completion")
	}
	if provider.calls != 1 {
		t.Errorf("Provider.Chat called %d times, want 1 (no further roundtrip once ctx is canceled)", provider.calls)
	}
	if want := "[Abgebrochen durch Nutzer.]"; !strings.Contains(result.Text, want) {
		t.Errorf("Text = %q, want it to contain %q", result.Text, want)
	}
	if len(roundtrips) != 2 {
		t.Fatalf("onRoundtrip fired %d times, want 2 (the dispatched roundtrip + the stopped wrap-up)", len(roundtrips))
	}
	if len(roundtrips[0].Results) != 1 {
		t.Errorf("first roundtrip has %d Results, want 1 (the tool call ran to completion)", len(roundtrips[0].Results))
	}
	for _, block := range roundtrips[1].Assistant.Content {
		if block.Type == ContentBlockToolUse {
			t.Errorf("stopped wrap-up roundtrip's Assistant.Content contains a tool_use block (%+v), want text only", block)
		}
	}
}

// TestLoop_Run_StopsMidStream covers "stop" firing while the model is still
// streaming its reply (no tool_use blocks committed yet): the Provider's
// event stream ends with a ChatEvent carrying context.Canceled, mirroring
// what classifyStreamError normalizes a genuinely canceled HTTP request to.
func TestLoop_Run_StopsMidStream(t *testing.T) {
	provider := &fakeProvider{
		batches: [][]ChatEvent{
			{
				{Type: ChatEventTextDelta, Text: "Teil"},
				{Error: context.Canceled},
			},
		},
	}
	loop := &Loop{Provider: provider}

	var roundtrips []RoundtripOutcome
	result, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {},
		func(r RoundtripOutcome) { roundtrips = append(roundtrips, r) },
	)
	if err != nil {
		t.Fatalf("Run returned error: %v, want nil (a stop is not a turn error)", err)
	}
	if want := "Teil\n\n[Abgebrochen durch Nutzer.]"; result.Text != want {
		t.Errorf("Text = %q, want %q", result.Text, want)
	}
	if len(roundtrips) != 1 {
		t.Fatalf("onRoundtrip fired %d times, want 1", len(roundtrips))
	}
	if len(roundtrips[0].Assistant.Content) != 1 || roundtrips[0].Assistant.Content[0].Type != ContentBlockText {
		t.Errorf("Assistant.Content = %+v, want one text block only", roundtrips[0].Assistant.Content)
	}
}

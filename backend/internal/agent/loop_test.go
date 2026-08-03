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
	batches [][]ChatEvent
	err     error
	calls   int
}

func (p *fakeProvider) SupportsDocumentInput() bool                   { return true }
func (p *fakeProvider) FormatTools([]Tool) any                        { return nil }
func (p *fakeProvider) FormatMessages([]domain.ChatMessage) []Message { return nil }

func (p *fakeProvider) Chat(context.Context, ChatRequest) (<-chan ChatEvent, error) {
	if p.err != nil {
		return nil, p.err
	}
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
	result, err := loop.Run(context.Background(), TurnRequest{}, func(e ChatEvent) { seen = append(seen, e) })
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
		Dispatch: func(_ context.Context, call ToolCall) (string, bool) {
			dispatched++
			if call.Name != "fetch_url" {
				t.Errorf("dispatched tool = %q, want fetch_url", call.Name)
			}
			return `{"text":"ok"}`, false
		},
	}

	result, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {})
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
		Dispatch:         func(context.Context, ToolCall) (string, bool) { return `{}`, false },
	}

	result, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if provider.calls != 2 {
		t.Errorf("Provider.Chat called %d times, want exactly MaxLLMRoundtrips=2", provider.calls)
	}
	if want := "[Limit von 2 LLM-Anfragen erreicht"; !strings.Contains(result.Text, want) {
		t.Errorf("Text = %q, want it to contain %q", result.Text, want)
	}
}

func TestLoop_Run_PropagatesProviderError(t *testing.T) {
	provider := &fakeProvider{err: errBoom}
	loop := &Loop{Provider: provider}

	_, err := loop.Run(context.Background(), TurnRequest{}, func(ChatEvent) {})
	if !errors.Is(err, errBoom) {
		t.Errorf("Run error = %v, want errBoom", err)
	}
}

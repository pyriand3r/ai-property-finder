// Package agent holds the multi-turn Agent loop's provider-independent
// boundary (ADR 0007). This is distinct from internal/ai, which holds the
// single-shot operations (evaluate_listing, GenerateRanking) kept from
// dev-property-finder — the Agent invokes those as Tools (Layer 6.5), it
// doesn't replace them.
//
// Nothing here knows about Claude, OpenAI, or any wire format. The
// boundary is content blocks and normalized events; each concrete
// provider (internal/agent/provider/claude, later provider/openai-
// compatible) translates to and from its own HTTP/SSE shape.
package agent

import (
	"context"
	"encoding/json"
	"propertyfinder/internal/domain"
)

// ContentBlockType distinguishes the kinds of content a Message or system
// prompt block can carry.
type ContentBlockType string

const (
	ContentBlockText       ContentBlockType = "text"
	ContentBlockToolUse    ContentBlockType = "tool_use"
	ContentBlockToolResult ContentBlockType = "tool_result"
)

// ContentBlock is one normalized piece of a Message or system prompt.
// Which fields are set depends on Type.
type ContentBlock struct {
	Type ContentBlockType

	// Text: set for ContentBlockText.
	Text string

	// ToolUse fields: set for ContentBlockToolUse — the assistant
	// requesting a tool call.
	ToolUseID string
	ToolName  string
	ToolInput json.RawMessage

	// ToolResult fields: set for ContentBlockToolResult — feeding a
	// tool's result back to the model. ToolResultForID matches a
	// preceding ToolUseID.
	ToolResultForID string
	ToolResultText  string
	ToolResultError bool
}

// Message is one normalized conversation turn sent to Chat(). Role is
// "user" or "assistant" — a tool result is a "user" turn from Claude's
// wire perspective (ADR 0010), but that mapping is decided inside each
// Provider's FormatMessages, not here.
type Message struct {
	Role    string
	Content []ContentBlock
}

// Tool is an app-level Tool definition — name, description, and JSON
// Schema parameters. internal/agent/tools.go (Layer 6.4+) defines the
// actual catalog; this type only describes the shape.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage
}

// ChatRequest is one turn's request to a Provider.
type ChatRequest struct {
	// System is the assembled preamble + Global Context + Profile
	// Context + listing context (00-03 "Context Assembly Detail"),
	// already split into blocks so a provider can send it as its own
	// system-prompt shape.
	System []ContentBlock
	// Messages is the full session history, already normalized via
	// FormatMessages.
	Messages []Message
	Tools    []Tool
	// Stream is always true in practice — loop.go only ever streams, so
	// there's currently no non-streaming caller. Kept on the struct for
	// interface fidelity with ADR 0007's original shape.
	Stream bool
}

// ChatEventType enumerates the normalized stream events a Provider emits.
type ChatEventType string

const (
	// ChatEventTextDelta carries an incremental chunk of assistant text.
	ChatEventTextDelta ChatEventType = "text_delta"
	// ChatEventToolCallDelta carries an incremental chunk of a tool
	// call's JSON arguments, still being streamed. loop.go doesn't act on
	// these except to relay "still working" progress; only
	// ChatEventToolCallRequest (the complete call) is dispatched.
	ChatEventToolCallDelta ChatEventType = "tool_call_delta"
	// ChatEventToolCallRequest carries one complete tool call, ready to
	// dispatch.
	ChatEventToolCallRequest ChatEventType = "tool_call_request"
	// ChatEventDone closes the stream and carries final Usage.
	ChatEventDone ChatEventType = "done"
)

// ToolCall is one complete tool-call request from the model.
type ToolCall struct {
	ID        string
	Name      string
	Arguments json.RawMessage
}

// Usage is the token accounting for one Chat() call (ADR 0014's token
// counter reads this directly from the API response — no estimation).
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ChatEvent is one normalized event from a Provider's stream.
type ChatEvent struct {
	Type     ChatEventType
	Text     string
	ToolCall *ToolCall
	Usage    *Usage
	Error    error
}

// Provider abstracts the swappable multi-turn LLM backend (ADR 0007). It
// does not know what Tools do, does not dispatch them, and does not decide
// whether to loop again — all of that is loop.go's job.
type Provider interface {
	// SupportsDocumentInput reports whether this provider accepts
	// document (PDF) content blocks natively (spec 00-03 "PDF Handling").
	SupportsDocumentInput() bool
	// FormatTools converts app-level Tool definitions into this
	// provider's expected JSON schema format. Called once per turn.
	FormatTools(tools []Tool) any
	// FormatMessages converts DB-neutral chat_message rows into this
	// provider's message structures (ADR 0010) — e.g. mapping a
	// role=tool row into a Claude tool_result content block on a "user"
	// message, or an OpenAI role="tool" message for a future provider.
	FormatMessages(messages []domain.ChatMessage) []Message
	// Chat sends req and returns a channel of normalized events. The
	// channel is closed after a ChatEventDone or ChatEvent with a
	// non-nil Error.
	Chat(ctx context.Context, req ChatRequest) (<-chan ChatEvent, error)
}

// StoredToolCall is the JSON shape of one element in
// chat_message.tool_calls for an assistant-role row — this project's own
// storage format (not any provider's wire format), shared by every
// Provider's FormatMessages so the parsing logic isn't duplicated.
type StoredToolCall struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// StoredToolResult is the JSON shape of chat_message.tool_result for a
// role=tool row (ADR 0010: `{"tool_call_id":"...","result":...}`).
type StoredToolResult struct {
	ToolCallID string          `json:"tool_call_id"`
	Result     json.RawMessage `json:"result"`
}
